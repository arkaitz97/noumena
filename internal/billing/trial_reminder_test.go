package billing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"testing"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/dbtest"
	"noumena/internal/email"
)

// Tests for the trial-expiry reminder cron (trial_reminder.go, task 5.5).
//
// runTrialReminderOnce takes a concrete *email.Pipeline, so the fakes
// below implement the pipeline's two seams — db.EmailLogStore (captures
// the durable enqueue as LogEmailAttempt) and email.SMTPSender (returns
// nil so the send path completes) — rather than mocking Send itself.

type fakeTrialStore struct {
	users []db.TrialReminderUser
	err   error
	calls int
	lo    time.Time
	hi    time.Time
}

func (f *fakeTrialStore) ListUsersForTrialExpiryReminder(ctx context.Context, lo, hi time.Time) ([]db.TrialReminderUser, error) {
	f.calls++
	f.lo, f.hi = lo, hi
	if f.err != nil {
		return nil, f.err
	}
	return f.users, nil
}

type fakeEmailLogStore struct {
	attempts []db.EmailAttemptInput
}

func (s *fakeEmailLogStore) IsEmailIdempotencyKeyUsed(context.Context, string) (bool, error) {
	return false, nil
}
func (s *fakeEmailLogStore) LogEmailAttempt(_ context.Context, in db.EmailAttemptInput) (string, error) {
	s.attempts = append(s.attempts, in)
	return fmt.Sprintf("row-%d", len(s.attempts)), nil
}
func (s *fakeEmailLogStore) MarkEmailSent(context.Context, string, time.Time) error { return nil }
func (s *fakeEmailLogStore) IncrementEmailAttempt(context.Context, string, int, string) error {
	return nil
}
func (s *fakeEmailLogStore) MarkEmailFailed(context.Context, string, string) error { return nil }
func (s *fakeEmailLogStore) MarkEmailAttemptError(context.Context, string, string) error {
	return nil
}
func (s *fakeEmailLogStore) ClaimPendingEmails(context.Context, time.Time, int) ([]*db.EmailLogRow, error) {
	return nil, nil
}
func (s *fakeEmailLogStore) GetEmailRowByIdempotencyKey(context.Context, string) (*db.EmailLogRow, error) {
	return nil, nil
}

type fakeSender struct{}

func (fakeSender) Send(context.Context, string, string, string, string, string, []string) error {
	return nil
}

func newTestPipeline(t *testing.T, store *fakeEmailLogStore) (*email.Pipeline, *fakeEmailLogStore) {
	t.Helper()
	if store == nil {
		store = &fakeEmailLogStore{}
	}
	p := &email.Pipeline{
		Queries: store,
		Sender:  fakeSender{},
		Cfg: &config.Config{
			EmailEnabled:     true,
			EmailFromAddress: "billing@noumena.test",
			EmailFromName:    "Noumena",
		},
		Logger: log.New(io.Discard, "", 0),
	}
	return p, store
}

// daysFromSubject pulls the numeric DaysRemaining out of the rendered
// subject "Your trial ends in N days".
func daysFromSubject(t *testing.T, subject string) int {
	t.Helper()
	fields := strings.Fields(subject)
	if len(fields) < 5 {
		t.Fatalf("unexpected subject %q", subject)
	}
	d, err := strconv.Atoi(fields[4])
	if err != nil {
		t.Fatalf("parse days from subject %q: %v", subject, err)
	}
	return d
}

func user(id, email string, endsAt time.Time) db.TrialReminderUser {
	return db.TrialReminderUser{ID: id, Email: email, TrialEndsAt: endsAt}
}

func TestTrialReminderDispatch_InWindowUser_SendsExactlyOne(t *testing.T) {
	store := &fakeTrialStore{users: []db.TrialReminderUser{
		user("u-1", "alice@example.com", time.Now().UTC().Add(48*time.Hour)),
	}}
	p, rec := newTestPipeline(t, nil)

	runTrialReminderOnce(context.Background(), store, p, 72*time.Hour)

	if len(rec.attempts) != 1 {
		t.Fatalf("send count: want 1 got %d", len(rec.attempts))
	}
	got := rec.attempts[0]
	if got.Template != email.TemplateTrialExpiry {
		t.Errorf("template: want %q got %q", email.TemplateTrialExpiry, got.Template)
	}
	if got.ToAddress != "alice@example.com" {
		t.Errorf("to_address: want alice@example.com got %q", got.ToAddress)
	}
	if got.UserID == nil || *got.UserID != "u-1" {
		t.Errorf("user_id: want u-1 got %v", got.UserID)
	}
	wantKey := "trial_expiry:u-1:" + time.Now().UTC().Format("2006-01-02")
	if got.IdempotencyKey != wantKey {
		t.Errorf("idempotency_key: want %q got %q", wantKey, got.IdempotencyKey)
	}
	if days := daysFromSubject(t, got.Subject); days != 2 {
		t.Errorf("DaysRemaining rendered: want 2 got %d", days)
	}
	if !strings.Contains(got.HTMLBody, "trial of Creator") {
		t.Errorf("html missing PlanName=Creator: %q", got.HTMLBody)
	}
	if !strings.Contains(got.HTMLBody, `href=""`) {
		t.Errorf("html missing empty UpgradeURL: %q", got.HTMLBody)
	}

	if store.calls != 1 {
		t.Errorf("store calls: want 1 got %d", store.calls)
	}
	if got := store.hi.Sub(store.lo); got != 24*time.Hour {
		t.Errorf("window width: want 24h got %v", got)
	}
	lead := store.hi.Sub(time.Now())
	if lead < 70*time.Hour || lead > 74*time.Hour {
		t.Errorf("hi not ~now+72h: %v", lead)
	}
}

func TestTrialReminderDispatch_DaysRemainingVariants(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeTrialStore{users: []db.TrialReminderUser{
		user("u-1", "soon@example.com", now.Add(48*time.Hour)),
		user("u-2", "later@example.com", now.Add(120*time.Hour)),
		user("u-3", "expired@example.com", now.Add(-48*time.Hour)),
	}}
	p, rec := newTestPipeline(t, nil)

	runTrialReminderOnce(context.Background(), store, p, 72*time.Hour)

	if len(rec.attempts) != 3 {
		t.Fatalf("send count: want 3 got %d", len(rec.attempts))
	}
	want := map[string]int{
		"soon@example.com":    2, // ~48h -> int(47.99/24)+1
		"later@example.com":   5, // ~120h -> int(119.99/24)+1
		"expired@example.com": 0, // -48h -> int(-2)+1 = -1, floored to 0
	}
	for _, a := range rec.attempts {
		if got := daysFromSubject(t, a.Subject); got != want[a.ToAddress] {
			t.Errorf("DaysRemaining for %s: want %d got %d (subject %q)",
				a.ToAddress, want[a.ToAddress], got, a.Subject)
		}
	}
}

func TestTrialReminderDispatch_NoUsers_NoEmail(t *testing.T) {
	store := &fakeTrialStore{}
	p, rec := newTestPipeline(t, nil)

	runTrialReminderOnce(context.Background(), store, p, 72*time.Hour)

	if len(rec.attempts) != 0 {
		t.Errorf("send count: want 0 got %d", len(rec.attempts))
	}
	if store.calls != 1 {
		t.Errorf("store calls: want 1 got %d", store.calls)
	}
}

func TestTrialReminderDispatch_StoreError_NoSendNoCrash(t *testing.T) {
	store := &fakeTrialStore{
		users: []db.TrialReminderUser{user("u-1", "a@example.com", time.Now().UTC().Add(48*time.Hour))},
		err:   errors.New("db boom"),
	}
	p, rec := newTestPipeline(t, nil)

	runTrialReminderOnce(context.Background(), store, p, 72*time.Hour)

	if len(rec.attempts) != 0 {
		t.Errorf("send count on store error: want 0 got %d", len(rec.attempts))
	}
}

func TestTrialReminderDispatch_IdempotencyKey_StableAcrossRuns(t *testing.T) {
	store := &fakeTrialStore{users: []db.TrialReminderUser{
		user("u-1", "alice@example.com", time.Now().UTC().Add(48*time.Hour)),
	}}
	p, rec := newTestPipeline(t, nil)

	runTrialReminderOnce(context.Background(), store, p, 72*time.Hour)
	runTrialReminderOnce(context.Background(), store, p, 72*time.Hour)

	if len(rec.attempts) != 2 {
		t.Fatalf("send count: want 2 got %d", len(rec.attempts))
	}
	if rec.attempts[0].IdempotencyKey != rec.attempts[1].IdempotencyKey {
		t.Errorf("keys differ across same-day runs: %q vs %q",
			rec.attempts[0].IdempotencyKey, rec.attempts[1].IdempotencyKey)
	}
	wantKey := "trial_expiry:u-1:" + time.Now().UTC().Format("2006-01-02")
	if rec.attempts[0].IdempotencyKey != wantKey {
		t.Errorf("idempotency_key: want %q got %q", wantKey, rec.attempts[0].IdempotencyKey)
	}
}

func TestTrialReminderStart_NilPipelineOrNilStore_Noop(t *testing.T) {
	ctx := context.Background()
	store := &fakeTrialStore{}
	p, _ := newTestPipeline(t, nil)

	StartTrialReminder(ctx, store, nil, TrialReminderConfig{})
	if store.calls != 0 {
		t.Errorf("nil pipeline must not call store; calls=%d", store.calls)
	}

	StartTrialReminder(ctx, nil, p, TrialReminderConfig{})
}

func TestTrialReminderStart_RunsImmediatelyThenStops(t *testing.T) {
	store := &fakeTrialStore{users: []db.TrialReminderUser{
		user("u-1", "alice@example.com", time.Now().UTC().Add(48*time.Hour)),
	}}
	p, rec := newTestPipeline(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	StartTrialReminder(ctx, store, p, TrialReminderConfig{Interval: time.Hour})
	time.Sleep(30 * time.Millisecond)
	cancel()

	if store.calls < 1 {
		t.Errorf("expected at least one cron run, got %d", store.calls)
	}
	if len(rec.attempts) != 1 {
		t.Errorf("send count: want 1 got %d", len(rec.attempts))
	}
}

// TestTrialReminderWindow_Boundaries_RealStore pins the SQL window
// semantics of ListUsersForTrialExpiryReminder against a real in-memory
// DB: trial_ends_at strictly greater than lo, less than or equal to hi,
// only for non-deleted trialing users.
func TestTrialReminderWindow_Boundaries_RealStore(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)

	lo := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	hi := lo.Add(48 * time.Hour)
	mid := lo.Add(24 * time.Hour)

	insert := func(id, email string, endsAt *time.Time, status string, deletedAt *string) {
		var endsStr any
		if endsAt != nil {
			endsStr = endsAt.Format(time.RFC3339)
		}
		_, err := database.ExecContext(context.Background(),
			`INSERT INTO users (id, email, password_hash, trial_ends_at, subscription_status, deletion_requested_at)
			 VALUES (?, ?, NULL, ?, ?, ?)`,
			id, email, endsStr, status, deletedAt)
		if err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	del := "2026-03-01T00:00:00Z"
	pastHi := hi.Add(time.Hour)

	// Boundary expectations: lo excluded (strict >), hi included (<=).
	insert("u-at-lo", "at-lo@example.com", &lo, "trialing", nil)         // excluded
	insert("u-mid", "mid@example.com", &mid, "trialing", nil)            // included
	insert("u-at-hi", "at-hi@example.com", &hi, "trialing", nil)         // included
	insert("u-past-hi", "past-hi@example.com", &pastHi, "trialing", nil) // excluded
	insert("u-not-trialing", "active@example.com", &mid, "active", nil)  // excluded
	insert("u-no-trial", "notrial@example.com", nil, "trialing", nil)    // excluded
	insert("u-deleted", "deleted@example.com", &mid, "trialing", &del)   // excluded

	got, err := q.ListUsersForTrialExpiryReminder(context.Background(), lo, hi)
	if err != nil {
		t.Fatalf("ListUsersForTrialExpiryReminder: %v", err)
	}

	emails := map[string]bool{}
	for _, u := range got {
		emails[u.Email] = true
	}
	for _, want := range []string{"mid@example.com", "at-hi@example.com"} {
		if !emails[want] {
			t.Errorf("expected %s in window (lo, hi]", want)
		}
	}
	for _, banned := range []string{"at-lo@example.com", "past-hi@example.com",
		"active@example.com", "notrial@example.com", "deleted@example.com"} {
		if emails[banned] {
			t.Errorf("did not expect %s in window", banned)
		}
	}
}

// TestTrialReminderDispatch_RealStore_EndToEnd wires the real store to
// the real dispatch path: only the in-window user gets an email, with the
// correct to_address and rendered DaysRemaining.
func TestTrialReminderDispatch_RealStore_EndToEnd(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)

	now := time.Now().UTC()
	insert := func(id, email string, endsAt time.Time) {
		_, err := database.ExecContext(context.Background(),
			`INSERT INTO users (id, email, password_hash, trial_ends_at, subscription_status)
			 VALUES (?, ?, NULL, ?, 'trialing')`,
			id, email, endsAt.Format(time.RFC3339))
		if err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	insert("u-in", "in@example.com", now.Add(72*time.Hour))            // at hi: included
	insert("u-far", "far@example.com", now.Add(96*time.Hour))          // beyond hi
	insert("u-early", "early@example.com", now.Add(24*time.Hour))      // below lo
	insert("u-expired", "expired@example.com", now.Add(-24*time.Hour)) // expired

	p, rec := newTestPipeline(t, nil)
	runTrialReminderOnce(context.Background(), q, p, 72*time.Hour)

	if len(rec.attempts) != 1 {
		t.Fatalf("send count: want 1 got %d", len(rec.attempts))
	}
	got := rec.attempts[0]
	if got.ToAddress != "in@example.com" {
		t.Errorf("to_address: want in@example.com got %q", got.ToAddress)
	}
	if !strings.HasPrefix(got.IdempotencyKey, "trial_expiry:u-in:") {
		t.Errorf("idempotency_key prefix: %q", got.IdempotencyKey)
	}
	if days := daysFromSubject(t, got.Subject); days != 3 {
		t.Errorf("DaysRemaining rendered: want 3 got %d (subject %q)", days, got.Subject)
	}
}
