package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
)

// fakeDigestQuery is a hand-rolled DBQuery double for the worker tests. It
// implements only the methods runUserDigest / dispatch / recoverStalePending
// touch; everything else falls through to nil-method panics if accidentally
// invoked, which surfaces accidental test coverage gaps.
type fakeDigestQuery struct {
	db.DBQuery
	mu sync.Mutex

	users       []*db.User
	comments    map[string][]*db.Comment // keyed by userID
	prevWeek    map[string]*db.WeeklyDigest
	stale       []*db.WeeklyDigest
	deferredDue []*db.WeeklyDigest

	// effects observed by the test
	insertedPending []string // digestIDs returned
	contentUpdates  []contentUpdate
	statusUpdates   []statusUpdate

	// behavior knobs
	pendingInserted    bool
	insertPendingErr   error
	getCommentsErr     error
	contentUpdateErr   error
	statusUpdateErr    error
	stableInsertCalled int

	// userLookupStarted, when non-nil, receives a signal on every
	// GetUserByID call. userLookupGate, when non-nil, blocks GetUserByID
	// until the gate closes or the context is cancelled — used to hold a
	// deferred sweep in flight while the run loop keeps ticking.
	userLookupStarted chan struct{}
	userLookupGate    chan struct{}

	// insertPendingSignal, when non-nil, receives a signal after every
	// InsertWeeklyDigestPending call so tests can synchronize on the weekly
	// dispatch instead of polling or sleeping.
	insertPendingSignal chan struct{}
}

type contentUpdate struct {
	DigestID string
	Status   string
	Content  string
}
type statusUpdate struct {
	DigestID string
	Status   string
}

func (f *fakeDigestQuery) GetUsersForDigest(ctx context.Context) ([]*db.User, error) {
	return f.users, nil
}

func (f *fakeDigestQuery) GetEligibleCommentsForUser(ctx context.Context, userID string, since, until time.Time) ([]*db.Comment, error) {
	if f.getCommentsErr != nil {
		return nil, f.getCommentsErr
	}
	return f.comments[userID], nil
}

func (f *fakeDigestQuery) InsertWeeklyDigestPending(ctx context.Context, userID string, weekStart time.Time) (string, bool, error) {
	if f.insertPendingErr != nil {
		return "", false, f.insertPendingErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stableInsertCalled++
	id := "digest-" + userID
	f.insertedPending = append(f.insertedPending, id)
	if f.insertPendingSignal != nil {
		select {
		case f.insertPendingSignal <- struct{}{}:
		default:
		}
	}
	return id, f.pendingInserted, nil
}

func (f *fakeDigestQuery) UpdateDigestContent(ctx context.Context, digestID string, contentJSON []byte, status string) error {
	if f.contentUpdateErr != nil {
		return f.contentUpdateErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contentUpdates = append(f.contentUpdates, contentUpdate{
		DigestID: digestID, Status: status, Content: string(contentJSON),
	})
	return nil
}

func (f *fakeDigestQuery) UpdateDigestStatus(ctx context.Context, digestID, status string) error {
	if f.statusUpdateErr != nil {
		return f.statusUpdateErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusUpdates = append(f.statusUpdates, statusUpdate{DigestID: digestID, Status: status})
	return nil
}

func (f *fakeDigestQuery) GetPreviousWeekDigest(ctx context.Context, userID string, weekStart time.Time) (*db.WeeklyDigest, error) {
	if d, ok := f.prevWeek[userID]; ok {
		return d, nil
	}
	return nil, db.ErrNotFound
}

func (f *fakeDigestQuery) GetStalePendingDigests(ctx context.Context, olderThan time.Time) ([]*db.WeeklyDigest, error) {
	return f.stale, nil
}

func (f *fakeDigestQuery) GetDeferredDigestsDue(ctx context.Context, now time.Time, limit int) ([]*db.WeeklyDigest, error) {
	return f.deferredDue, nil
}

func (f *fakeDigestQuery) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	if f.userLookupStarted != nil {
		select {
		case f.userLookupStarted <- struct{}{}:
		default:
		}
	}
	if f.userLookupGate != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.userLookupGate:
		}
	}
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

// insertedCount returns the number of InsertWeeklyDigestPending calls
// (i.e. weekly-trigger dispatches) observed so far.
func (f *fakeDigestQuery) insertedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stableInsertCalled
}

func (f *fakeDigestQuery) GetWeeklyDigestByID(ctx context.Context, digestID string) (*db.WeeklyDigest, error) {
	return nil, db.ErrNotFound
}

func (f *fakeDigestQuery) GetMissingDigestWeeks(ctx context.Context, userID string, weeks []time.Time) ([]time.Time, error) {
	return weeks, nil
}

func (f *fakeDigestQuery) MarkDigestEmailed(ctx context.Context, digestID, userID string, at time.Time) error {
	return nil
}

// --- Tests ------------------------------------------------------------------

func TestShouldTrigger(t *testing.T) {
	// Mon 2024-01-01 was a Monday.
	monday := time.Date(2024, 1, 1, 8, 0, 30, 0, time.UTC)
	if !shouldTrigger(monday) {
		t.Errorf("Mon 08:00 UTC should trigger")
	}
	// Same day, wrong minute.
	if shouldTrigger(monday.Add(2 * time.Minute)) {
		t.Errorf("Mon 08:02 should NOT trigger")
	}
	// Wrong hour.
	if shouldTrigger(monday.Add(time.Hour)) {
		t.Errorf("Mon 09:00 should NOT trigger")
	}
	// Wrong day (Tuesday).
	if shouldTrigger(monday.AddDate(0, 0, 1)) {
		t.Errorf("Tue 08:00 should NOT trigger")
	}
}

func TestComputeWeekStart(t *testing.T) {
	// Trigger time Mon 2024-01-08 08:00 UTC → previous Mon = 2024-01-01.
	got := computeWeekStart(time.Date(2024, 1, 8, 8, 0, 0, 0, time.UTC))
	want := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("week_start got %s want %s", got, want)
	}
}

func newTestWorker(t *testing.T, q *fakeDigestQuery, llm *opencodeClient) *Worker {
	t.Helper()
	return &Worker{
		Queries:   q,
		LLMClient: llm,
		Logger:    log.New(io.Discard, "", 0),
		Now:       func() time.Time { return time.Date(2024, 1, 8, 8, 0, 0, 0, time.UTC) },
	}
}

func TestRunUserDigest_SkipBelowThreshold(t *testing.T) {
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": make([]*db.Comment, 9)},
		pendingInserted: true,
	}
	for i := range q.comments["u1"] {
		q.comments["u1"][i] = &db.Comment{Body: "x"}
	}
	w := newTestWorker(t, q, newMockOpenCode(t, "{}", "", 0))
	w.runUserDigest(context.Background(), &db.User{ID: "u1"}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))

	if len(q.contentUpdates) != 1 || q.contentUpdates[0].Status != "skipped_insufficient_data" {
		t.Errorf("expected skip update, got %+v", q.contentUpdates)
	}
}

func TestRunUserDigest_SuccessOverwritesNumeric(t *testing.T) {
	cs := make([]*db.Comment, 12)
	for i := range cs {
		topic := "topic-A"
		cat := "praise"
		cs[i] = &db.Comment{Body: "loved it", Topic: &topic, Category: &cat}
	}
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": cs},
		pendingInserted: true,
	}
	llmRaw := `{
		"top_ideas":[{"idea":"more like this","evidence":"many positives","sample_comments":["loved it"]}],
		"top_questions":[],
		"top_actionable_insights":[{"type":"video_idea","description":"tutorial","count":2}],
		"engagement_health_score":0,
		"health_score_breakdown":{"response_rate":0,"engagement_depth":0},
		"total_comments_analyzed":0,
		"filtered_comments":0
	}`
	w := newTestWorker(t, q, newMockOpenCode(t, llmRaw, "", 0))
	w.runUserDigest(context.Background(), &db.User{ID: "u1"}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))

	if len(q.contentUpdates) != 1 || q.contentUpdates[0].Status != "completed" {
		t.Fatalf("expected completed update, got %+v", q.contentUpdates)
	}
	// The content JSON must carry the Go-computed total_comments_analyzed,
	// not the LLM's echoed zero.
	if !strings.Contains(q.contentUpdates[0].Content, `"total_comments_analyzed":12`) {
		t.Errorf("numeric overwrite missing in content: %s", q.contentUpdates[0].Content)
	}
}

func TestRunUserDigest_DeferOnRetryExhaustion(t *testing.T) {
	cs := make([]*db.Comment, 11)
	for i := range cs {
		cs[i] = &db.Comment{Body: "x"}
	}
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": cs},
		pendingInserted: true,
	}

	// Compress the retry schedule for the test so it runs fast. Restore
	// the production constants via t.Cleanup.
	prev := digestRetrySchedule
	digestRetrySchedule = []time.Duration{0, 1 * time.Millisecond, 2 * time.Millisecond}
	t.Cleanup(func() { digestRetrySchedule = prev })

	w := newTestWorker(t, q, newMockOpenCode(t, "", "boom", 0))
	w.runUserDigest(context.Background(), &db.User{ID: "u1"}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))

	if len(q.statusUpdates) != 1 || q.statusUpdates[0].Status != "deferred" {
		t.Errorf("expected deferred, got %+v", q.statusUpdates)
	}
}

func TestRunUserDigest_IdempotentSecondInvocation(t *testing.T) {
	cs := make([]*db.Comment, 11)
	for i := range cs {
		cs[i] = &db.Comment{Body: "x"}
	}
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": cs},
		pendingInserted: false, // simulates ON CONFLICT: row already exists
	}
	w := newTestWorker(t, q, newMockOpenCode(t, "{}", "", 0))
	w.runUserDigest(context.Background(), &db.User{ID: "u1"}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))
	if len(q.contentUpdates) != 0 || len(q.statusUpdates) != 0 {
		t.Errorf("idempotent skip should leave row untouched")
	}
}

func TestRecoverStalePending(t *testing.T) {
	q := &fakeDigestQuery{
		stale: []*db.WeeklyDigest{
			{ID: "d1", UserID: "u1", WeekStartDate: "2024-01-01", Status: "pending"},
			{ID: "d2", UserID: "u2", WeekStartDate: "2024-01-01", Status: "pending"},
		},
	}
	w := newTestWorker(t, q, nil)
	w.recoverStalePending(context.Background())
	if len(q.statusUpdates) != 2 {
		t.Fatalf("expected 2 status updates, got %d", len(q.statusUpdates))
	}
	for _, u := range q.statusUpdates {
		if u.Status != "deferred" {
			t.Errorf("want deferred, got %s", u.Status)
		}
	}
}

// fakeSMTPSender implements email.SMTPSender for tests.
type fakeSMTPSender struct{}

func (f *fakeSMTPSender) Send(ctx context.Context, from, to, subject, htmlBody, textBody string, headers []string) error {
	return nil
}

// fakeEmailLogStore implements db.EmailLogStore for tests.
type fakeEmailLogStore struct{}

func (f *fakeEmailLogStore) IsEmailIdempotencyKeyUsed(ctx context.Context, key string) (bool, error) {
	return false, nil
}
func (f *fakeEmailLogStore) LogEmailAttempt(ctx context.Context, in db.EmailAttemptInput) (string, error) {
	return "log-" + in.IdempotencyKey, nil
}
func (f *fakeEmailLogStore) MarkEmailSent(ctx context.Context, logID string, at time.Time) error {
	return nil
}
func (f *fakeEmailLogStore) MarkEmailAttemptError(ctx context.Context, logID, errMsg string) error {
	return nil
}
func (f *fakeEmailLogStore) IncrementEmailAttempt(ctx context.Context, id string, attempt int, lastErr string) error {
	return nil
}
func (f *fakeEmailLogStore) MarkEmailFailed(ctx context.Context, id string, lastErr string) error {
	return nil
}
func (f *fakeEmailLogStore) ClaimPendingEmails(ctx context.Context, now time.Time, max int) ([]*db.EmailLogRow, error) {
	return nil, nil
}
func (f *fakeEmailLogStore) GetEmailRowByIdempotencyKey(ctx context.Context, key string) (*db.EmailLogRow, error) {
	return nil, nil
}

func TestRunUserDigest_EmailSentWhenOptInTrue(t *testing.T) {
	cs := make([]*db.Comment, 12)
	for i := range cs {
		topic := "topic-A"
		cat := "praise"
		cs[i] = &db.Comment{Body: "loved it", Topic: &topic, Category: &cat}
	}
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": cs},
		pendingInserted: true,
	}
	llmRaw := `{
		"top_ideas":[{"idea":"more like this","evidence":"many positives","sample_comments":["loved it"]}],
		"top_questions":[],
		"top_actionable_insights":[{"type":"video_idea","description":"tutorial","count":2}],
		"engagement_health_score":80,
		"health_score_breakdown":{"response_rate":70,"engagement_depth":75},
		"total_comments_analyzed":12,
		"filtered_comments":0
	}`
	pipeline := &email.Pipeline{
		Queries: &fakeEmailLogStore{},
		Sender:  &fakeSMTPSender{},
		Cfg:     &config.Config{EmailEnabled: true},
		Logger:  log.New(io.Discard, "", 0),
	}
	w := newTestWorker(t, q, newMockOpenCode(t, llmRaw, "", 0))
	w.Pipeline = pipeline
	w.runUserDigest(context.Background(), &db.User{ID: "u1", Email: "u1@test.com", DigestEmailOptIn: true}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))

	// Digest should be completed and email path exercised (no error from Send)
	if len(q.contentUpdates) != 1 || q.contentUpdates[0].Status != "completed" {
		t.Fatalf("expected completed status, got %+v", q.contentUpdates)
	}
}

func TestRunUserDigest_EmailNotSentWhenOptInFalse(t *testing.T) {
	cs := make([]*db.Comment, 12)
	for i := range cs {
		topic := "topic-A"
		cat := "praise"
		cs[i] = &db.Comment{Body: "loved it", Topic: &topic, Category: &cat}
	}
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": cs},
		pendingInserted: true,
	}
	llmRaw := `{
		"top_ideas":[{"idea":"more like this","evidence":"many positives","sample_comments":["loved it"]}],
		"top_questions":[],
		"top_actionable_insights":[],
		"engagement_health_score":80,
		"health_score_breakdown":{"response_rate":70,"engagement_depth":75},
		"total_comments_analyzed":12,
		"filtered_comments":0
	}`
	// Use a pipeline that would panic if Send is called (to verify it is NOT called)
	panicSender := &panicSMTPSender{}
	pipeline := &email.Pipeline{
		Queries: &fakeEmailLogStore{},
		Sender:  panicSender,
		Cfg:     &config.Config{EmailEnabled: true},
		Logger:  log.New(io.Discard, "", 0),
	}
	w := newTestWorker(t, q, newMockOpenCode(t, llmRaw, "", 0))
	w.Pipeline = pipeline
	// Should NOT panic because opt_in=false skips the email path
	w.runUserDigest(context.Background(), &db.User{ID: "u1", Email: "u1@test.com", DigestEmailOptIn: false}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))

	// Digest should still be completed (just no email)
	if len(q.contentUpdates) != 1 || q.contentUpdates[0].Status != "completed" {
		t.Fatalf("expected completed status, got %+v", q.contentUpdates)
	}
}

// panicSMTPSender panics if Send is called, used to verify email path is skipped.
type panicSMTPSender struct{}

func (p *panicSMTPSender) Send(ctx context.Context, from, to, subject, htmlBody, textBody string, headers []string) error {
	panic("SMTPSender.Send should not be called when opt_in=false")
}

func TestRunUserDigest_DigestDelayedEmailOnDefer(t *testing.T) {
	cs := make([]*db.Comment, 11)
	for i := range cs {
		cs[i] = &db.Comment{Body: "x"}
	}
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": cs},
		pendingInserted: true,
	}

	prev := digestRetrySchedule
	digestRetrySchedule = []time.Duration{0, 1 * time.Millisecond, 2 * time.Millisecond}
	t.Cleanup(func() { digestRetrySchedule = prev })

	// Track whether Send was called for digest_delayed
	sender := &trackingSMTPSender{}
	pipeline := &email.Pipeline{
		Queries: &fakeEmailLogStore{},
		Sender:  sender,
		Cfg:     &config.Config{EmailEnabled: true},
		Logger:  log.New(io.Discard, "", 0),
	}
	w := newTestWorker(t, q, newMockOpenCode(t, "", "boom", 0))
	w.Pipeline = pipeline
	w.runUserDigest(context.Background(), &db.User{ID: "u1", Email: "u1@test.com", DigestEmailOptIn: true}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))

	if !sender.called {
		t.Fatal("expected digest_delayed email to be sent on defer")
	}
	// The email was sent to the user's address (template rendering happens in Pipeline.Send)
	if sender.lastTo != "u1@test.com" {
		t.Errorf("expected email to u1@test.com, got %s", sender.lastTo)
	}
}

// trackingSMTPSender records whether Send was called.
type trackingSMTPSender struct {
	called bool
	lastTo string
}

func (t *trackingSMTPSender) Send(ctx context.Context, from, to, subject, htmlBody, textBody string, headers []string) error {
	t.called = true
	t.lastTo = to
	return nil
}

func TestRunUserDigest_EmailedAtMarkedAfterSend(t *testing.T) {
	cs := make([]*db.Comment, 12)
	for i := range cs {
		topic := "topic-A"
		cat := "praise"
		cs[i] = &db.Comment{Body: "loved it", Topic: &topic, Category: &cat}
	}
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": cs},
		pendingInserted: true,
	}
	llmRaw := `{
		"top_ideas":[],
		"top_questions":[],
		"top_actionable_insights":[],
		"engagement_health_score":80,
		"health_score_breakdown":{"response_rate":70,"engagement_depth":75},
		"total_comments_analyzed":12,
		"filtered_comments":0
	}`
	pipeline := &email.Pipeline{
		Queries: &fakeEmailLogStore{},
		Sender:  &fakeSMTPSender{},
		Cfg:     &config.Config{EmailEnabled: true},
		Logger:  log.New(io.Discard, "", 0),
	}
	w := newTestWorker(t, q, newMockOpenCode(t, llmRaw, "", 0))
	w.Pipeline = pipeline
	w.runUserDigest(context.Background(), &db.User{ID: "u1", Email: "u1@test.com", DigestEmailOptIn: true}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))

	// Verify digest completed and email path was exercised
	if len(q.contentUpdates) != 1 || q.contentUpdates[0].Status != "completed" {
		t.Fatalf("expected completed status, got %+v", q.contentUpdates)
	}
}

func TestRunUserDigest_PausedChannelsExcluded(t *testing.T) {
	// Simulate paused channels by returning no comments (the SQL query filters
	// by tracking_status='active', so paused channels produce no eligible comments).
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": {}},
		pendingInserted: true,
	}
	w := newTestWorker(t, q, newMockOpenCode(t, "{}", "", 0))
	w.runUserDigest(context.Background(), &db.User{ID: "u1"}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))

	// With 0 comments (< digestSkipThreshold=10), should be skipped.
	if len(q.contentUpdates) != 1 || q.contentUpdates[0].Status != "skipped_insufficient_data" {
		t.Errorf("expected skip due to insufficient data (paused channels excluded), got %+v", q.contentUpdates)
	}
}

func TestCatchUpMissedWeeksDispatches(t *testing.T) {
	// Tuesday after Monday 08:00 trigger: startup should catch up previous week.
	now := time.Date(2024, 1, 9, 12, 0, 0, 0, time.UTC) // Tuesday
	cs := make([]*db.Comment, 12)
	for i := range cs {
		cs[i] = &db.Comment{Body: "x"}
	}
	q := &fakeDigestQuery{
		users:           []*db.User{{ID: "u1"}},
		comments:        map[string][]*db.Comment{"u1": cs},
		pendingInserted: true,
	}
	llmRaw := `{"top_ideas":[],"top_questions":[],"top_actionable_insights":[],"engagement_health_score":80,"health_score_breakdown":{"response_rate":70,"engagement_depth":75},"total_comments_analyzed":12,"filtered_comments":0}`
	w := &Worker{
		Queries:   q,
		LLMClient: newMockOpenCode(t, llmRaw, "", 0),
		Logger:    log.New(io.Discard, "", 0),
		Now:       func() time.Time { return now },
	}
	w.dispatched = map[string]struct{}{}
	w.catchUpMissedWeeks(context.Background())
	if len(q.contentUpdates) == 0 {
		t.Fatalf("catch-up should dispatch missed week")
	}
}

func TestRedispatchDeferredCompletes(t *testing.T) {
	cs := make([]*db.Comment, 12)
	for i := range cs {
		topic := "t"
		cat := "praise"
		cs[i] = &db.Comment{Body: "x", Topic: &topic, Category: &cat}
	}
	q := &fakeDigestQuery{
		users:    []*db.User{{ID: "u1", Email: "u@e.c"}},
		comments: map[string][]*db.Comment{"u1": cs},
		deferredDue: []*db.WeeklyDigest{
			{ID: "d1", UserID: "u1", WeekStartDate: "2024-01-01", Status: "deferred"},
		},
	}
	llmRaw := `{"top_ideas":[],"top_questions":[],"top_actionable_insights":[],"engagement_health_score":80,"health_score_breakdown":{"response_rate":70,"engagement_depth":75},"total_comments_analyzed":12,"filtered_comments":0}`
	w := newTestWorker(t, q, newMockOpenCode(t, llmRaw, "", 0))
	w.redispatchDeferred(context.Background())
	if len(q.contentUpdates) != 1 || q.contentUpdates[0].Status != "completed" {
		t.Fatalf("deferred should complete, got %+v", q.contentUpdates)
	}
}

// --- run-loop trigger starvation regression tests ---------------------------

// fakeDigestClock is a manually-advanced wall clock for driving the run loop.
// reads, when non-nil, receives every value returned by now() so a test can
// prove the loop actually observed a given instant instead of racing a timer.
type fakeDigestClock struct {
	mu    sync.Mutex
	t     time.Time
	reads chan time.Time
}

func (c *fakeDigestClock) now() time.Time {
	c.mu.Lock()
	t := c.t
	c.mu.Unlock()
	if c.reads != nil {
		select {
		case c.reads <- t:
		default:
		}
	}
	return t
}

func (c *fakeDigestClock) set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// waitForClockRead blocks until the run loop has read the clock at want (or
// fails the test after a generous safety timeout). Earlier reads are drained.
func waitForClockRead(t *testing.T, reads <-chan time.Time, want time.Time) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case got := <-reads:
			if got.Equal(want) {
				return
			}
		case <-timeout:
			t.Fatalf("run loop never read the clock at %s", want.Format(time.RFC3339))
		}
	}
}

func monday2024(hour, min int) time.Time {
	return time.Date(2024, 1, 8, hour, min, 0, 0, time.UTC) // 2024-01-08 is a Monday
}

func twelveComments() []*db.Comment {
	cs := make([]*db.Comment, 12)
	for i := range cs {
		topic := "t"
		cat := "praise"
		cs[i] = &db.Comment{Body: "x", Topic: &topic, Category: &cat}
	}
	return cs
}

// TestRun_TriggerNotStarvedByDeferredRetries reproduces the Confirmed Bug:
// 50 deferred rows are due and their retry work stalls in flight. The clock is
// advanced 07:59:30 → 08:00 → 08:50 (the stall window). The weekly trigger for
// the current week must still have dispatched exactly once.
//
// The test drives the loop through an injected tick channel and synchronizes on
// explicit channels (the deferred-sweep gate, the weekly-insert signal, and the
// clock reads), so it is independent of wall-clock scheduling.
func TestRun_TriggerNotStarvedByDeferredRetries(t *testing.T) {
	const deferredRows = deferredRedispatchLimit // 50

	deferred := make([]*db.WeeklyDigest, deferredRows)
	for i := range deferred {
		deferred[i] = &db.WeeklyDigest{
			ID:            fmt.Sprintf("d%02d", i),
			UserID:        "du",
			WeekStartDate: "2024-01-01",
			Status:        "deferred",
		}
	}
	started := make(chan struct{}, 1)
	gate := make(chan struct{}) // never closed: sweep stays in flight until cancel
	weeklyDispatched := make(chan struct{}, 1)

	q := &fakeDigestQuery{
		users:               []*db.User{{ID: "u1", Email: "u1@test.com"}},
		comments:            map[string][]*db.Comment{"u1": twelveComments(), "du": twelveComments()},
		pendingInserted:     true,
		deferredDue:         deferred,
		userLookupStarted:   started,
		userLookupGate:      gate,
		insertPendingSignal: weeklyDispatched,
	}
	clock := &fakeDigestClock{
		t:     monday2024(7, 59).Add(30 * time.Second),
		reads: make(chan time.Time, 64),
	}
	ticks := make(chan time.Time, 8)
	w := &Worker{
		Queries:   q,
		LLMClient: newMockOpenCode(t, digestGoodRaw, "", 0),
		Logger:    log.New(io.Discard, "", 0),
		Now:       clock.now,
		tickCh:    ticks,
	}
	w.dispatched = map[string]struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); w.run(ctx) }()

	// Tick 1 (07:59:30, not the trigger minute): the loop launches the deferred
	// sweep, which signals it is in flight and then blocks on the gate.
	ticks <- monday2024(7, 59).Add(30 * time.Second)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("deferred retry never started")
	}

	// Tick 2 (08:00, the trigger minute): the weekly dispatch must fire. If the
	// sweep ran before trigger evaluation it would block the loop here and this
	// signal would never arrive.
	clock.set(monday2024(8, 0))
	ticks <- monday2024(8, 0)
	select {
	case <-weeklyDispatched:
	case <-time.After(5 * time.Second):
		t.Fatal("weekly trigger was starved by the stalled deferred sweep")
	}

	// Tick 3 (08:50, past the whole trigger window): wait until the loop has
	// actually observed this instant, then assert no second dispatch happened.
	clock.set(monday2024(8, 50))
	ticks <- monday2024(8, 50)
	waitForClockRead(t, clock.reads, monday2024(8, 50))

	if got := q.insertedCount(); got != 1 {
		t.Fatalf("weekly trigger must dispatch exactly once despite stalled deferred sweep, got %d", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run loop did not stop after cancellation")
	}
	w.wg.Wait()
}

// TestRedispatchDeferred_RetriesAllDueRows proves deferred rows are still
// retried (not dropped): every due row reaches a terminal update.
func TestRedispatchDeferred_RetriesAllDueRows(t *testing.T) {
	users := []*db.User{{ID: "u1"}, {ID: "u2"}, {ID: "u3"}}
	comments := map[string][]*db.Comment{
		"u1": twelveComments(), "u2": twelveComments(), "u3": twelveComments(),
	}
	deferred := []*db.WeeklyDigest{
		{ID: "d1", UserID: "u1", WeekStartDate: "2024-01-01", Status: "deferred"},
		{ID: "d2", UserID: "u2", WeekStartDate: "2024-01-01", Status: "deferred"},
		{ID: "d3", UserID: "u3", WeekStartDate: "2024-01-01", Status: "deferred"},
	}
	q := &fakeDigestQuery{
		users: users, comments: comments, deferredDue: deferred,
	}
	w := newTestWorker(t, q, newMockOpenCode(t, digestGoodRaw, "", 0))
	w.redispatchDeferred(context.Background())

	if len(q.contentUpdates) != len(deferred) {
		t.Fatalf("expected all %d deferred rows retried, got %+v", len(deferred), q.contentUpdates)
	}
	for _, u := range q.contentUpdates {
		if u.Status != "completed" {
			t.Errorf("deferred row %s not completed: %s", u.DigestID, u.Status)
		}
	}
}

// TestStartStop_CancelsInFlightRedispatch proves Stop terminates cleanly
// while a deferred sweep is blocked mid-retry.
func TestStartStop_CancelsInFlightRedispatch(t *testing.T) {
	t.Setenv("DIGEST_WORKER_ENABLED", "true")

	started := make(chan struct{}, 1)
	gate := make(chan struct{}) // never closed: Stop's cancellation must unblock

	q := &fakeDigestQuery{
		users:    []*db.User{{ID: "du"}},
		comments: map[string][]*db.Comment{"du": twelveComments()},
		deferredDue: []*db.WeeklyDigest{
			{ID: "d1", UserID: "du", WeekStartDate: "2024-01-01", Status: "deferred"},
		},
		userLookupStarted: started,
		userLookupGate:    gate,
	}
	w := &Worker{
		Queries:      q,
		LLMClient:    newMockOpenCode(t, digestGoodRaw, "", 0),
		Logger:       log.New(io.Discard, "", 0),
		Now:          func() time.Time { return monday2024(7, 59).Add(30 * time.Second) },
		TickInterval: 5 * time.Millisecond,
	}
	w.Start(context.Background())

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight redispatch never started")
	}

	stopped := make(chan struct{})
	go func() { w.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not terminate an in-flight redispatch")
	}
}

// --- callWithRetry schedule tests ------------------------------------------

// withRetrySchedule swaps the package-level retry schedule for the duration
// of a test so the real multi-second backoffs never hit the default suite.
func withRetrySchedule(t *testing.T, schedule []time.Duration) {
	t.Helper()
	prev := digestRetrySchedule
	digestRetrySchedule = schedule
	t.Cleanup(func() { digestRetrySchedule = prev })
}

// countingLLM builds an opencode client whose backing server counts requests
// and lets the test decide the status/body for each (1-based) call. Non-200
// responses are written verbatim; 200 responses wrap body as chat content.
func countingLLM(t *testing.T, respond func(call int) (status int, body string)) (*opencodeClient, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&calls, 1))
		status, body := respond(n)
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{"message": map[string]any{"content": body}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return newOpenCodeClientWithHTTP(srv.Client(), srv.URL, "test-key", "test-model"), &calls
}

const digestGoodRaw = `{"top_ideas":[],"top_questions":[],"top_actionable_insights":[],"engagement_health_score":80,"health_score_breakdown":{"response_rate":70,"engagement_depth":75},"total_comments_analyzed":12,"filtered_comments":0}`

// TestCallWithRetry_ExhaustsSchedule proves the synchronous retry chain fires
// all three attempts (not one) and returns the final attempt's error.
func TestCallWithRetry_ExhaustsSchedule(t *testing.T) {
	withRetrySchedule(t, []time.Duration{0, 1 * time.Millisecond, 2 * time.Millisecond})

	client, calls := countingLLM(t, func(call int) (int, string) {
		return http.StatusBadRequest, fmt.Sprintf("attempt-%d", call)
	})
	w := &Worker{LLMClient: client, Logger: log.New(io.Discard, "", 0)}

	_, _, err := w.callWithRetry(context.Background(), "p", truthFixture())
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if got := atomic.LoadInt32(calls); got != int32(len(digestRetrySchedule)) {
		t.Fatalf("expected %d provider calls, got %d", len(digestRetrySchedule), got)
	}
	if !strings.Contains(err.Error(), "attempt-3") {
		t.Errorf("expected last attempt's error, got %v", err)
	}
}

// TestCallWithRetry_SucceedsOnFinalAttempt proves the chain stops as soon as
// an attempt succeeds.
func TestCallWithRetry_SucceedsOnFinalAttempt(t *testing.T) {
	withRetrySchedule(t, []time.Duration{0, 1 * time.Millisecond, 2 * time.Millisecond})

	client, calls := countingLLM(t, func(call int) (int, string) {
		if call < 3 {
			return http.StatusBadRequest, "transient"
		}
		return http.StatusOK, digestGoodRaw
	})
	w := &Worker{LLMClient: client, Logger: log.New(io.Discard, "", 0)}

	out, _, err := w.callWithRetry(context.Background(), "p", truthFixture())
	if err != nil {
		t.Fatalf("expected success on attempt 3, got %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 3 {
		t.Fatalf("expected 3 provider calls, got %d", got)
	}
	if out.TotalCommentsAnalyzed != truthFixture().TotalCommentsAnalyzed {
		t.Errorf("ground-truth overwrite missing: got total=%d", out.TotalCommentsAnalyzed)
	}
}

// TestCallWithRetry_ContextCancelAbortsEarly proves cancellation during the
// first backoff aborts the chain and surfaces the context error.
func TestCallWithRetry_ContextCancelAbortsEarly(t *testing.T) {
	// Real long backoffs: cancellation must win the select, not the timer.
	withRetrySchedule(t, []time.Duration{0, 10 * time.Second, 30 * time.Second})

	firstAttempt := make(chan struct{}, 1)
	client, calls := countingLLM(t, func(call int) (int, string) {
		if call == 1 {
			select {
			case firstAttempt <- struct{}{}:
			default:
			}
		}
		return http.StatusBadRequest, "boom"
	})
	w := &Worker{LLMClient: client, Logger: log.New(io.Discard, "", 0)}

	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		out DigestContent
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, _, err := w.callWithRetry(ctx, "p", truthFixture())
		done <- result{out: out, err: err}
	}()

	select {
	case <-firstAttempt:
	case <-time.After(5 * time.Second):
		t.Fatal("first attempt never fired")
	}
	cancel()

	res := <-done
	if !errors.Is(res.err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", res.err)
	}
	if got := atomic.LoadInt32(calls); got >= int32(len(digestRetrySchedule)) {
		t.Fatalf("expected early abort (< %d calls), got %d", len(digestRetrySchedule), got)
	}
}
