package llm

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"database/sql"

	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// fakeClient is a deterministic LLMClient used across processor tests.
type fakeClient struct {
	mu       sync.Mutex
	calls    [][]PromptComment
	results  []LLMResult // returned for every call (length must match input)
	missing  []int       // indexes the fake reports as missing/invalid
	usage    TokenUsage  // reported back alongside success
	err      error
	provider string
	model    string

	// resultsFn allows callers to vary results per call.
	resultsFn func(comments []PromptComment) ([]LLMResult, error)
}

func (f *fakeClient) ProviderName() string { return f.provider }
func (f *fakeClient) ModelName() string    { return f.model }

func (f *fakeClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, []int, TokenUsage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]PromptComment(nil), comments...))
	f.mu.Unlock()
	if f.resultsFn != nil {
		results, err := f.resultsFn(comments)
		return results, f.missing, f.usage, err
	}
	if f.err != nil {
		return nil, nil, TokenUsage{}, f.err
	}
	if len(f.results) > 0 {
		return f.results, f.missing, f.usage, nil
	}
	out := make([]LLMResult, len(comments))
	for i := range comments {
		out[i] = LLMResult{
			CommentIndex:       i,
			Category:           "praise",
			Confidence:         0.9,
			Topic:              "topic",
			ActionableInsights: []Insight{},
		}
	}
	return out, nil, f.usage, nil
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

	sentiments []sentimentCall

	jobs       map[string]string // jobID -> status
	jobErrMsgs map[string]string
	enqueueErr error
}

type updateCall struct {
	id, category, topic, provider, model string
	actionableInsights                   []byte
	confidence                           float64
}

type sentimentCall struct {
	id    string
	score int
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

func (q *fakeQuery) GetUnprocessedCommentsByChannel(ctx context.Context, limit int, maxAttempts int) (map[string][]*db.Comment, error) {
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

func (q *fakeQuery) UpdateCommentLLMData(ctx context.Context, id, category, topic, provider, model string, actionableInsights []byte, confidence float64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.updates = append(q.updates, updateCall{id, category, topic, provider, model, actionableInsights, confidence})
	return nil
}

func (q *fakeQuery) UpdateCommentSentiment(ctx context.Context, id string, score int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.sentiments = append(q.sentiments, sentimentCall{id, score})
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

func (q *fakeQuery) EnqueueLLMJobIfNotExists(ctx context.Context, commentIDs []string, provider string) (string, bool, error) {
	id, err := q.EnqueueLLMJob(ctx, commentIDs, provider)
	return id, true, err
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

func (q *fakeQuery) GetChannelCustomPrompt(ctx context.Context, platformChannelID, userID string) (string, error) {
	return "", nil
}

func (q *fakeQuery) GetCommentBodyByID(ctx context.Context, commentID string) (string, error) {
	return "", nil
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
				{CommentIndex: 0, Category: "praise", Confidence: 0.55, Topic: "x", ActionableInsights: []Insight{}},
				{CommentIndex: 1, Category: "question", Confidence: 0.95, Topic: "y", ActionableInsights: []Insight{}},
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

// 8.9 — parse failure => comments remain unprocessed.
func TestProcessor_LLMReturnsInvalidResponse(t *testing.T) {
	// We test by injecting via resultsFn that returns an error mimicking what
	// the parser would surface. The processor must mark job failed, no
	// updates, no quota increment.
	q := newFakeQuery()
	q.grouped["ch1"] = makeBatch("ch1", 3)
	q.primaryByChannel["ch1"] = withinQuotaUser("u1")

	cli := &fakeClient{
		provider: "openai", model: "gpt-4o",
		resultsFn: func(comments []PromptComment) ([]LLMResult, error) {
			return nil, errors.New("parse: invalid category at index 0")
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
	costCalls []llmCostCall
}

// llmCostCall captures AddLLMCost invocations for cost assertions.
type llmCostCall struct {
	UserID       string
	CostMicro    int64
	PriceVersion int
}

func (q *fakeQueryBilling) InsertCommentUsageLog(ctx context.Context, tx *sql.Tx, userID string, count int, periodStart, periodEnd time.Time) error {
	q.fakeQuery.mu.Lock()
	defer q.fakeQuery.mu.Unlock()
	q.usageLogs = append(q.usageLogs, usageLogCall{UserID: userID, Count: count})
	return nil
}

func (q *fakeQueryBilling) AddLLMCost(ctx context.Context, userID string, periodStart, periodEnd time.Time, costUSDMicro int64, priceVersion int) error {
	q.fakeQuery.mu.Lock()
	defer q.fakeQuery.mu.Unlock()
	q.costCalls = append(q.costCalls, llmCostCall{UserID: userID, CostMicro: costUSDMicro, PriceVersion: priceVersion})
	return nil
}

// llm-cost-analytics 1.3 — a successful batch prices its reported usage
// against the active card and accumulates it on the quota-decrementing
// user's period rollup (attribution follows the overage rule).
func TestProcessor_BatchCostAccumulated(t *testing.T) {
	inner := newFakeQuery()
	q := &fakeQueryBilling{fakeQuery: inner}
	channel := "ch-cost"
	q.grouped[channel] = makeBatch(channel, 3)
	q.primaryByChannel[channel] = withinQuotaUser("u-cost")

	// gpt-4o-mini @ v1: 1000 in * $0.15/M + 500 out * $0.60/M = 450 micro-USD.
	cli := &fakeClient{provider: "openai", model: "gpt-4o-mini", usage: TokenUsage{Input: 1000, Output: 500}}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.costCalls) != 1 {
		t.Fatalf("want 1 cost call, got %d", len(q.costCalls))
	}
	got := q.costCalls[0]
	if got.UserID != "u-cost" {
		t.Errorf("cost attributed to %s, want u-cost (quota-decrementing user)", got.UserID)
	}
	if got.CostMicro != 450 {
		t.Errorf("cost: want 450 micro-USD got %d", got.CostMicro)
	}
	if got.PriceVersion != PriceVersion {
		t.Errorf("price_version: want %d got %d", PriceVersion, got.PriceVersion)
	}
}

// Zero/unreported usage must not write cost rows at all (absent = zero).
func TestProcessor_UnreportedUsageWritesNoCost(t *testing.T) {
	inner := newFakeQuery()
	q := &fakeQueryBilling{fakeQuery: inner}
	channel := "ch-nocost"
	q.grouped[channel] = makeBatch(channel, 2)
	q.primaryByChannel[channel] = withinQuotaUser("u-nocost")

	cli := &fakeClient{provider: "openai", model: "gpt-4o-mini"} // usage stays zero
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.costCalls) != 0 {
		t.Errorf("want no cost rows for unreported usage, got %d", len(q.costCalls))
	}
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

// TestLogOverage_ConcurrentBatchesUseFreshTotal drives two batches for the
// same primary user (two channels, same owner) through persistBatch against a
// shared real SQLite database. The first crosses the quota (95 -> 105) and the
// second pushes further past it (105 -> 115); the true overage is 15. The old
// stale-snapshot path used primary.CommentsUsedThisPeriod for both batches and
// logged 5+5=10. The fresh-total path logs 5+10=15. Run with -race.
func TestLogOverage_ConcurrentBatchesUseFreshTotal(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	ctx := context.Background()

	u, err := q.CreateUser(ctx, fmt.Sprintf("concurrent-overage-%d@example.com", time.Now().UnixNano()), "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	plan, status := "creator", "active"
	used, quota := 95, 100
	if err := q.ApplySubscriptionUpdate(ctx, nil, db.SubscriptionUpdate{
		UserID:                 u.ID,
		SubscriptionPlan:       &plan,
		SubscriptionStatus:     &status,
		CommentsUsedThisPeriod: &used,
		CommentQuotaMonthly:    &quota,
	}); err != nil {
		t.Fatalf("ApplySubscriptionUpdate: %v", err)
	}

	// Shared, read-only snapshot; the fresh total comes from the DB.
	primary := &db.User{
		ID:                     u.ID,
		SubscriptionPlan:       plan,
		SubscriptionStatus:     status,
		CommentQuotaMonthly:    quota,
		CommentsUsedThisPeriod: used,
	}

	p := NewProcessor(&fakeClient{provider: "openai", model: "gpt-4o"}, q)

	makeResults := func(n int) []LLMResult {
		out := make([]LLMResult, n)
		for i := range out {
			out[i] = LLMResult{CommentIndex: i, Category: "praise", Confidence: 0.9, Topic: "t", ActionableInsights: []Insight{}}
		}
		return out
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, ch := range []string{"chA", "chB"} {
		batch := makeBatch(ch, 10)
		results := makeResults(len(batch))
		wg.Add(1)
		go func(ch string, batch []*db.Comment, results []LLMResult) {
			defer wg.Done()
			if _, _, err := p.persistBatch(ctx, batch, results, nil, primary, "job-"+ch); err != nil {
				errs <- err
			}
		}(ch, batch, results)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("persistBatch: %v", err)
	}

	var overage int
	if err := q.DB().QueryRowContext(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM comment_usage_logs WHERE user_id = ?`, u.ID).Scan(&overage); err != nil {
		t.Fatalf("sum overage logs: %v", err)
	}
	if overage != 15 {
		t.Errorf("logged overage: want 15 (true excess 95+20-100), got %d", overage)
	}

	var stored int
	if err := q.DB().QueryRowContext(ctx,
		`SELECT comments_used_this_period FROM users WHERE id = ?`, u.ID).Scan(&stored); err != nil {
		t.Fatalf("select stored count: %v", err)
	}
	if stored != 115 {
		t.Errorf("stored comments_used_this_period: want 115 got %d", stored)
	}
}

// TestOveragePeriod_NilPeriodEndIsStable: with no durable anchor the fallback
// must be derived from the calendar period, not from now itself, so two
// batches in the same period share one period_start/period_end (otherwise
// ListOverageGroups fragments and rounds each tiny group up).
func TestOveragePeriod_NilPeriodEndIsStable(t *testing.T) {
	t1 := time.Date(2026, 5, 3, 1, 2, 3, 0, time.UTC)
	t2 := time.Date(2026, 5, 28, 23, 59, 59, 0, time.UTC)

	start1, end1 := overagePeriod(nil, t1)
	start2, end2 := overagePeriod(nil, t2)
	if !start1.Equal(start2) || !end1.Equal(end2) {
		t.Fatalf("nil CurrentPeriodEnd unstable across one period: (%v..%v) vs (%v..%v)", start1, end1, start2, end2)
	}
	if end1.Year() != 2026 || end1.Month() != time.May {
		t.Errorf("fallback window should end in May 2026, got %v", end1)
	}

	// A different calendar month must yield a different window.
	start3, _ := overagePeriod(nil, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if start3.Equal(start1) {
		t.Errorf("fallback window must change across calendar months, stayed %v", start1)
	}

	// A durable quota_reset_at anchor takes precedence and is stable.
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	u := &db.User{QuotaResetAt: &anchor}
	as, ae := overagePeriod(u, t1)
	bs, be := overagePeriod(u, t2)
	if !as.Equal(bs) || !ae.Equal(be) {
		t.Errorf("QuotaResetAt anchor unstable: (%v..%v) vs (%v..%v)", as, ae, bs, be)
	}
	if !ae.Equal(anchor) {
		t.Errorf("QuotaResetAt anchor: want period_end %v got %v", anchor, ae)
	}
}

// Sentiment labels map to -1/0/1 and are persisted for every comment.
func TestProcessor_PersistsSentimentScores(t *testing.T) {
	q := newFakeQuery()
	channel := "ch1"
	batch := makeBatch(channel, 3)
	q.grouped[channel] = batch
	q.primaryByChannel[channel] = withinQuotaUser("u1")

	cli := &fakeClient{
		provider: "openai", model: "gpt-4o",
		resultsFn: func(comments []PromptComment) ([]LLMResult, error) {
			return []LLMResult{
				{CommentIndex: 0, Category: "praise", Confidence: 0.9, Topic: "x", Sentiment: "positive", ActionableInsights: []Insight{}},
				{CommentIndex: 1, Category: "question", Confidence: 0.9, Topic: "y", Sentiment: "negative", ActionableInsights: []Insight{}},
				{CommentIndex: 2, Category: "other", Confidence: 0.9, Topic: "z", Sentiment: "neutral", ActionableInsights: []Insight{}},
			}, nil
		},
	}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.sentiments) != 3 {
		t.Fatalf("want 3 sentiment updates, got %d", len(q.sentiments))
	}
	wantScores := map[string]int{
		batch[0].ID: 1,
		batch[1].ID: -1,
		batch[2].ID: 0,
	}
	for _, s := range q.sentiments {
		if want, ok := wantScores[s.id]; !ok || s.score != want {
			t.Errorf("sentiment for %s = %d, want %d", s.id, s.score, want)
		}
	}
}

// A comment whose LLM output omits sentiment defaults to neutral (0) — the
// batch still completes.
func TestProcessor_SentimentMissingDefaultsNeutral(t *testing.T) {
	q := newFakeQuery()
	channel := "ch1"
	batch := makeBatch(channel, 1)
	q.grouped[channel] = batch
	q.primaryByChannel[channel] = withinQuotaUser("u1")

	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.sentiments) != 1 {
		t.Fatalf("want 1 sentiment update, got %d", len(q.sentiments))
	}
	if q.sentiments[0].score != 0 {
		t.Errorf("score = %d, want 0", q.sentiments[0].score)
	}
	var completed int
	for _, st := range q.jobs {
		if st == "completed" {
			completed++
		}
	}
	if completed != 1 {
		t.Errorf("want 1 completed job, got %d", completed)
	}
}

func TestProcessor_PanicInTickDoesNotKill(t *testing.T) {
	q := newFakeQuery()
	q.grouped["ch1"] = makeBatch("ch1", 2)
	q.primaryByChannel["ch1"] = withinQuotaUser("u1")
	cli := &fakeClient{
		provider: "openai", model: "gpt-4o",
		resultsFn: func(comments []PromptComment) ([]LLMResult, error) {
			panic("boom")
		},
	}
	p := NewProcessor(cli, q)
	// Must not propagate panic.
	p.tickOnce(context.Background())
	// Second tick with healthy client still works.
	cli2 := &fakeClient{provider: "openai", model: "gpt-4o"}
	p2 := NewProcessor(cli2, q)
	p2.tickOnce(context.Background())
	if len(cli2.calls) != 1 {
		t.Fatalf("want processor alive after panic, got %d calls", len(cli2.calls))
	}
}

type dedupeQuery struct {
	*fakeQuery
	existingID string
}

func (q *dedupeQuery) EnqueueLLMJobIfNotExists(ctx context.Context, commentIDs []string, provider string) (string, bool, error) {
	return q.existingID, false, nil
}

func TestProcessor_DedupeSkipsLLMCall(t *testing.T) {
	inner := newFakeQuery()
	inner.grouped["ch1"] = makeBatch("ch1", 3)
	inner.primaryByChannel["ch1"] = withinQuotaUser("u1")
	q := &dedupeQuery{fakeQuery: inner, existingID: "job-existing"}
	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())
	if len(cli.calls) != 0 {
		t.Fatalf("deduped batch must skip LLM, got %d calls", len(cli.calls))
	}
	if len(inner.updates) != 0 {
		t.Fatalf("deduped batch must not persist, got %d", len(inner.updates))
	}
}

// blockingClient is a goroutine-safe LLMClient whose ClassifyBatch blocks on a
// release channel. It records the maximum number of concurrent in-flight calls
// so tests can assert the processor's effective concurrency.
type blockingClient struct {
	mu        sync.Mutex
	active    int
	maxActive int
	calls     int
	release   chan struct{}
	entered   chan struct{}
}

func (c *blockingClient) ProviderName() string { return "test" }
func (c *blockingClient) ModelName() string    { return "test-model" }

func (c *blockingClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, []int, TokenUsage, error) {
	c.mu.Lock()
	c.active++
	c.calls++
	if c.active > c.maxActive {
		c.maxActive = c.active
	}
	c.mu.Unlock()
	if c.entered != nil {
		select {
		case c.entered <- struct{}{}:
		default:
		}
	}
	select {
	case <-c.release:
	case <-ctx.Done():
	}
	c.mu.Lock()
	c.active--
	c.mu.Unlock()
	out := make([]LLMResult, len(comments))
	for i := range comments {
		out[i] = LLMResult{CommentIndex: i, Category: "praise", Confidence: 0.9, Topic: "topic", ActionableInsights: []Insight{}}
	}
	return out, nil, TokenUsage{}, nil
}

func (c *blockingClient) max() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxActive
}

func (c *blockingClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// seedChannels registers n single-comment channels with an active primary.
func seedChannels(q *fakeQuery, n int) {
	for i := 0; i < n; i++ {
		ch := fmt.Sprintf("ch%d", i)
		q.grouped[ch] = makeBatch(ch, 1)
		q.primaryByChannel[ch] = withinQuotaUser("u-" + ch)
	}
}

// 4.2 — with MaxConcurrency=3 and 6 channel batches, effective concurrency is
// exactly 3: wait until 3 calls are blocked, release them, then assert the
// semaphore never let a 4th overlap.
func TestProcessor_ConcurrencyBounded(t *testing.T) {
	q := newFakeQuery()
	seedChannels(q, 6)
	cli := &blockingClient{release: make(chan struct{}), entered: make(chan struct{}, 6)}
	p := NewProcessor(cli, q)
	p.MaxConcurrency = 3

	done := make(chan struct{})
	go func() { p.tickOnce(context.Background()); close(done) }()

	for i := 0; i < 3; i++ {
		select {
		case <-cli.entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d concurrent batches entered, want 3", i)
		}
	}
	// Give any (incorrectly) unbounded goroutines a chance to overlap.
	time.Sleep(50 * time.Millisecond)
	if got := cli.max(); got != 3 {
		t.Fatalf("max concurrency = %d, want 3", got)
	}
	close(cli.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tickOnce did not finish")
	}
	if got := cli.callCount(); got != 6 {
		t.Fatalf("calls = %d, want 6", got)
	}
}

// 4.3 — MaxConcurrency=1 keeps execution strictly serial.
func TestProcessor_SerialAtConcurrencyOne(t *testing.T) {
	q := newFakeQuery()
	seedChannels(q, 5)
	release := make(chan struct{})
	close(release) // never block; we only care about overlap
	cli := &blockingClient{release: release}
	p := NewProcessor(cli, q)
	p.MaxConcurrency = 1

	p.tickOnce(context.Background())
	if got := cli.max(); got != 1 {
		t.Fatalf("max concurrency = %d, want 1 (serial)", got)
	}
	if got := cli.callCount(); got != 5 {
		t.Fatalf("calls = %d, want 5", got)
	}
}

// 4.4 — disjoint concurrent batches classify every comment exactly once.
func TestProcessor_ConcurrentBatchesNoDuplicates(t *testing.T) {
	q := newFakeQuery()
	total := 0
	for i := 0; i < 6; i++ {
		ch := fmt.Sprintf("ch%d", i)
		q.grouped[ch] = makeBatch(ch, 2)
		q.primaryByChannel[ch] = withinQuotaUser("u-" + ch)
		total += 2
	}
	cli := &fakeClient{provider: "openai", model: "gpt-4o"}
	p := NewProcessor(cli, q)
	p.MaxConcurrency = 3

	p.tickOnce(context.Background())

	seen := map[string]int{}
	for _, call := range cli.calls {
		for _, c := range call {
			seen[c.Body]++
		}
	}
	if len(seen) != total {
		t.Fatalf("classified %d distinct comments, want %d", len(seen), total)
	}
	for body, n := range seen {
		if n != 1 {
			t.Errorf("comment %q classified %d times, want 1", body, n)
		}
	}
	if len(q.updates) != total {
		t.Fatalf("persisted %d updates, want %d", len(q.updates), total)
	}
}

// 4.5 — shutdown waits for in-flight concurrent batches; no goroutine leaks
// once tickOnce returns.
func TestProcessor_ShutdownWaitsForConcurrentBatches(t *testing.T) {
	q := newFakeQuery()
	seedChannels(q, 4)
	cli := &blockingClient{release: make(chan struct{}), entered: make(chan struct{}, 4)}
	p := NewProcessor(cli, q)
	p.MaxConcurrency = 4

	before := runtime.NumGoroutine()
	done := make(chan struct{})
	go func() { p.tickOnce(context.Background()); close(done) }()
	for i := 0; i < 4; i++ {
		select {
		case <-cli.entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d batches entered, want 4", i)
		}
	}

	shutdownDone := make(chan struct{})
	go func() { p.shutdown(); close(shutdownDone) }()
	select {
	case <-shutdownDone:
		t.Fatal("shutdown returned while batches were in flight")
	case <-time.After(100 * time.Millisecond):
	}

	close(cli.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tickOnce did not finish after release")
	}
	select {
	case <-shutdownDone:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not return after batches drained")
	}

	// Goroutines should settle back to ~baseline.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("goroutine leak: before=%d after=%d", before, after)
	}
}

// 4.6 — a cancelled context prevents any batch from starting.
func TestProcessor_CancelledContextStartsNoBatches(t *testing.T) {
	q := newFakeQuery()
	seedChannels(q, 3)
	release := make(chan struct{})
	close(release)
	cli := &blockingClient{release: release}
	p := NewProcessor(cli, q)
	p.MaxConcurrency = 4

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.tickOnce(ctx)

	if got := cli.callCount(); got != 0 {
		t.Fatalf("calls = %d, want 0 (cancelled context)", got)
	}
}

// comment fabricates a single comment with a known body for scripted tests.
func comment(id, channel, body string) *db.Comment {
	return &db.Comment{ID: id, PlatformChannelID: channel, Body: body}
}

// roundQuery serves a fixed sequence of claim rounds regardless of the SQL
// claim query (which the db tests cover). Each call returns the next scripted
// map, so a test can control exactly which batches are claimable when.
type roundQuery struct {
	*fakeQuery
	rounds []map[string][]*db.Comment
	i      int
}

func (q *roundQuery) GetUnprocessedCommentsByChannel(ctx context.Context, limit, maxAttempts int) (map[string][]*db.Comment, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.i >= len(q.rounds) {
		return map[string][]*db.Comment{}, nil
	}
	r := q.rounds[q.i]
	q.i++
	return r, nil
}

// scriptedClient blocks the "b1" batch until released and signals when the
// second "a2" batch starts.
type scriptedClient struct {
	a2       chan struct{}
	releaseB chan struct{}
}

func (c *scriptedClient) ProviderName() string { return "test" }
func (c *scriptedClient) ModelName() string    { return "test" }

func (c *scriptedClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, []int, TokenUsage, error) {
	if len(comments) > 0 && comments[0].Body == "b1" {
		select {
		case <-c.releaseB:
		case <-ctx.Done():
		}
	}
	if len(comments) > 0 && comments[0].Body == "a2" {
		select {
		case c.a2 <- struct{}{}:
		default:
		}
	}
	out := make([]LLMResult, len(comments))
	for i := range comments {
		out[i] = LLMResult{CommentIndex: i, Category: "praise", Confidence: 0.9, Topic: "topic", ActionableInsights: []Insight{}}
	}
	return out, nil, TokenUsage{}, nil
}

// 6.1 — a freed worker takes the next claimed batch immediately instead of
// waiting for the other in-flight batch (the per-tick barrier is gone).
func TestProcessor_WorkerRefillsWithoutBarrier(t *testing.T) {
	inner := newFakeQuery()
	inner.primaryByChannel["A"] = withinQuotaUser("uA")
	inner.primaryByChannel["B"] = withinQuotaUser("uB")
	q := &roundQuery{fakeQuery: inner, rounds: []map[string][]*db.Comment{
		{"A": {comment("a1", "A", "a1")}, "B": {comment("b1", "B", "b1")}},
		{"A": {comment("a2", "A", "a2")}},
	}}
	cli := &scriptedClient{a2: make(chan struct{}, 1), releaseB: make(chan struct{})}
	p := NewProcessor(cli, q)
	p.MaxConcurrency = 2
	p.TickInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()

	select {
	case <-cli.a2:
	case <-time.After(2 * time.Second):
		t.Fatal("second A batch did not start while B was still in flight")
	}
	close(cli.releaseB)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

// 6.2 — reservations/started jobs are bounded by MaxConcurrency, not by the
// number of claimable channels: with 9 channels and a limit of 4, only 4
// batches may be reserved and started while none complete.
func TestProcessor_ReservationsBoundedByConcurrency(t *testing.T) {
	q := newFakeQuery()
	seedChannels(q, 9)
	cli := &blockingClient{release: make(chan struct{}), entered: make(chan struct{}, 9)}
	p := NewProcessor(cli, q)
	p.MaxConcurrency = 4
	p.TickInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()

	for i := 0; i < 4; i++ {
		select {
		case <-cli.entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d batches started, want 4", i)
		}
	}
	time.Sleep(150 * time.Millisecond) // let any over-reservation surface
	if got := cli.callCount(); got != 4 {
		t.Fatalf("classify calls = %d, want 4 (reservations must be bounded)", got)
	}
	q.mu.Lock()
	jobs := len(q.jobs)
	q.mu.Unlock()
	if jobs != 4 {
		t.Fatalf("reserved jobs = %d, want 4", jobs)
	}

	cancel()
	close(cli.release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
}
