package llm

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"noumena/internal/db"
)

// fakeDigestQuery is a hand-rolled DBQuery double for the worker tests. It
// implements only the methods runUserDigest / dispatch / recoverStalePending
// touch; everything else falls through to nil-method panics if accidentally
// invoked, which surfaces accidental test coverage gaps.
type fakeDigestQuery struct {
	db.DBQuery
	mu sync.Mutex

	users    []*db.User
	comments map[string][]*db.Comment // keyed by userID
	prevWeek map[string]*db.WeeklyDigest
	stale    []*db.WeeklyDigest

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

func newTestWorker(t *testing.T, q *fakeDigestQuery, llm DigestLLMClient) *Worker {
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
	w := newTestWorker(t, q, &fakeDigestLLM{raw: "{}"})
	w.runUserDigest(context.Background(), &db.User{ID: "u1"}, time.Now().UTC(), time.Now().UTC().Add(7*24*time.Hour))

	if len(q.contentUpdates) != 1 || q.contentUpdates[0].Status != "skipped_insufficient_data" {
		t.Errorf("expected skip update, got %+v", q.contentUpdates)
	}
}

func TestRunUserDigest_SuccessOverwritesNumeric(t *testing.T) {
	cs := make([]*db.Comment, 12)
	for i := range cs {
		s := 0.5
		topic := "topic-A"
		cat := "praise"
		cs[i] = &db.Comment{Body: "loved it", SentimentScore: &s, Topic: &topic, Category: &cat}
	}
	q := &fakeDigestQuery{
		comments:        map[string][]*db.Comment{"u1": cs},
		pendingInserted: true,
	}
	llm := &fakeDigestLLM{raw: `{
		"top_ideas":[{"idea":"more like this","evidence":"many positives","sample_comments":["loved it"]}],
		"top_questions":[],
		"sentiment_trend":{"current_avg":0,"change_pct":0,"direction":"stable"},
		"engagement_health_score":0,
		"health_score_breakdown":{"response_rate":0,"sentiment_quality":0,"engagement_depth":0},
		"total_comments_analyzed":0,
		"filtered_comments":0
	}`}
	w := newTestWorker(t, q, llm)
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
	llm := &fakeDigestLLM{err: errors.New("boom")}

	// Compress the retry schedule for the test so it runs fast. Restore
	// the production constants via t.Cleanup.
	prev := digestRetrySchedule
	digestRetrySchedule = []time.Duration{0, 1 * time.Millisecond, 2 * time.Millisecond}
	t.Cleanup(func() { digestRetrySchedule = prev })

	w := newTestWorker(t, q, llm)
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
	w := newTestWorker(t, q, &fakeDigestLLM{raw: "{}"})
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
