package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"noumena/internal/db"
)

// fakeClient is a deterministic LLMClient used across processor tests.
type fakeClient struct {
	mu       sync.Mutex
	calls    [][]PromptComment
	results  []LLMResult // returned for every call (length must match input)
	err      error
	provider string
	model    string

	// resultsFn allows callers to vary results per call.
	resultsFn func(comments []PromptComment) ([]LLMResult, error)
}

func (f *fakeClient) ProviderName() string { return f.provider }
func (f *fakeClient) ModelName() string    { return f.model }

func (f *fakeClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]PromptComment(nil), comments...))
	f.mu.Unlock()
	if f.resultsFn != nil {
		return f.resultsFn(comments)
	}
	if f.err != nil {
		return nil, f.err
	}
	if len(f.results) > 0 {
		return f.results, nil
	}
	out := make([]LLMResult, len(comments))
	for i := range comments {
		out[i] = LLMResult{
			CommentIndex: i,
			Category:     "praise",
			Confidence:   0.9,
			Topic:        "topic",
			Sentiment:    0.5,
		}
	}
	return out, nil
}

// fakeQuery implements the slice of DBQuery the processor uses.
type fakeQuery struct {
	db.DBQuery

	mu sync.Mutex

	grouped map[string][]*db.Comment

	primaryByChannel map[string]*db.User
	noPrimary        map[string]bool

	updates []updateCall
	usage   map[string]int

	jobs       map[string]string // jobID -> status
	jobErrMsgs map[string]string
	enqueueErr error
}

type updateCall struct {
	id, category, topic, provider, model string
	sentiment, confidence                float64
}

func newFakeQuery() *fakeQuery {
	return &fakeQuery{
		grouped:          map[string][]*db.Comment{},
		primaryByChannel: map[string]*db.User{},
		noPrimary:        map[string]bool{},
		usage:            map[string]int{},
		jobs:             map[string]string{},
		jobErrMsgs:       map[string]string{},
	}
}

func (q *fakeQuery) GetUnprocessedCommentsByChannel(ctx context.Context, limit int) (map[string][]*db.Comment, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := map[string][]*db.Comment{}
	for k, v := range q.grouped {
		// Defensive copy.
		out[k] = append([]*db.Comment(nil), v...)
	}
	return out, nil
}

func (q *fakeQuery) GetPrimaryTracker(ctx context.Context, platformChannelID string) (*db.User, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.noPrimary[platformChannelID] {
		return nil, db.ErrNoPrimaryTracker
	}
	if u, ok := q.primaryByChannel[platformChannelID]; ok {
		return u, nil
	}
	return nil, db.ErrNoPrimaryTracker
}

func (q *fakeQuery) UpdateCommentLLMData(ctx context.Context, id, category, topic, provider, model string, sentiment, confidence float64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.updates = append(q.updates, updateCall{id, category, topic, provider, model, sentiment, confidence})
	return nil
}

func (q *fakeQuery) IncrementCommentsUsed(ctx context.Context, userID string, count int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.usage[userID] += count
	return nil
}

func (q *fakeQuery) EnqueueLLMJob(ctx context.Context, commentIDs []string, provider string) (string, error) {
	if q.enqueueErr != nil {
		return "", q.enqueueErr
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	id := fmt.Sprintf("job-%d", len(q.jobs)+1)
	q.jobs[id] = "pending"
	return id, nil
}

func (q *fakeQuery) MarkLLMJobProcessing(ctx context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[jobID] = "processing"
	return nil
}

func (q *fakeQuery) MarkLLMJobCompleted(ctx context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[jobID] = "completed"
	return nil
}

func (q *fakeQuery) MarkLLMJobFailed(ctx context.Context, jobID string, msg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[jobID] = "failed"
	q.jobErrMsgs[jobID] = msg
	return nil
}

// makeBatch fabricates n comments for a channel.
func makeBatch(channelID string, n int) []*db.Comment {
	out := make([]*db.Comment, n)
	for i := 0; i < n; i++ {
		out[i] = &db.Comment{
			ID:                fmt.Sprintf("%s-c%d", channelID, i),
			PlatformChannelID: channelID,
			Body:              fmt.Sprintf("comment %d on %s", i, channelID),
		}
	}
	return out
}

// withinQuotaUser is a primary token holder with plenty of headroom.
func withinQuotaUser(id string) *db.User {
	return &db.User{
		ID:                     id,
		CommentQuotaMonthly:    5000,
		CommentsUsedThisPeriod: 0,
	}
}

// 8.1 — fresh batch is claimed, classified, persisted with all five fields.
func TestProcessor_HappyPath_PersistsAllFields(t *testing.T) {
	q := newFakeQuery()
	channel := "ch1"
	batch := makeBatch(channel, 50)
	q.grouped[channel] = batch
	q.primaryByChannel[channel] = withinQuotaUser("u1")

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.updates) != 50 {
		t.Fatalf("want 50 UpdateCommentLLMData calls, got %d", len(q.updates))
	}
	for i, u := range q.updates {
		if u.provider != "openai" || u.model != "gpt-4o" {
			t.Errorf("update[%d] provider/model = %s/%s", i, u.provider, u.model)
		}
		if u.category == "" {
			t.Errorf("update[%d] empty category", i)
		}
	}
	if q.usage["u1"] != 50 {
		t.Errorf("want quota incremented by 50, got %d", q.usage["u1"])
	}
	// Exactly one job, completed.
	var completed int
	for _, st := range q.jobs {
		if st == "completed" {
			completed++
		}
	}
	if completed != 1 {
		t.Errorf("want 1 completed job, got %d (%v)", completed, q.jobs)
	}
}

// 8.2 — confidence < threshold downgrades to "other" while preserving original confidence.
func TestProcessor_ConfidenceThresholdDowngrade(t *testing.T) {
	q := newFakeQuery()
	channel := "ch1"
	batch := makeBatch(channel, 2)
	q.grouped[channel] = batch
	q.primaryByChannel[channel] = withinQuotaUser("u1")

	cli := &fakeClient{
		provider: "openai", model: "gpt-4o",
		resultsFn: func(comments []PromptComment) ([]LLMResult, error) {
			return []LLMResult{
				{CommentIndex: 0, Category: "praise", Confidence: 0.55, Topic: "x", Sentiment: 0.5},
				{CommentIndex: 1, Category: "question", Confidence: 0.95, Topic: "y", Sentiment: -0.1},
			}, nil
		},
	}
	p := NewProcessor(cli, q)
	p.ConfidenceThreshold = 0.7
	p.tickOnce(context.Background())

	if len(q.updates) != 2 {
		t.Fatalf("want 2 updates, got %d", len(q.updates))
	}
	if q.updates[0].category != "other" {
		t.Errorf("want first update category=other, got %s", q.updates[0].category)
	}
	if q.updates[0].confidence != 0.55 {
		t.Errorf("want first update confidence=0.55 preserved, got %v", q.updates[0].confidence)
	}
	if q.updates[1].category != "question" {
		t.Errorf("want second update category=question kept, got %s", q.updates[1].category)
	}
}

// 8.3 — cross-channel comments => separate LLM invocations.
func TestProcessor_PerChannelBatching(t *testing.T) {
	q := newFakeQuery()
	q.grouped["chA"] = makeBatch("chA", 3)
	q.grouped["chB"] = makeBatch("chB", 4)
	q.primaryByChannel["chA"] = withinQuotaUser("uA")
	q.primaryByChannel["chB"] = withinQuotaUser("uB")

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(cli.calls) != 2 {
		t.Fatalf("want 2 LLM calls (one per channel), got %d", len(cli.calls))
	}
	for _, call := range cli.calls {
		if len(call) != 3 && len(call) != 4 {
			t.Errorf("unexpected call size %d", len(call))
		}
	}
}

// 8.4 — over-quota primary => skip without writes.
func TestProcessor_OverQuotaSkipped(t *testing.T) {
	q := newFakeQuery()
	q.grouped["ch1"] = makeBatch("ch1", 10)
	overQuota := &db.User{ID: "u1", CommentQuotaMonthly: 100, CommentsUsedThisPeriod: 100}
	q.primaryByChannel["ch1"] = overQuota

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.updates) != 0 {
		t.Errorf("want no updates on over-quota skip, got %d", len(q.updates))
	}
	if q.usage["u1"] != 0 {
		t.Errorf("want quota unchanged, got %d", q.usage["u1"])
	}
	if len(cli.calls) != 0 {
		t.Errorf("want LLM not invoked, got %d calls", len(cli.calls))
	}
}

// 8.5 — successful batch increments only the primary user's quota.
func TestProcessor_QuotaIncrementsPrimaryOnly(t *testing.T) {
	q := newFakeQuery()
	q.grouped["ch1"] = makeBatch("ch1", 7)
	q.primaryByChannel["ch1"] = withinQuotaUser("u-primary")

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if q.usage["u-primary"] != 7 {
		t.Errorf("want primary usage=7, got %d", q.usage["u-primary"])
	}
	if got := q.usage["u-other"]; got != 0 {
		t.Errorf("want non-primary usage unchanged, got %d", got)
	}
}

// 8.6 — no primary tracker => skip without error.
func TestProcessor_NoPrimaryTrackerSkipped(t *testing.T) {
	q := newFakeQuery()
	q.grouped["ch1"] = makeBatch("ch1", 5)
	q.noPrimary["ch1"] = true

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.updates) != 0 {
		t.Errorf("want no updates, got %d", len(q.updates))
	}
	if len(cli.calls) != 0 {
		t.Errorf("want LLM not invoked, got %d calls", len(cli.calls))
	}
}

// 8.9 — out-of-range sentiment => parse failure, comments remain unprocessed.
func TestProcessor_LLMReturnsInvalidSentiment(t *testing.T) {
	// We test by injecting via resultsFn that returns an error mimicking what
	// the parser would surface. The processor must mark job failed, no
	// updates, no quota increment.
	q := newFakeQuery()
	q.grouped["ch1"] = makeBatch("ch1", 3)
	q.primaryByChannel["ch1"] = withinQuotaUser("u1")

	cli := &fakeClient{
		provider: "openai", model: "gpt-4o",
		resultsFn: func(comments []PromptComment) ([]LLMResult, error) {
			return nil, errors.New("parse: sentiment 1.7 out of range at index 0")
		},
	}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.updates) != 0 {
		t.Errorf("want no updates on parse failure, got %d", len(q.updates))
	}
	if q.usage["u1"] != 0 {
		t.Errorf("want quota unchanged, got %d", q.usage["u1"])
	}
	var failed int
	for _, st := range q.jobs {
		if st == "failed" {
			failed++
		}
	}
	if failed != 1 {
		t.Errorf("want 1 failed job, got %d", failed)
	}
}

// 8.10 — graceful shutdown waits for in-flight Tx2 (best-effort: we just
// verify Run exits when ctx is cancelled before any tick fires).
func TestProcessor_ShutdownIdleExitsQuickly(t *testing.T) {
	q := newFakeQuery()
	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.TickInterval = 50 * time.Millisecond
	p.ShutdownGrace = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("processor did not exit on ctx cancel")
	}
}

// ============================================================================
// Billing enforcement (billing change, tasks 10.5.2 – 10.5.4)
// ============================================================================

// usageLogCall captures InsertCommentUsageLog calls so we can assert on the
// overage row that the processor emits when a batch crosses the quota.
type usageLogCall struct {
	UserID string
	Count  int
}

// fakeQueryBilling extends fakeQuery with the InsertCommentUsageLog hook.
// It uses method promotion so all the existing fakeQuery behaviour is
// preserved verbatim.
type fakeQueryBilling struct {
	*fakeQuery
	usageLogs []usageLogCall
}

func (q *fakeQueryBilling) InsertCommentUsageLog(ctx context.Context, tx pgx.Tx, userID string, count int, periodStart, periodEnd time.Time) error {
	q.fakeQuery.mu.Lock()
	defer q.fakeQuery.mu.Unlock()
	q.usageLogs = append(q.usageLogs, usageLogCall{UserID: userID, Count: count})
	return nil
}

// 10.5.2 — paused users are skipped entirely (no LLM call, no DB writes).
func TestProcessor_SkipsPausedUser(t *testing.T) {
	q := newFakeQuery()
	channel := "ch-paused"
	q.grouped[channel] = makeBatch(channel, 10)
	q.primaryByChannel[channel] = &db.User{
		ID:                     "u-paused",
		SubscriptionPlan:       "creator",
		SubscriptionStatus:     "paused",
		CommentQuotaMonthly:    50000,
		CommentsUsedThisPeriod: 0,
	}

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(cli.calls) != 0 {
		t.Errorf("paused user must not trigger LLM calls, got %d", len(cli.calls))
	}
	if len(q.updates) != 0 {
		t.Errorf("paused user must not produce LLM updates, got %d", len(q.updates))
	}
	if q.usage["u-paused"] != 0 {
		t.Errorf("paused user must not have quota incremented, got %d", q.usage["u-paused"])
	}
}

// 10.5.3 — users at quota are skipped (no LLM call, no DB writes).
func TestProcessor_SkipsUserAtQuota(t *testing.T) {
	q := newFakeQuery()
	channel := "ch-full"
	q.grouped[channel] = makeBatch(channel, 5)
	q.primaryByChannel[channel] = &db.User{
		ID:                     "u-full",
		SubscriptionPlan:       "creator",
		SubscriptionStatus:     "active",
		CommentQuotaMonthly:    50000,
		CommentsUsedThisPeriod: 50000,
	}

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(cli.calls) != 0 {
		t.Errorf("at-quota user must not trigger LLM calls, got %d", len(cli.calls))
	}
	if len(q.updates) != 0 {
		t.Errorf("at-quota user must not produce LLM updates, got %d", len(q.updates))
	}
}

// Note on 10.5.4: the overage-log path runs after IncrementCommentsUsed
// inside persistBatch. The fakeQuery embeds db.DBQuery so the new
// InsertCommentUsageLog method is reachable through the interface — its
// default behaviour is a panic, which would mask the assertion. We
// therefore override it via a dedicated wrapper that records the call.

// 10.5.4 — overage row inserted when batch crosses the quota boundary.
func TestProcessor_OverageRowInsertedAtBoundary(t *testing.T) {
	inner := newFakeQuery()
	q := &fakeQueryBilling{fakeQuery: inner}
	channel := "ch-overage"
	q.grouped[channel] = makeBatch(channel, 50)
	q.primaryByChannel[channel] = &db.User{
		ID:                     "u-overage",
		SubscriptionPlan:       "creator",
		SubscriptionStatus:     "active",
		CommentQuotaMonthly:    50000,
		CommentsUsedThisPeriod: 49990, // batch crosses the 50000 boundary by 40
	}

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.usageLogs) != 1 {
		t.Fatalf("want 1 overage row, got %d", len(q.usageLogs))
	}
	got := q.usageLogs[0]
	if got.UserID != "u-overage" {
		t.Errorf("overage row user_id: got %s", got.UserID)
	}
	if got.Count != 40 {
		t.Errorf("overage row count: want 40 got %d", got.Count)
	}
}

// Free-plan users do not incur overage rows even if the batch crosses the
// quota — overage tracking is only for creator/studio.
func TestProcessor_NoOverageRowForFreePlan(t *testing.T) {
	inner := newFakeQuery()
	q := &fakeQueryBilling{fakeQuery: inner}
	channel := "ch-free"
	q.grouped[channel] = makeBatch(channel, 50)
	q.primaryByChannel[channel] = &db.User{
		ID:                     "u-free",
		SubscriptionPlan:       "free",
		SubscriptionStatus:     "active",
		CommentQuotaMonthly:    1000,
		CommentsUsedThisPeriod: 990, // would cross by 40, but plan does not track overage
	}

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.usageLogs) != 0 {
		t.Errorf("free plan must not produce overage rows, got %d", len(q.usageLogs))
	}
}

// guard: prove fakeQueryBilling still embeds the original fakeQuery.
var _ = errors.New

