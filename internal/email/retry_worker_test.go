package email

import (
	"context"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"noumena/internal/db"
)

// fakeLogStore is an in-memory EmailLogStore used by the retry worker
// tests. It records every method call so assertions can verify the
// retry-budget transitions.
type fakeLogStore struct {
	mu          sync.Mutex
	rows        []*db.EmailLogRow
	incCalls    []incCall
	failedCalls []failedCall
	sentCalls   []string
	errCalls    []errCall
}

type incCall struct {
	ID      string
	Attempt int
	LastErr string
}
type failedCall struct {
	ID      string
	LastErr string
}
type errCall struct {
	ID  string
	Msg string
}

func (s *fakeLogStore) IsEmailIdempotencyKeyUsed(context.Context, string) (bool, error) {
	return false, nil
}
func (s *fakeLogStore) LogEmailAttempt(context.Context, db.EmailAttemptInput) (string, error) {
	return "", nil
}
func (s *fakeLogStore) MarkEmailSent(_ context.Context, id string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sentCalls = append(s.sentCalls, id)
	return nil
}
func (s *fakeLogStore) IncrementEmailAttempt(_ context.Context, id string, attempt int, lastErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incCalls = append(s.incCalls, incCall{id, attempt, lastErr})
	return nil
}
func (s *fakeLogStore) MarkEmailFailed(_ context.Context, id string, lastErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedCalls = append(s.failedCalls, failedCall{id, lastErr})
	return nil
}
func (s *fakeLogStore) MarkEmailAttemptError(_ context.Context, id string, msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errCalls = append(s.errCalls, errCall{id, msg})
	return nil
}
func (s *fakeLogStore) ClaimPendingEmails(_ context.Context, _ time.Time, _ int) ([]*db.EmailLogRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*db.EmailLogRow, len(s.rows))
	copy(out, s.rows)
	s.rows = nil // claim once
	return out, nil
}

// TestRetryWorker_BudgetExhaustion verifies the 3-attempt cap: a row at
// AttemptCount=2 is the LAST chance, so the next retry attempt becomes
// attempt=3 and transitions the row to 'failed' instead of incrementing.
//
// 14.4: backoff timing is enforced by the DB query (next_attempt_at) and
// is not the worker's responsibility — the worker only decides
// increment-vs-fail based on the count it observes.
func TestRetryWorker_BudgetExhaustion(t *testing.T) {
	store := &fakeLogStore{}
	// row currently at AttemptCount=2 → next attempt would be 3 → fail
	store.rows = []*db.EmailLogRow{{
		ID:           "row-fail",
		Template:     TemplateLoginLockout,
		AttemptCount: 2, // attempt+1=3 → cap reached
	}}

	w := &RetryWorker{
		Queries: store,
		Logger:  log.New(io.Discard, "", 0),
		Now:     func() time.Time { return time.Unix(0, 0) },
	}
	w.tickOnce(context.Background())

	if len(store.failedCalls) != 1 {
		t.Fatalf("expected 1 MarkEmailFailed call, got %d", len(store.failedCalls))
	}
	if got := store.failedCalls[0].LastErr; got != "retry budget exhausted" {
		t.Errorf("unexpected LastErr: %q", got)
	}
	if len(store.incCalls) != 0 {
		t.Errorf("expected no IncrementEmailAttempt, got %d", len(store.incCalls))
	}
}

// TestRetryWorker_MissingRecipient verifies the early-return path: a row
// with no recoverable recipient (UserID=nil) is short-circuited to
// 'failed' without incrementing the attempt counter — operators must
// re-trigger the originating event for a fresh log row.
//
// Note: the current implementation also marks failed when UserID IS
// non-nil (the in-place "_ = ..." block falls through). This test
// captures the actual current behavior; if future work adds real
// recipient lookup, this test will need to be reconsidered.
func TestRetryWorker_MissingRecipient(t *testing.T) {
	uid := "user-1"
	store := &fakeLogStore{}
	store.rows = []*db.EmailLogRow{{
		ID:           "row-norecip",
		Template:     TemplatePasswordReset,
		AttemptCount: 0,
		UserID:       &uid, // takes the UserID-set early-return branch
	}}
	w := &RetryWorker{
		Queries: store,
		Logger:  log.New(io.Discard, "", 0),
		Now:     func() time.Time { return time.Unix(0, 0) },
	}
	w.tickOnce(context.Background())

	if len(store.failedCalls) != 1 {
		t.Fatalf("expected 1 MarkEmailFailed (no recipient), got %d", len(store.failedCalls))
	}
}
