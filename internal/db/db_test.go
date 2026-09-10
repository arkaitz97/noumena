package db_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"noumena/internal/db"
	"noumena/internal/dbtest"
)

func TestDBQueries_Full(t *testing.T) {
	dbPath := os.Getenv("TEST_DATABASE_PATH")
	if dbPath == "" {
		dbPath = ":memory:"
	}

	err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.CloseDB()

	// Apply schema to in-memory database
	ctx := context.Background()
	if err := db.ApplyMigrations(ctx); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	queries := db.NewSQLiteQuery(db.DB)
	sq := queries.(*db.SQLiteQuery)

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
	_, _ = sq.DB().ExecContext(ctx, "UPDATE user_channel_tracking SET tracking_status='active'")

	// Sync
	_, _ = sq.DB().ExecContext(ctx, "UPDATE platform_channels SET last_synced_at = datetime('now', '-1 hour') WHERE id = ?", pcID)

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
	err = sq.DB().QueryRowContext(ctx, "SELECT id FROM comments WHERE platform_comment_id = ? LIMIT 1", testCommentID).Scan(&commID)
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

	err = queries.UpdateCommentLLMData(ctx, commID, "praise", "none", "openai", "gpt-4o", []byte(`[]`), 0.9)
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
	sq.DB().ExecContext(ctx, "UPDATE users SET email_verified_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?", user.ID)
	digestUsers, err := queries.GetUsersForDigest(ctx)
	if err != nil {
		t.Fatalf("GetUsersForDigest error: %v", err)
	}
	if len(digestUsers) == 0 {
		t.Fatalf("GetUsersForDigest returned 0 users")
	}

	since := time.Now().Add(-1 * time.Hour)
	recent, err := queries.GetRecentProcessedCommentsForUser(ctx, user.ID, since)
	if err != nil {
		t.Fatalf("GetRecentProcessedCommentsForUser error: %v", err)
	}
	if len(recent) == 0 {
		t.Fatalf("GetRecentProcessedCommentsForUser returned 0 comments")
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

func setupRecoveryDB(t *testing.T) (context.Context, *db.SQLiteQuery) {
	t.Helper()
	database := dbtest.SetupTestDB(t)
	ctx := context.Background()
	if err := db.Migrate(ctx, database); err != nil {
		// Harness already migrated; ignore duplicate baseline errors only if applied.
		_ = err
	}
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	return ctx, q
}

func seedChannel(t *testing.T, ctx context.Context, q *db.SQLiteQuery, tag string) (string, string) {
	t.Helper()
	email := fmt.Sprintf("rec_%s_%d@example.com", tag, time.Now().UnixNano())
	u, err := q.CreateUser(ctx, email, "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	chID, _, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-"+tag+"-"+u.ID[:8], "C")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	exp := time.Now().UTC().Add(time.Hour)
	if _, err := q.CreateUserChannelTracking(ctx, u.ID, chID, "a", "r", exp, 90, 0); err != nil {
		t.Fatalf("tracking: %v", err)
	}
	return u.ID, chID
}

func TestRecoverStaleIngestionJobs(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	_, chID := seedChannel(t, ctx, q, "ing")
	jobID, err := q.EnqueueIngestionJob(ctx, chID, "poll")
	if err != nil || jobID == "" {
		t.Fatalf("enqueue: %v %q", err, jobID)
	}
	if _, err := q.ClaimNextIngestionJob(ctx); err != nil {
		t.Fatalf("claim: %v", err)
	}
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := q.DB().ExecContext(ctx, `UPDATE ingestion_jobs SET started_at=? WHERE id=?`, old, jobID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	n, err := q.RecoverStaleIngestionJobs(ctx, 15*time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("recover want 1 got %d err %v", n, err)
	}
}

func countActiveIngestionJobs(t *testing.T, ctx context.Context, q *db.SQLiteQuery, chID string) int {
	t.Helper()
	var n int
	if err := q.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ingestion_jobs
		  WHERE platform_channel_id = ? AND status IN ('pending','running')`, chID).Scan(&n); err != nil {
		t.Fatalf("count active jobs: %v", err)
	}
	return n
}

// A pending backfill must suppress a poll for the same channel: with
// AcquireChannelLock a no-op under SQLite, cross-type enqueue dedupe is the
// only thing preventing backfill and poll from racing on the same channel.
func TestEnqueueIngestionJob_SuppressesCrossTypeForSameChannel(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	_, chID := seedChannel(t, ctx, q, "xsuppress")

	backfillID, err := q.EnqueueIngestionJob(ctx, chID, "backfill")
	if err != nil || backfillID == "" {
		t.Fatalf("backfill enqueue want id: id=%q err=%v", backfillID, err)
	}

	pollID, err := q.EnqueueIngestionJob(ctx, chID, "poll")
	if err != nil {
		t.Fatalf("poll enqueue error: %v", err)
	}
	if pollID != "" {
		t.Fatalf("poll must be suppressed while backfill active, got id %q", pollID)
	}
	if n := countActiveIngestionJobs(t, ctx, q, chID); n != 1 {
		t.Fatalf("want exactly 1 active job, got %d", n)
	}

	// Running state must suppress just the same as pending.
	if _, err := q.ClaimNextIngestionJob(ctx); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if id, err := q.EnqueueIngestionJob(ctx, chID, "poll"); err != nil || id != "" {
		t.Fatalf("poll must stay suppressed while backfill running: id=%q err=%v", id, err)
	}

	// A failed job is re-pended for retry, so it must still suppress a poll.
	if err := q.MarkJobFailed(ctx, backfillID, "retry", 0); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if id, err := q.EnqueueIngestionJob(ctx, chID, "poll"); err != nil || id != "" {
		t.Fatalf("poll must stay suppressed while backfill retry pending: id=%q err=%v", id, err)
	}
}

// Completed/dead-lettered jobs must not block a fresh job of either type.
func TestEnqueueIngestionJob_AllowsAfterTerminalState(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	_, chID := seedChannel(t, ctx, q, "xterminal")

	backfillID, err := q.EnqueueIngestionJob(ctx, chID, "backfill")
	if err != nil || backfillID == "" {
		t.Fatalf("backfill enqueue: id=%q err=%v", backfillID, err)
	}
	if err := q.MarkJobCompleted(ctx, backfillID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	pollID, err := q.EnqueueIngestionJob(ctx, chID, "poll")
	if err != nil || pollID == "" {
		t.Fatalf("poll after completion want id: id=%q err=%v", pollID, err)
	}
	if err := q.MarkJobDeadLettered(ctx, pollID, "dead"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}
	backfill2, err := q.EnqueueIngestionJob(ctx, chID, "backfill")
	if err != nil || backfill2 == "" {
		t.Fatalf("backfill after dead-letter want id: id=%q err=%v", backfill2, err)
	}
	if n := countActiveIngestionJobs(t, ctx, q, chID); n != 1 {
		t.Fatalf("want 1 active after terminal transitions, got %d", n)
	}
}

// Suppression is scoped per channel: two channels may each hold an active job.
func TestEnqueueIngestionJob_DifferentChannelsRunConcurrently(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	_, ch1 := seedChannel(t, ctx, q, "xchan1")
	_, ch2 := seedChannel(t, ctx, q, "xchan2")

	bf1, err := q.EnqueueIngestionJob(ctx, ch1, "backfill")
	if err != nil || bf1 == "" {
		t.Fatalf("ch1 backfill: id=%q err=%v", bf1, err)
	}
	bf2, err := q.EnqueueIngestionJob(ctx, ch2, "backfill")
	if err != nil || bf2 == "" {
		t.Fatalf("ch2 backfill must not be suppressed: id=%q err=%v", bf2, err)
	}
	if n := countActiveIngestionJobs(t, ctx, q, ch1); n != 1 {
		t.Fatalf("ch1 want 1 active, got %d", n)
	}
	if n := countActiveIngestionJobs(t, ctx, q, ch2); n != 1 {
		t.Fatalf("ch2 want 1 active, got %d", n)
	}
}

func llmJobStatus(t *testing.T, ctx context.Context, q *db.SQLiteQuery, id string) string {
	t.Helper()
	var status string
	if err := q.DB().QueryRowContext(ctx, `SELECT status FROM llm_processing_jobs WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("read job %s status: %v", id, err)
	}
	return status
}

func TestRecoverStaleLLMJobs(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	jobID, err := q.EnqueueLLMJob(ctx, []string{"c1"}, "p")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := q.MarkLLMJobProcessing(ctx, jobID); err != nil {
		t.Fatalf("processing: %v", err)
	}
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := q.DB().ExecContext(ctx, `UPDATE llm_processing_jobs SET started_at=? WHERE id=?`, old, jobID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	n, err := q.RecoverStaleLLMJobs(ctx, 15*time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("recover want 1 got %d err %v", n, err)
	}
	if got := llmJobStatus(t, ctx, q, jobID); got != "failed" {
		t.Fatalf("stale processing job status = %q, want failed", got)
	}
	var completedAt, errMsg sql.NullString
	if err := q.DB().QueryRowContext(ctx,
		`SELECT completed_at, error_message FROM llm_processing_jobs WHERE id = ?`, jobID).
		Scan(&completedAt, &errMsg); err != nil {
		t.Fatalf("read failed job fields: %v", err)
	}
	if !completedAt.Valid || completedAt.String == "" {
		t.Errorf("failed job must record completed_at")
	}
	if !errMsg.Valid || errMsg.String == "" {
		t.Errorf("failed job must record an error_message")
	}
}

// A pending orphan (crash between enqueue and MarkLLMJobProcessing) must be
// reaped, otherwise its comment set is excluded by the claim query forever.
func TestRecoverStaleLLMJobs_FailsStalePendingAndClearsWedge(t *testing.T) {
	ctx, q, chID := retryDB(t, "reap-pending")
	commentID := insertRetryComment(t, ctx, q, chID, "stale-pending")

	jobID, created, err := q.EnqueueLLMJobIfNotExists(ctx, []string{commentID}, "p")
	if err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}
	if got := llmJobStatus(t, ctx, q, jobID); got != "pending" {
		t.Fatalf("fresh job status = %q, want pending", got)
	}
	if contains(claimedIDs(t, ctx, q, chID, 3), commentID) {
		t.Fatalf("pending-orphan comment must be excluded before reaping")
	}

	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := q.DB().ExecContext(ctx,
		`UPDATE llm_processing_jobs SET created_at=? WHERE id=?`, old, jobID); err != nil {
		t.Fatalf("backdate created_at: %v", err)
	}

	n, err := q.RecoverStaleLLMJobs(ctx, 10*time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("recover want 1 got %d err %v", n, err)
	}
	if got := llmJobStatus(t, ctx, q, jobID); got != "failed" {
		t.Fatalf("stale pending job status = %q, want failed", got)
	}
	if !contains(claimedIDs(t, ctx, q, chID, 3), commentID) {
		t.Fatalf("wedge not cleared: comment still excluded after reaping pending orphan")
	}

	// The identical payload must be enqueueable again as a fresh job.
	rejobID, created2, err := q.EnqueueLLMJobIfNotExists(ctx, []string{commentID}, "p")
	if err != nil || !created2 {
		t.Fatalf("re-enqueue want created: created=%v err=%v", created2, err)
	}
	if rejobID == jobID {
		t.Fatalf("re-enqueue returned the failed job id %q", rejobID)
	}
	if got := llmJobStatus(t, ctx, q, rejobID); got != "pending" {
		t.Fatalf("re-enqueued job status = %q, want pending", got)
	}
}

// A pending job that was just created is about to be marked processing by a
// live worker and must not be reaped by the periodic sweep.
func TestRecoverStaleLLMJobs_KeepsFreshPending(t *testing.T) {
	ctx, q, chID := retryDB(t, "reap-fresh")
	commentID := insertRetryComment(t, ctx, q, chID, "fresh-pending")

	jobID, created, err := q.EnqueueLLMJobIfNotExists(ctx, []string{commentID}, "p")
	if err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}

	n, err := q.RecoverStaleLLMJobs(ctx, 10*time.Minute)
	if err != nil || n != 0 {
		t.Fatalf("fresh pending must not be reaped: got n=%d err=%v", n, err)
	}
	if got := llmJobStatus(t, ctx, q, jobID); got != "pending" {
		t.Fatalf("fresh pending job status = %q, want pending", got)
	}
	if contains(claimedIDs(t, ctx, q, chID, 3), commentID) {
		t.Fatalf("live pending job's comment must stay excluded")
	}
}

// olderThan == 0 is the post-restart boot sweep: no in-flight work can exist,
// so every pending/processing row is failed regardless of age.
func TestRecoverStaleLLMJobs_ZeroAgeReapsPendingAndProcessing(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	pendingID, err := q.EnqueueLLMJob(ctx, []string{"p-pending"}, "p")
	if err != nil {
		t.Fatalf("enqueue pending: %v", err)
	}
	processingID, err := q.EnqueueLLMJob(ctx, []string{"p-processing"}, "p")
	if err != nil {
		t.Fatalf("enqueue processing: %v", err)
	}
	if err := q.MarkLLMJobProcessing(ctx, processingID); err != nil {
		t.Fatalf("mark processing: %v", err)
	}

	n, err := q.RecoverStaleLLMJobs(ctx, 0)
	if err != nil || n != 2 {
		t.Fatalf("zero-age recover want 2 got %d err %v", n, err)
	}
	if got := llmJobStatus(t, ctx, q, pendingID); got != "failed" {
		t.Fatalf("pending job status = %q, want failed", got)
	}
	if got := llmJobStatus(t, ctx, q, processingID); got != "failed" {
		t.Fatalf("processing job status = %q, want failed", got)
	}
}

func TestEnqueueLLMJobDedupe(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	ids := []string{"c1", "c2"}
	id1, created1, err := q.EnqueueLLMJobIfNotExists(ctx, ids, "p")
	if err != nil || !created1 {
		t.Fatalf("first want created: %v %v", id1, err)
	}
	id2, created2, err := q.EnqueueLLMJobIfNotExists(ctx, ids, "p")
	if err != nil || created2 || id2 != id1 {
		t.Fatalf("second want deduped same id: %q created=%v err=%v", id2, created2, err)
	}
}

func TestGetIngestionTokenRowPrefersUnexpired(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	u1, err := q.CreateUser(ctx, fmt.Sprintf("tok1_%d@example.com", time.Now().UnixNano()), "h")
	if err != nil {
		t.Fatalf("user1: %v", err)
	}
	u2, err := q.CreateUser(ctx, fmt.Sprintf("tok2_%d@example.com", time.Now().UnixNano()), "h")
	if err != nil {
		t.Fatalf("user2: %v", err)
	}
	chID, _, err := q.UpsertPlatformChannel(ctx, "youtube", "yt-tok-"+u1.ID[:8], "C")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	expired := time.Now().UTC().Add(-time.Hour)
	fresh := time.Now().UTC().Add(time.Hour)
	if _, err := q.CreateUserChannelTracking(ctx, u1.ID, chID, "a1", "r1", expired, 90, 0); err != nil {
		t.Fatalf("primary expired: %v", err)
	}
	if _, err := q.CreateUserChannelTracking(ctx, u2.ID, chID, "a2", "r2", fresh, 90, 1); err != nil {
		t.Fatalf("fallback: %v", err)
	}
	row, ok, err := q.GetIngestionTokenRow(ctx, chID)
	if err != nil || !ok {
		t.Fatalf("GetIngestionTokenRow: %v %v", ok, err)
	}
	if row.UserID != u2.ID {
		t.Fatalf("want fallback u2 got %s", row.UserID)
	}
}

func TestClaimPendingEmailsDisjoint(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	now := time.Now().UTC()
	old := now.Add(-2 * time.Minute)
	for i := 0; i < 4; i++ {
		_, err := q.LogEmailAttempt(ctx, db.EmailAttemptInput{
			Template: "t", IdempotencyKey: fmt.Sprintf("k-%d-%d", time.Now().UnixNano(), i),
			Status: "pending", AttemptCount: 1, LastAttemptedAt: &old,
			ToAddress: "a@b.c", Subject: "s", HTMLBody: "h", TextBody: "t",
		})
		if err != nil {
			t.Fatalf("log: %v", err)
		}
	}
	var wg sync.WaitGroup
	results := make([][]*db.EmailLogRow, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rows, err := q.ClaimPendingEmails(ctx, now, 2)
			results[idx] = rows
			errs[idx] = err
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("claim err: %v", err)
		}
	}
	seen := map[string]int{}
	for _, rows := range results {
		for _, r := range rows {
			seen[r.ID]++
		}
	}
	for id, c := range seen {
		if c > 1 {
			t.Fatalf("row %s claimed %d times", id, c)
		}
	}
	if len(seen) == 0 {
		t.Fatalf("no rows claimed")
	}
}

func TestMarkJobFailedJitterVaries(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	_, chID := seedChannel(t, ctx, q, "jit")
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		jobID, err := q.EnqueueIngestionJob(ctx, chID, fmt.Sprintf("jt-%d-%d", i, time.Now().UnixNano()))
		if err != nil || jobID == "" {
			// dedupe gate may skip; force direct insert
			continue
		}
		if err := q.MarkJobFailed(ctx, jobID, "e", 2); err != nil {
			t.Fatalf("failed: %v", err)
		}
		var sched string
		if err := q.DB().QueryRowContext(ctx, `SELECT scheduled_at FROM ingestion_jobs WHERE id=?`, jobID).Scan(&sched); err != nil {
			t.Fatalf("read sched: %v", err)
		}
		seen[sched] = true
		_, _ = q.DB().ExecContext(ctx, `DELETE FROM ingestion_jobs WHERE id=?`, jobID)
	}
	if len(seen) < 2 {
		t.Fatalf("want jitter to vary scheduled_at, got %d distinct", len(seen))
	}
}

func TestChannelCap_AtomicEnforcement(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	u, err := q.CreateUser(ctx, fmt.Sprintf("cap_%d@example.com", time.Now().UnixNano()), "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	ch1, _, err := q.UpsertPlatformChannel(ctx, "youtube", fmt.Sprintf("yt-cap-1-%d", time.Now().UnixNano()), "C")
	if err != nil {
		t.Fatalf("upsert1: %v", err)
	}
	ch2, _, err := q.UpsertPlatformChannel(ctx, "youtube", fmt.Sprintf("yt-cap-2-%d", time.Now().UnixNano()), "C")
	if err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	exp := time.Now().UTC().Add(time.Hour)
	if _, err := q.CreateUserChannelTrackingWithLimit(ctx, u.ID, ch1, "a", "r", exp, 90, 0, 1); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := q.CreateUserChannelTrackingWithLimit(ctx, u.ID, ch2, "a", "r", exp, 90, 0, 1); !errors.Is(err, db.ErrChannelLimitExceeded) {
		t.Fatalf("second want ErrChannelLimitExceeded got %v", err)
	}
}

func TestChannelCap_ConcurrentInserts(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	u, err := q.CreateUser(ctx, fmt.Sprintf("capc_%d@example.com", time.Now().UnixNano()), "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	chIDs := make([]string, 4)
	for i := range chIDs {
		ch, _, err := q.UpsertPlatformChannel(ctx, "youtube", fmt.Sprintf("yt-capc-%d-%d", time.Now().UnixNano(), i), "C")
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}
		chIDs[i] = ch
	}
	exp := time.Now().UTC().Add(time.Hour)
	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	limited := 0
	for _, chID := range chIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := q.CreateUserChannelTrackingWithLimit(ctx, u.ID, id, "a", "r", exp, 90, 0, 2)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				succeeded++
			} else if errors.Is(err, db.ErrChannelLimitExceeded) {
				limited++
			} else {
				t.Errorf("unexpected err: %v", err)
			}
		}(chID)
	}
	wg.Wait()
	if succeeded != 2 || limited != 2 {
		t.Fatalf("want 2 succeeded 2 limited got %d %d", succeeded, limited)
	}
}

func TestUpsertPlatformChannel_InsertedFlag(t *testing.T) {
	ctx, q := setupRecoveryDB(t)
	ytID := fmt.Sprintf("yt-cap-flag-%d", time.Now().UnixNano())
	id1, inserted1, err := q.UpsertPlatformChannel(ctx, "youtube", ytID, "C")
	if err != nil || !inserted1 {
		t.Fatalf("first want inserted got %v %v", inserted1, err)
	}
	id2, inserted2, err := q.UpsertPlatformChannel(ctx, "youtube", ytID, "C")
	if err != nil || inserted2 || id2 != id1 {
		t.Fatalf("second want !inserted same id got %q %v %v", id2, inserted2, err)
	}
}
