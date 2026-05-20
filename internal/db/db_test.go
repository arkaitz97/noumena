package db_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"noumena/internal/db"
)

func TestDBQueries_Full(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping DB integration tests; TEST_DATABASE_URL not set")
	}

	err := db.InitDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to dev database: %v", err)
	}
	defer db.CloseDB()

	queries := db.NewPostgresQuery(db.Pool)
	ctx := context.Background()

	// 1. User
	email := fmt.Sprintf("test_full_%d@example.com", time.Now().UnixNano())
	user, err := queries.CreateUser(ctx, email, "testhash")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	fetchedUser, err := queries.GetUserByEmail(ctx, email)
	if err != nil || fetchedUser.Email != email {
		t.Fatalf("GetUserByEmail failed: expected %s, got err %v", email, err)
	}

	// 2. Channels
	now := time.Now()
	testChannelID := fmt.Sprintf("yt123_%d", now.UnixNano())
	err = queries.ConnectChannelTx(ctx, user.ID, "youtube", testChannelID, "Test Channel", "enc_acc", "enc_ref", &now)
	if err != nil {
		t.Fatalf("ConnectChannelTx failed: %v", err)
	}

	chans, err := queries.ListChannelsForUser(ctx, user.ID)
	if err != nil || len(chans) != 1 {
		t.Fatalf("ListChannelsForUser failed: %v", err)
	}
	pcID := chans[0].ID

	// 3. Pause
	err = queries.PauseChannelTracking(ctx, user.ID, pcID)
	if err != nil {
		t.Fatalf("PauseChannelTracking failed: %v", err)
	}

	// Unpause manually for further tests
	_, _ = db.Pool.Exec(ctx, "UPDATE user_channel_tracking SET tracking_status='active'")

	// Sync
	_, _ = db.Pool.Exec(ctx, "UPDATE platform_channels SET last_synced_at = NOW() - interval '1 hour' WHERE id = $1", pcID)

	syncChans, err := queries.GetChannelsForSync(ctx, 30)
	if err != nil || len(syncChans) == 0 {
		t.Fatalf("GetChannelsForSync failed: %v", err)
	}
	err = queries.UpdateSyncTime(ctx, pcID)
	if err != nil {
		t.Fatalf("UpdateSyncTime failed: %v", err)
	}

	// 4. Tracking / Token
	acc, ref, err := queries.GetIngestionToken(ctx, pcID)
	if err != nil || acc != "enc_acc" || ref != "enc_ref" {
		t.Fatalf("GetIngestionToken failed")
	}
	err = queries.PromoteNextIngestionToken(ctx, pcID)
	if err != nil {
		t.Fatalf("PromoteNextIngestionToken failed")
	}

	// 5. Comments
	testCommentID := fmt.Sprintf("comm1_%d", time.Now().UnixNano())
	comment := &db.Comment{
		PlatformChannelID: pcID,
		PlatformCommentID: testCommentID,
		Body:              "Test comment",
		PostedAt:          time.Now(),
	}
	err = queries.InsertComment(ctx, comment)
	if err != nil {
		t.Fatalf("InsertComment failed: %v", err)
	}

	// Fetch comment ID correctly
	var commID string
	err = db.Pool.QueryRow(ctx, "SELECT id FROM comments WHERE platform_comment_id = $1 LIMIT 1", testCommentID).Scan(&commID)
	if err != nil {
		t.Fatalf("Failed to find inserted comment: %v", err)
	}

	err = queries.StarComment(ctx, user.ID, commID)
	if err != nil {
		t.Fatalf("StarComment failed: %v", err)
	}

	// Unprocessed
	unproc, err := queries.GetUnprocessedComments(ctx, 10)
	if err != nil || len(unproc) == 0 {
		t.Fatalf("GetUnprocessedComments failed: %v", err)
	}

	err = queries.UpdateCommentLLMData(ctx, commID, "praise", "none", "openai", "gpt-4o", 0.5, 0.9)
	if err != nil {
		t.Fatalf("UpdateCommentLLMData failed: %v", err)
	}

	feed, err := queries.GetCommentFeed(ctx, user.ID, 10, 0)
	if err != nil {
		t.Fatalf("GetCommentFeed failed: %v", err)
	}
	if len(feed) != 1 {
		t.Fatalf("GetCommentFeed expected 1, got %d", len(feed))
	}

	// 6. Digests
	db.Pool.Exec(ctx, "UPDATE users SET email_verified_at = NOW() WHERE id = $1", user.ID)
	digestUsers, err := queries.GetUsersForDigest(ctx)
	if err != nil || len(digestUsers) == 0 {
		t.Fatalf("GetUsersForDigest failed")
	}

	recent, err := queries.GetRecentProcessedCommentsForUser(ctx, user.ID, time.Now().Add(-1*time.Hour))
	if err != nil || len(recent) == 0 {
		t.Fatalf("GetRecentProcessedCommentsForUser failed")
	}

	digest := &db.WeeklyDigest{
		UserID:        user.ID,
		WeekStartDate: time.Now().Format("2006-01-02"),
		Status:        "generated",
		Content:       "{}",
	}
	err = queries.InsertWeeklyDigest(ctx, digest)
	if err != nil {
		t.Fatalf("InsertWeeklyDigest failed: %v", err)
	}

	// 7. Remove
	err = queries.RemoveChannelTracking(ctx, user.ID, pcID)
	if err != nil {
		t.Fatalf("RemoveChannelTracking failed")
	}
}
