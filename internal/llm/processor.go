package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"noumena/internal/billing"
	"noumena/internal/db"
)

// Processor is the always-on batch worker that drives every classified
// comment through an LLM. It implements the two-transaction lifecycle from
// the spec:
//  1. Tx1: SELECT FOR UPDATE SKIP LOCKED + enqueue llm_processing_jobs
//     row (currently a single transaction at the DB level via the helper
//     methods; the row-locks are implicit in the claim query and released
//     at function return).
//  2. LLM call (no DB locks held).
//  3. Tx2: write classification results, increment quota, mark job done.
//
// The LLM call is performed entirely outside any DB transaction.
type Processor struct {
	Client              LLMClient
	Q                   db.DBQuery
	TickInterval        time.Duration
	BatchSize           int
	BatchTimeout        time.Duration
	ShutdownGrace       time.Duration
	ConfidenceThreshold float64

	// inflight tracks Tx2 work-in-progress so graceful shutdown can wait.
	inflight sync.WaitGroup
}

// NewProcessor returns a Processor with sensible defaults applied.
func NewProcessor(client LLMClient, q db.DBQuery) *Processor {
	return &Processor{
		Client:              client,
		Q:                   q,
		TickInterval:        10 * time.Second,
		BatchSize:           50,
		BatchTimeout:        30 * time.Second,
		ShutdownGrace:       30 * time.Second,
		ConfidenceThreshold: 0.7,
	}
}

// Run blocks until ctx is cancelled, ticking on TickInterval. On shutdown it
// waits for any in-flight Tx2 to finish (bounded by ShutdownGrace) before
// returning.
func (p *Processor) Run(ctx context.Context) {
	if p.Client == nil || p.Q == nil {
		log.Println("LLM Processor: missing client or db, exiting")
		return
	}
	// Boot sweep: a restart can leave 'processing' rows that shadow the
	// dedupe forever (event=batch_deduped). Single processor instance, so
	// olderThan=0 is safe — no live batch can exist before Run starts.
	p.sweepOrphanedLLMJobs(ctx, 0)
	ticker := time.NewTicker(p.TickInterval)
	defer ticker.Stop()

	log.Printf("LLM Processor: started provider=%s model=%s tick=%s batch_size=%d threshold=%.2f",
		p.Client.ProviderName(), p.Client.ModelName(), p.TickInterval, p.BatchSize, p.ConfidenceThreshold)

	for {
		select {
		case <-ctx.Done():
			p.shutdown()
			return
		case <-ticker.C:
			// Sweep orphans older than the LLM batch age bound before
			// claiming, so a corpse can never dedupe-shadow a live batch.
			p.sweepOrphanedLLMJobs(ctx, 10*time.Minute)
			p.tickOnce(ctx)
		}
	}
}

// sweepOrphanedLLMJobs marks llm_processing_jobs rows stuck in 'processing'
// older than olderThan as failed (see RecoverStaleLLMJobs: failed, never
// pending, so they cannot shadow the dedupe again). RecoverStaleLLMJobs is
// declared on db.DBQuery, so the type-assert is belt-and-braces: fakes that
// embed a nil db.DBQuery promote the method but panic on call (see
// enqueueLLMJobDeduped), and a recovered panic keeps them a silent no-op.
func (p *Processor) sweepOrphanedLLMJobs(ctx context.Context, olderThan time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			// Nil-embedded db.DBQuery test double: treat as no reaper.
			return
		}
	}()
	reaper, ok := p.Q.(interface {
		RecoverStaleLLMJobs(context.Context, time.Duration) (int64, error)
	})
	if !ok {
		return
	}
	n, err := reaper.RecoverStaleLLMJobs(ctx, olderThan)
	if err != nil {
		log.Printf("LLM Processor: orphan sweep failed: %v", err)
		return
	}
	if olderThan == 0 {
		log.Printf("LLM Processor: startup reaper expired %d orphaned jobs", n)
		return
	}
	if n > 0 {
		log.Printf("LLM Processor: reaper expired %d orphaned jobs", n)
	}
}

func (p *Processor) shutdown() {
	log.Println("LLM Processor: shutdown requested, waiting for in-flight batches")
	done := make(chan struct{})
	go func() {
		p.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Println("LLM Processor: graceful shutdown complete")
	case <-time.After(p.ShutdownGrace):
		log.Printf("LLM Processor: shutdown grace %s elapsed; exiting", p.ShutdownGrace)
	}
}

// tickOnce claims and processes batches for one tick. Each per-channel batch
// is processed serially within the tick to keep transaction churn predictable;
// horizontal scaling comes from running multiple processor instances (the
// FOR UPDATE SKIP LOCKED claim guarantees disjoint subsets).
func (p *Processor) tickOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("LLM Processor: PANIC recovered in tick: %v", r)
			recordLLMRequest(ctx, "unknown", "unknown", time.Now(), errors.New("panic"))
		}
	}()
	grouped, err := p.Q.GetUnprocessedCommentsByChannel(ctx, p.BatchSize)
	if err != nil {
		log.Printf("LLM Processor: claim failed: %v", err)
		return
	}
	if len(grouped) == 0 {
		return
	}

	for channelID, batch := range grouped {
		if ctx.Err() != nil {
			return
		}
		p.processChannelBatch(ctx, channelID, batch)
	}
}

// processChannelBatch runs the full lifecycle for a single per-channel batch.
func (p *Processor) processChannelBatch(ctx context.Context, channelID string, batch []*db.Comment) {
	primary, ok := p.requireActivePrimary(ctx, channelID, batch)
	if !ok {
		return
	}
	jobID, ok := p.startJob(ctx, channelID, batch)
	if !ok {
		return
	}
	results, err := p.classify(ctx, p.buildPrompts(ctx, channelID, primary, batch))
	if err != nil {
		p.failBatch(ctx, channelID, jobID, "", err)
		return
	}
	// Apply confidence-threshold downgrade.
	results = ApplyConfidenceThreshold(results, p.ConfidenceThreshold)
	// Persist results, increment quota, mark job complete.
	if err := p.persistBatch(ctx, batch, results, primary, jobID); err != nil {
		p.failBatch(ctx, channelID, jobID, "persist", err)
		return
	}
	log.Printf("event=batch_completed channel_id=%s job_id=%s batch_size=%d primary_user_id=%s",
		channelID, jobID, len(batch), primary.ID)
}

// requireActivePrimary resolves the primary token holder (no primary => skip,
// no DB writes) and applies the pre-batch billing guard (billing change,
// task 7.3): skip when the holder is paused OR already at quota (advisory;
// the post-batch overage logger handles the boundary-crossing case).
func (p *Processor) requireActivePrimary(ctx context.Context, channelID string, batch []*db.Comment) (*db.User, bool) {
	if len(batch) == 0 {
		return nil, false
	}
	primary, err := p.Q.GetPrimaryTracker(ctx, channelID)
	if err != nil {
		p.logPrimaryLookup(channelID, len(batch), err)
		return nil, false
	}
	if primary.SubscriptionStatus == "paused" {
		log.Printf("event=batch_skipped_paused channel_id=%s primary_user_id=%s batch_size=%d",
			channelID, primary.ID, len(batch))
		return nil, false
	}
	if primary.CommentsUsedThisPeriod >= primary.CommentQuotaMonthly {
		log.Printf("event=batch_skipped_quota channel_id=%s primary_user_id=%s used=%d quota=%d batch_size=%d",
			channelID, primary.ID, primary.CommentsUsedThisPeriod, primary.CommentQuotaMonthly, len(batch))
		return nil, false
	}
	return primary, true
}

func (p *Processor) logPrimaryLookup(channelID string, batchLen int, err error) {
	if errors.Is(err, db.ErrNoPrimaryTracker) {
		log.Printf("event=batch_skipped_no_primary channel_id=%s batch_size=%d", channelID, batchLen)
		return
	}
	log.Printf("LLM Processor: GetPrimaryTracker failed for channel=%s: %v", channelID, err)
}

// startJob enqueues the job row for observability with dedupe and flips it
// to processing; false means the batch was deduped or enqueue failed.
func (p *Processor) startJob(ctx context.Context, channelID string, batch []*db.Comment) (string, bool) {
	commentIDs := make([]string, 0, len(batch))
	for _, c := range batch {
		commentIDs = append(commentIDs, c.ID)
	}
	jobID, created, err := enqueueLLMJobDeduped(ctx, p.Q, commentIDs, p.Client.ProviderName())
	if err != nil {
		log.Printf("LLM Processor: EnqueueLLMJob failed channel=%s: %v", channelID, err)
		return "", false
	}
	if !created {
		log.Printf("event=batch_deduped channel_id=%s job_id=%s batch_size=%d", channelID, jobID, len(batch))
		return "", false
	}
	if err := p.Q.MarkLLMJobProcessing(ctx, jobID); err != nil {
		// Continue: we still want to attempt classification.
		log.Printf("LLM Processor: MarkLLMJobProcessing failed job=%s: %v", jobID, err)
	}
	log.Printf("event=batch_started channel_id=%s job_id=%s batch_size=%d provider=%s model=%s",
		channelID, jobID, len(batch), p.Client.ProviderName(), p.Client.ModelName())
	return jobID, true
}

// buildPrompts renders the batch prompt input: the primary's custom prompt
// plus a flattened parent context for replies. The LLM call happens outside
// any DB transaction.
func (p *Processor) buildPrompts(ctx context.Context, channelID string, primary *db.User, batch []*db.Comment) []PromptComment {
	customPrompt, _ := p.Q.GetChannelCustomPrompt(ctx, channelID, primary.ID)
	prompts := make([]PromptComment, len(batch))
	for i, c := range batch {
		prompts[i] = PromptComment{Body: c.Body, ParentContext: p.parentContext(ctx, c), CustomPrompt: customPrompt}
	}
	return prompts
}

// parentContext fetches the parent comment body for reply rows.
func (p *Processor) parentContext(ctx context.Context, c *db.Comment) string {
	if c.ParentCommentID == nil || *c.ParentCommentID == "" {
		return ""
	}
	parent, _ := p.Q.GetCommentBodyByID(ctx, *c.ParentCommentID)
	return parent
}

// classify runs the LLM call under the batch timeout. The inflight counter
// covers the call so graceful shutdown waits for it.
func (p *Processor) classify(ctx context.Context, prompts []PromptComment) ([]LLMResult, error) {
	p.inflight.Add(1)
	defer p.inflight.Done()
	llmCtx, cancel := context.WithTimeout(ctx, p.BatchTimeout)
	defer cancel()
	// otel-instrumentation: wrap each LLM call in a tracer span and
	// emit the (request, duration) metrics on every return path.
	// Provider/model are read once before the call because the
	// failover wrapper may swap them mid-flight; recording the
	// pre-call values matches the "what we asked for" semantic.
	provider := p.Client.ProviderName()
	model := p.Client.ModelName()
	traced, span := llmTracer.Start(llmCtx, "llm.classify_batch", traceWithAttrs(provider, model, len(prompts)))
	defer span.End()
	llmStart := time.Now()
	results, err := p.Client.ClassifyBatch(traced, prompts)
	recordLLMRequest(llmCtx, provider, model, llmStart, err)
	return results, err
}

// failBatch records the truncated error on the job row and logs the
// batch_failed event (phase names the persist-stage failure variant).
func (p *Processor) failBatch(ctx context.Context, channelID, jobID, phase string, err error) {
	msg := truncate(fmt.Sprintf("%v", err), 4000)
	_ = p.Q.MarkLLMJobFailed(ctx, jobID, msg)
	if phase == "" {
		log.Printf("event=batch_failed channel_id=%s job_id=%s error=%v", channelID, jobID, err)
		return
	}
	log.Printf("event=batch_failed phase=%s channel_id=%s job_id=%s error=%v", phase, channelID, jobID, err)
}

func enqueueLLMJobDeduped(ctx context.Context, q db.DBQuery, commentIDs []string, provider string) (id string, created bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			// Embedded nil DBQuery double (test fakes) promoted the method
			// set but panics on call — fall back to plain enqueue.
			id, err = q.EnqueueLLMJob(ctx, commentIDs, provider)
			created = true
		}
	}()
	if deduper, ok := q.(interface {
		EnqueueLLMJobIfNotExists(context.Context, []string, string) (string, bool, error)
	}); ok {
		return deduper.EnqueueLLMJobIfNotExists(ctx, commentIDs, provider)
	}
	id, err = q.EnqueueLLMJob(ctx, commentIDs, provider)
	return id, true, err
}

// persistBatch writes every classification result, increments the primary
// token holder's usage by len(batch), logs overage rows when the batch
// crosses the quota boundary for an overage-tracked plan, and marks the job
// row completed. The DBQuery interface does not expose explicit
// transactions; each underlying method is its own short statement. For
// strict atomicity a future iteration can wrap these in a Tx accessor —
// accepted in design.md as MVP behavior.
func (p *Processor) persistBatch(ctx context.Context, batch []*db.Comment, results []LLMResult, primary *db.User, jobID string) error {
	// Map results by comment_index defensively in case the LLM emitted them
	// out of order.
	byIdx := make(map[int]LLMResult, len(results))
	for _, r := range results {
		byIdx[r.CommentIndex] = r
	}
	for i, c := range batch {
		r, ok := byIdx[i]
		if !ok {
			return fmt.Errorf("missing result for comment_index=%d id=%s", i, c.ID)
		}
		if err := p.Q.UpdateCommentLLMData(ctx, c.ID, r.Category, r.Topic, p.Client.ProviderName(), p.Client.ModelName(), mustMarshalInsights(r.ActionableInsights), r.Confidence); err != nil {
			return fmt.Errorf("update comment %s: %w", c.ID, err)
		}
		if err := p.updateCommentSentiment(ctx, c.ID, SentimentScore(r.Sentiment)); err != nil {
			return fmt.Errorf("update comment %s sentiment: %w", c.ID, err)
		}
	}
	if err := p.Q.IncrementCommentsUsed(ctx, primary.ID, len(batch)); err != nil {
		return fmt.Errorf("increment quota: %w", err)
	}

	// Overage logging (billing change, task 7.5). When this batch crosses
	// the user's quota boundary AND their plan tracks overage (creator,
	// studio), insert a comment_usage_logs row with the delta beyond the
	// quota.
	p.logOverage(ctx, primary, len(batch))

	if err := p.Q.MarkLLMJobCompleted(ctx, jobID); err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}
	return nil
}

// logOverage records usage beyond quota when the plan tracks overage.
// Non-fatal: usage was already incremented; we log and continue so the job
// can complete.
func (p *Processor) logOverage(ctx context.Context, primary *db.User, batchLen int) {
	if !billing.TracksOverage(primary.SubscriptionPlan) || primary.CommentQuotaMonthly <= 0 {
		return
	}
	preUsed := primary.CommentsUsedThisPeriod
	postUsed := preUsed + batchLen
	delta := postUsed - max2(primary.CommentQuotaMonthly, preUsed)
	if delta > 0 {
		p.recordOverageDelta(ctx, primary, delta, preUsed, postUsed)
	}
}

func (p *Processor) recordOverageDelta(ctx context.Context, primary *db.User, delta, preUsed, postUsed int) {
	periodStart, periodEnd := overagePeriod(primary.CurrentPeriodEnd)
	if err := p.Q.InsertCommentUsageLog(ctx, nil, primary.ID, delta, periodStart, periodEnd); err != nil {
		log.Printf("event=overage_log_failed user=%s delta=%d err=%v", primary.ID, delta, err)
		return
	}
	log.Printf("event=overage_logged user=%s plan=%s pre_used=%d post_used=%d quota=%d delta=%d",
		primary.ID, primary.SubscriptionPlan, preUsed, postUsed, primary.CommentQuotaMonthly, delta)
}

// overagePeriod bounds the usage log: prefer current_period_end if known,
// otherwise approximate a month window ending now.
func overagePeriod(currentPeriodEnd *time.Time) (time.Time, time.Time) {
	periodEnd := time.Now().UTC().Add(31 * 24 * time.Hour)
	if currentPeriodEnd != nil {
		periodEnd = *currentPeriodEnd
	}
	return periodEnd.Add(-31 * 24 * time.Hour), periodEnd
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// updateCommentSentiment persists the LLM sentiment score. The DBQuery
// interface does not declare this method (it lives on the concrete
// SQLiteQuery), so we type-assert like enqueueLLMJobDeduped; DBs that lack
// it (test fakes) are skipped gracefully.
func (p *Processor) updateCommentSentiment(ctx context.Context, commentID string, score int) error {
	su, ok := p.Q.(interface {
		UpdateCommentSentiment(context.Context, string, int) error
	})
	if !ok {
		return nil
	}
	return su.UpdateCommentSentiment(ctx, commentID, score)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func mustMarshalInsights(insights []Insight) []byte {
	b, err := json.Marshal(insights)
	if err != nil {
		return []byte("[]")
	}
	return b
}
