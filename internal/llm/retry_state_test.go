package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"noumena/internal/db"
)

// retryFakeQuery extends fakeQuery with the per-comment retry-state surface
// (IncrementLLMAttempts / MaxLLMAttempts) and a claim query that honors
// llm_attempts / llm_next_retry_at against a fake clock.
type retryFakeQuery struct {
	*fakeQuery
	queue     []*db.Comment
	attempts  map[string]int
	nextRetry map[string]time.Time
	nowFn     func() time.Time
}

func newRetryFakeQuery() *retryFakeQuery {
	return &retryFakeQuery{
		fakeQuery: newFakeQuery(),
		attempts:  map[string]int{},
		nextRetry: map[string]time.Time{},
		nowFn:     time.Now,
	}
}

func (q *retryFakeQuery) GetUnprocessedCommentsByChannel(ctx context.Context, limit, maxAttempts int) (map[string][]*db.Comment, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := map[string][]*db.Comment{}
	now := q.nowFn()
	n := 0
	for _, c := range q.queue {
		if n >= limit {
			break
		}
		if q.attempts[c.ID] >= maxAttempts {
			continue
		}
		if nr, ok := q.nextRetry[c.ID]; ok && nr.After(now) {
			continue
		}
		out[c.PlatformChannelID] = append(out[c.PlatformChannelID], c)
		n++
	}
	return out, nil
}

func (q *retryFakeQuery) IncrementLLMAttempts(ctx context.Context, ids []string, next *time.Time, park bool, maxAttempts int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, id := range ids {
		if park {
			q.attempts[id] = maxAttempts
			delete(q.nextRetry, id)
			continue
		}
		q.attempts[id]++
		if next != nil {
			q.nextRetry[id] = *next
		} else {
			delete(q.nextRetry, id)
		}
	}
	return nil
}

func (q *retryFakeQuery) MaxLLMAttempts(ctx context.Context, ids []string) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	m := 0
	for _, id := range ids {
		if q.attempts[id] > m {
			m = q.attempts[id]
		}
	}
	return m, nil
}

func fullResults(n int) []LLMResult {
	out := make([]LLMResult, n)
	for i := range out {
		out[i] = LLMResult{CommentIndex: i, Category: "praise", Confidence: 0.9, Topic: "t", ActionableInsights: []Insight{}}
	}
	return out
}

// 4.3 — 49 valid + 1 invalid persists 49, charges 49, and burns an attempt
// only on the invalid comment.
func TestProcessor_PartialResults_49Valid1Invalid(t *testing.T) {
	q := newRetryFakeQuery()
	batch := makeBatch("ch1", 50)
	q.queue = batch
	q.primaryByChannel["ch1"] = withinQuotaUser("u1")

	cli := &fakeClient{provider: "openai", model: "m", results: fullResults(49), missing: []int{49}}
	p := NewProcessor(cli, q)
	p.BatchSize = 50
	p.tickOnce(context.Background())

	if len(q.updates) != 49 {
		t.Fatalf("updates = %d, want 49", len(q.updates))
	}
	if q.usage["u1"] != 49 {
		t.Errorf("quota = %d, want 49", q.usage["u1"])
	}
	if q.attempts[batch[49].ID] != 1 {
		t.Errorf("invalid comment attempts = %d, want 1", q.attempts[batch[49].ID])
	}
	if q.attempts[batch[0].ID] != 0 {
		t.Errorf("valid comment attempts = %d, want 0", q.attempts[batch[0].ID])
	}
	var failed int
	for _, st := range q.jobs {
		if st == "failed" {
			failed++
		}
	}
	if failed != 1 {
		t.Errorf("failed jobs = %d, want 1", failed)
	}
}

// 4.3 — zero valid results persists nothing and burns all comments.
func TestProcessor_AllInvalid_FullFailure(t *testing.T) {
	q := newRetryFakeQuery()
	batch := makeBatch("ch1", 3)
	q.queue = batch
	q.primaryByChannel["ch1"] = withinQuotaUser("u1")

	cli := &fakeClient{
		provider: "openai", model: "m",
		resultsFn: func([]PromptComment) ([]LLMResult, error) { return nil, nil },
	}
	p := NewProcessor(cli, q)
	p.tickOnce(context.Background())

	if len(q.updates) != 0 {
		t.Fatalf("updates = %d, want 0", len(q.updates))
	}
	if q.usage["u1"] != 0 {
		t.Errorf("quota = %d, want 0", q.usage["u1"])
	}
	for _, c := range batch {
		if q.attempts[c.ID] != 1 {
			t.Errorf("attempts[%s] = %d, want 1", c.ID, q.attempts[c.ID])
		}
	}
}

// 5.4 — a permanently-failing comment set stops after exactly LLM_MAX_ATTEMPTS
// provider calls across many ticks (the incident was 89).
func TestProcessor_BoundedRetry_StopsAtCap(t *testing.T) {
	q := newRetryFakeQuery()
	c := makeBatch("ch1", 1)[0]
	q.queue = []*db.Comment{c}
	q.primaryByChannel["ch1"] = withinQuotaUser("u1")

	now := time.Unix(1_700_000_000, 0)
	q.nowFn = func() time.Time { return now }

	cli := &fakeClient{provider: "openai", model: "m", err: errors.New("transport: boom")}
	p := NewProcessor(cli, q)
	p.now = func() time.Time { return now }

	for i := 0; i < 50; i++ {
		p.tickOnce(context.Background())
		now = now.Add(20 * time.Minute) // always past the backoff gap
	}
	if got := len(cli.calls); got != 3 {
		t.Fatalf("provider calls = %d, want exactly 3 (bounded)", got)
	}
	if q.attempts[c.ID] != 3 {
		t.Errorf("attempts = %d, want 3", q.attempts[c.ID])
	}
}

// 5.4 — backoff gaps gate claiming: a second tick inside the backoff window
// makes no provider call.
func TestProcessor_BackoffGatesClaim(t *testing.T) {
	q := newRetryFakeQuery()
	c := makeBatch("ch1", 1)[0]
	q.queue = []*db.Comment{c}
	q.primaryByChannel["ch1"] = withinQuotaUser("u1")

	now := time.Unix(1_700_000_000, 0)
	q.nowFn = func() time.Time { return now }

	cli := &fakeClient{provider: "openai", model: "m", err: errors.New("transport: boom")}
	p := NewProcessor(cli, q)
	p.now = func() time.Time { return now }

	p.tickOnce(context.Background())
	if got := len(cli.calls); got != 1 {
		t.Fatalf("first tick calls = %d, want 1", got)
	}

	// 30s later: still inside the 1m backoff → no claim, no call.
	now = now.Add(30 * time.Second)
	p.tickOnce(context.Background())
	if got := len(cli.calls); got != 1 {
		t.Fatalf("gated tick calls = %d, want 1 (backoff must hold)", got)
	}

	// 60s after the failure: backoff elapsed → retry.
	now = now.Add(30 * time.Second)
	p.tickOnce(context.Background())
	if got := len(cli.calls); got != 2 {
		t.Fatalf("post-backoff tick calls = %d, want 2", got)
	}
}
