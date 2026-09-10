package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// retryDB provisions a migrated in-memory DB and a channel with a primary
// tracker, returning the query handle and the platform_channel_id.
func retryDB(t *testing.T, tag string) (context.Context, *db.SQLiteQuery, string) {
	t.Helper()
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	_, chID := seedChannel(t, ctx, q, tag)
	return ctx, q, chID
}

func insertRetryComment(t *testing.T, ctx context.Context, q *db.SQLiteQuery, chID, suffix string) string {
	t.Helper()
	c := &db.Comment{
		PlatformChannelID: chID,
		PlatformCommentID: "pc-" + suffix,
		Body:              "comment " + suffix,
		PostedAt:          time.Now(),
	}
	if err := q.InsertComment(ctx, c); err != nil {
		t.Fatalf("InsertComment: %v", err)
	}
	var id string
	if err := q.DB().QueryRowContext(ctx,
		`SELECT id FROM comments WHERE platform_channel_id = ? AND platform_comment_id = ?`,
		chID, "pc-"+suffix).Scan(&id); err != nil {
		t.Fatalf("select comment id: %v", err)
	}
	return id
}

func claimedIDs(t *testing.T, ctx context.Context, q *db.SQLiteQuery, chID string, maxAttempts int) []string {
	t.Helper()
	grouped, err := q.GetUnprocessedCommentsByChannel(ctx, 100, maxAttempts)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	ids := []string{}
	for _, c := range grouped[chID] {
		ids = append(ids, c.ID)
	}
	return ids
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestClaim_ExcludesParkedAndBackedOff(t *testing.T) {
	ctx, q, chID := retryDB(t, "retry-claim")
	eligible := insertRetryComment(t, ctx, q, chID, "eligible")
	parked := insertRetryComment(t, ctx, q, chID, "parked")
	backedOff := insertRetryComment(t, ctx, q, chID, "backoff")

	// Parked: at the cap.
	if _, err := q.DB().ExecContext(ctx, `UPDATE comments SET llm_attempts = 3 WHERE id = ?`, parked); err != nil {
		t.Fatalf("park: %v", err)
	}
	// Backed off: future next_retry_at.
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := q.DB().ExecContext(ctx, `UPDATE comments SET llm_attempts = 1, llm_next_retry_at = ? WHERE id = ?`, future, backedOff); err != nil {
		t.Fatalf("backoff: %v", err)
	}

	got := claimedIDs(t, ctx, q, chID, 3)
	if !contains(got, eligible) {
		t.Errorf("fresh comment should be claimable; got %v", got)
	}
	if contains(got, parked) {
		t.Errorf("parked comment must not be claimed; got %v", got)
	}
	if contains(got, backedOff) {
		t.Errorf("backed-off comment must not be claimed; got %v", got)
	}
}

func TestClaim_RequeuedParkedCommentVisibleAgain(t *testing.T) {
	ctx, q, chID := retryDB(t, "retry-requeue")
	id := insertRetryComment(t, ctx, q, chID, "requeue")
	if _, err := q.DB().ExecContext(ctx, `UPDATE comments SET llm_attempts = 3 WHERE id = ?`, id); err != nil {
		t.Fatalf("park: %v", err)
	}
	if contains(claimedIDs(t, ctx, q, chID, 3), id) {
		t.Fatalf("parked comment should be excluded before requeue")
	}

	// Operator un-park: reset attempts and clear the backoff timestamp.
	if _, err := q.DB().ExecContext(ctx, `UPDATE comments SET llm_attempts = 0, llm_next_retry_at = NULL WHERE id = ?`, id); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if !contains(claimedIDs(t, ctx, q, chID, 3), id) {
		t.Errorf("requeued comment should be claimable again")
	}
}

func TestIncrementLLMAttempts_TransientAndPark(t *testing.T) {
	ctx, q, chID := retryDB(t, "retry-inc")
	id := insertRetryComment(t, ctx, q, chID, "inc")

	next := time.Now().UTC().Add(5 * time.Minute)
	if err := q.IncrementLLMAttempts(ctx, []string{id}, &next, false, 3); err != nil {
		t.Fatalf("increment: %v", err)
	}
	var attempts int
	var nextRetry *string
	if err := q.DB().QueryRowContext(ctx, `SELECT llm_attempts, llm_next_retry_at FROM comments WHERE id = ?`, id).Scan(&attempts, &nextRetry); err != nil {
		t.Fatalf("read: %v", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if nextRetry == nil || *nextRetry == "" {
		t.Errorf("next_retry_at should be set")
	}

	// Terminal park sets attempts to the cap and clears the timestamp.
	if err := q.IncrementLLMAttempts(ctx, []string{id}, nil, true, 3); err != nil {
		t.Fatalf("park: %v", err)
	}
	if err := q.DB().QueryRowContext(ctx, `SELECT llm_attempts, llm_next_retry_at FROM comments WHERE id = ?`, id).Scan(&attempts, &nextRetry); err != nil {
		t.Fatalf("read park: %v", err)
	}
	if attempts != 3 {
		t.Errorf("parked attempts = %d, want 3", attempts)
	}
	if nextRetry != nil && *nextRetry != "" {
		t.Errorf("parked next_retry_at should be NULL, got %q", *nextRetry)
	}
}

func TestCountParkedComments(t *testing.T) {
	ctx, q, chID := retryDB(t, "retry-count")
	a := insertRetryComment(t, ctx, q, chID, "count-a")
	b := insertRetryComment(t, ctx, q, chID, "count-b")
	insertRetryComment(t, ctx, q, chID, "count-c")

	if _, err := q.DB().ExecContext(ctx, `UPDATE comments SET llm_attempts = 3 WHERE id IN (?, ?)`, a, b); err != nil {
		t.Fatalf("park: %v", err)
	}
	n, err := q.CountParkedComments(ctx, 3)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("parked = %d, want 2", n)
	}
}

// A comment reserved by a pending/processing llm_processing_jobs row is
// excluded from the claim so a continuous claimer can never reclaim work that
// is still in flight. Completing the job releases the exclusion.
func TestClaim_ExcludesInFlightJobs(t *testing.T) {
	ctx, q, chID := retryDB(t, "retry-inflight")
	inFlight := insertRetryComment(t, ctx, q, chID, "inflight")
	eligible := insertRetryComment(t, ctx, q, chID, "eligible")

	jobID, created, err := q.EnqueueLLMJobIfNotExists(ctx, []string{inFlight}, "test")
	if err != nil || !created {
		t.Fatalf("enqueue: created=%v err=%v", created, err)
	}
	if err := q.MarkLLMJobProcessing(ctx, jobID); err != nil {
		t.Fatalf("mark processing: %v", err)
	}

	got := claimedIDs(t, ctx, q, chID, 3)
	if contains(got, inFlight) {
		t.Errorf("in-flight comment must be excluded; got %v", got)
	}
	if !contains(got, eligible) {
		t.Errorf("eligible comment must be claimable; got %v", got)
	}

	// A terminal job no longer shadows the comment (it will have been
	// persisted; if not, it is retried).
	if err := q.MarkLLMJobCompleted(ctx, jobID); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	if !contains(claimedIDs(t, ctx, q, chID, 3), inFlight) {
		t.Errorf("completed-job comment should be claimable again")
	}
}

// A reservation row whose comment_ids is JSON null (or otherwise yields a NULL
// element) must not suppress the entire claim. `NOT IN (subquery)` yields NULL
// when the subquery emits a NULL, which would filter out every comment.
func TestClaim_NullJobCommentIdsDoesNotSuppressClaims(t *testing.T) {
	ctx, q, chID := retryDB(t, "retry-nulljob")
	eligible := insertRetryComment(t, ctx, q, chID, "eligible-null")
	if _, err := q.DB().ExecContext(ctx,
		`INSERT INTO llm_processing_jobs (id, status, comment_ids) VALUES ('legacy-null', 'processing', 'null')`); err != nil {
		t.Fatalf("insert null job: %v", err)
	}
	got := claimedIDs(t, ctx, q, chID, 3)
	if !contains(got, eligible) {
		t.Fatalf("a JSON-null comment_ids job must not suppress claims; got %v", got)
	}
}

// Resuming a paused primary must restore a token_priority=0 tracker, otherwise
// the claim query (which requires one) stops processing the channel forever.
func TestResume_RestoresPrimaryAndClaimVisibility(t *testing.T) {
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)

	primaryUser, chID := seedChannel(t, ctx, q, "resume-primary") // token_priority 0
	secondary, err := q.CreateUser(ctx, fmt.Sprintf("resume-secondary-%d@example.com", time.Now().UnixNano()), "h")
	if err != nil {
		t.Fatalf("CreateUser secondary: %v", err)
	}
	if _, err := q.CreateUserChannelTracking(ctx, secondary.ID, chID, "a2", "r2", time.Now().UTC().Add(time.Hour), 90, 1); err != nil {
		t.Fatalf("CreateUserChannelTracking secondary: %v", err)
	}
	id := insertRetryComment(t, ctx, q, chID, "resume")

	if err := q.PauseTracking(ctx, primaryUser, chID); err != nil {
		t.Fatalf("PauseTracking: %v", err)
	}
	if err := q.ResumeTracking(ctx, primaryUser, chID); err != nil {
		t.Fatalf("ResumeTracking: %v", err)
	}

	var primaries int
	if err := q.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_channel_tracking
		 WHERE platform_channel_id = ? AND tracking_status = 'active' AND token_priority = 0`,
		chID).Scan(&primaries); err != nil {
		t.Fatalf("count primaries: %v", err)
	}
	if primaries != 1 {
		t.Fatalf("active priority-0 trackers = %d, want 1", primaries)
	}
	if !contains(claimedIDs(t, ctx, q, chID, 3), id) {
		t.Fatalf("resumed channel's comments must be claimable")
	}
}
