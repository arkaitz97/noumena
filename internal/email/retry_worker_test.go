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
func (s *fakeLogStore) GetEmailRowByIdempotencyKey(context.Context, string) (*db.EmailLogRow, error) {
	return nil, nil
}

// stubSender records every Send invocation; failErr makes it fail.
type stubSender struct {
	mu    sync.Mutex
	calls []sendCall
	fail  error
}

type sendCall struct {
	to, subject, html, text string
	headers                 []string
}

func (s *stubSender) Send(_ context.Context, _ string, to, subject, htmlBody, textBody string, headers []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, sendCall{to, subject, htmlBody, textBody, headers})
	return s.fail
}

func newRetryWorker(store *fakeLogStore, sender *stubSender) *RetryWorker {
	return &RetryWorker{
		Queries: store,
		Sender:  sender,
		Logger:  log.New(io.Discard, "", 0),
		Now:     func() time.Time { return time.Unix(0, 0) },
	}
}

// Task 1.4: a pending row with a persisted recipient + body is actually
// re-sent (stub sender), and success flips the row to sent.
func TestRetryWorker_SendsPersistedBody(t *testing.T) {
	store := &fakeLogStore{}
	store.rows = []*db.EmailLogRow{{
		ID:             "row-ok",
		Template:       TemplateWeeklyDigest,
		IdempotencyKey: "weekly_digest:dig-1",
		AttemptCount:   1,
		ToAddress:      "user@example.com",
		Subject:        "Your weekly digest",
		HTMLBody:       "<p>digest</p>",
		TextBody:       "digest\n",
	}}
	sender := &stubSender{}
	w := newRetryWorker(store, sender)
	w.tickOnce(context.Background())

	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 Send call, got %d", len(sender.calls))
	}
	c := sender.calls[0]
	if c.to != "user@example.com" || c.subject != "Your weekly digest" ||
		c.html != "<p>digest</p>" || c.text != "digest\n" {
		t.Errorf("Send got unexpected payload: %+v", c)
	}
	if len(store.sentCalls) != 1 || store.sentCalls[0] != "row-ok" {
		t.Errorf("expected MarkEmailSent(row-ok), got %v", store.sentCalls)
	}
	if len(store.failedCalls) != 0 || len(store.incCalls) != 0 {
		t.Errorf("unexpected failed/inc calls: failed=%v inc=%v", store.failedCalls, store.incCalls)
	}
}

// Task 1.5: repeated send failures exhaust the 3-attempt budget and the
// row transitions to failed (terminal metric via recordEmailSent).
func TestRetryWorker_BudgetExhaustion(t *testing.T) {
	store := &fakeLogStore{}
	// row at AttemptCount=2 → next attempt = 3 → cap reached
	store.rows = []*db.EmailLogRow{{
		ID:           "row-fail",
		Template:     TemplateLoginLockout,
		AttemptCount: 2,
		ToAddress:    "user@example.com",
		Subject:      "lockout",
		HTMLBody:     "<p>locked</p>",
		TextBody:     "locked",
	}}
	sender := &stubSender{fail: context.DeadlineExceeded}
	w := newRetryWorker(store, sender)
	w.tickOnce(context.Background())

	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 Send attempt, got %d", len(sender.calls))
	}
	if len(store.failedCalls) != 1 {
		t.Fatalf("expected 1 MarkEmailFailed call, got %d", len(store.failedCalls))
	}
	if got := store.failedCalls[0].LastErr; got != "retry budget exhausted" {
		t.Errorf("unexpected LastErr: %q", got)
	}
	if len(store.incCalls) != 0 {
		t.Errorf("expected no IncrementEmailAttempt, got %d", len(store.incCalls))
	}
	if len(store.sentCalls) != 0 {
		t.Errorf("expected no MarkEmailSent, got %v", store.sentCalls)
	}
}

// Non-terminal failure bumps the attempt count instead of failing the row.
func TestRetryWorker_DefersWithinBudget(t *testing.T) {
	store := &fakeLogStore{}
	store.rows = []*db.EmailLogRow{{
		ID:           "row-defer",
		Template:     TemplatePasswordReset,
		AttemptCount: 1,
		ToAddress:    "user@example.com",
		Subject:      "reset",
		HTMLBody:     "<p>reset</p>",
		TextBody:     "reset",
	}}
	sender := &stubSender{fail: context.DeadlineExceeded}
	w := newRetryWorker(store, sender)
	w.tickOnce(context.Background())

	if len(store.incCalls) != 1 || store.incCalls[0].Attempt != 2 {
		t.Errorf("expected IncrementEmailAttempt to attempt 2, got %v", store.incCalls)
	}
	if len(store.failedCalls) != 0 {
		t.Errorf("expected no MarkEmailFailed, got %v", store.failedCalls)
	}
}

// Legacy rows without a persisted recipient cannot be re-sent; they are
// marked failed so an operator can re-trigger the originating event.
func TestRetryWorker_MissingRecipient(t *testing.T) {
	uid := "user-1"
	store := &fakeLogStore{}
	store.rows = []*db.EmailLogRow{{
		ID:           "row-norecip",
		Template:     TemplatePasswordReset,
		AttemptCount: 0,
		UserID:       &uid,
	}}
	sender := &stubSender{}
	w := newRetryWorker(store, sender)
	w.tickOnce(context.Background())

	if len(sender.calls) != 0 {
		t.Errorf("expected no Send call, got %d", len(sender.calls))
	}
	if len(store.failedCalls) != 1 {
		t.Fatalf("expected 1 MarkEmailFailed (no recipient), got %d", len(store.failedCalls))
	}
	if got := store.failedCalls[0].LastErr; got != "retry-without-recipient: original Send dropped recipient context" {
		t.Errorf("unexpected LastErr: %q", got)
	}
}
