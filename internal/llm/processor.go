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
	if len(batch) == 0 {
		return
	}

	// Resolve primary token holder. No primary => skip (no DB writes).
	primary, err := p.Q.GetPrimaryTracker(ctx, channelID)
	if err != nil {
		if errors.Is(err, db.ErrNoPrimaryTracker) {
			log.Printf("event=batch_skipped_no_primary channel_id=%s batch_size=%d", channelID, len(batch))
			return
		}
		log.Printf("LLM Processor: GetPrimaryTracker failed for channel=%s: %v", channelID, err)
		return
	}

	// Pre-batch billing guard (billing change, task 7.3).
	// Skip the entire batch if the primary token holder is paused, OR if
	// they have already hit their quota (advisory; the post-batch overage
	// logger handles the boundary-crossing case).
	if primary.SubscriptionStatus == "paused" {
		log.Printf("event=batch_skipped_paused channel_id=%s primary_user_id=%s batch_size=%d",
			channelID, primary.ID, len(batch))
		return
	}
	if !(primary.CommentsUsedThisPeriod < primary.CommentQuotaMonthly) {
		log.Printf("event=batch_skipped_quota channel_id=%s primary_user_id=%s used=%d quota=%d batch_size=%d",
			channelID, primary.ID, primary.CommentsUsedThisPeriod, primary.CommentQuotaMonthly, len(batch))
		return
	}

	// Enqueue the job row for observability with dedupe.
	commentIDs := make([]string, 0, len(batch))
	for _, c := range batch {
		commentIDs = append(commentIDs, c.ID)
	}
	jobID, created, err := enqueueLLMJobDeduped(ctx, p.Q, commentIDs, p.Client.ProviderName())
	if err != nil {
		log.Printf("LLM Processor: EnqueueLLMJob failed channel=%s: %v", channelID, err)
		return
	}
	if !created {
		log.Printf("event=batch_deduped channel_id=%s job_id=%s batch_size=%d", channelID, jobID, len(batch))
		return
	}
	if err := p.Q.MarkLLMJobProcessing(ctx, jobID); err != nil {
		log.Printf("LLM Processor: MarkLLMJobProcessing failed job=%s: %v", jobID, err)
		// Continue: we still want to attempt classification.
	}

	log.Printf("event=batch_started channel_id=%s job_id=%s batch_size=%d provider=%s model=%s",
		channelID, jobID, len(batch), p.Client.ProviderName(), p.Client.ModelName())

	// inflight covers the LLM call so graceful shutdown waits for it.
	p.inflight.Add(1)
	defer p.inflight.Done()

	// Build prompt input. The LLM call happens outside any DB transaction.
	customPrompt := ""
	if primary != nil {
		customPrompt, _ = p.Q.GetChannelCustomPrompt(ctx, channelID, primary.ID)
	}
	prompts := make([]PromptComment, len(batch))
	for i, c := range batch {
		pc := PromptComment{Body: c.Body, CustomPrompt: customPrompt}
		if c.ParentCommentID != nil && *c.ParentCommentID != "" {
			parent, _ := p.Q.GetCommentBodyByID(ctx, *c.ParentCommentID)
			pc.ParentContext = parent
		}
		prompts[i] = pc
	}

	llmCtx, cancel := context.WithTimeout(ctx, p.BatchTimeout)
	// otel-instrumentation: wrap each LLM call in a tracer span and
	// emit the (request, duration) metrics on every return path.
	// Provider/model are read once before the call because the
	// failover wrapper may swap them mid-flight; recording the
	// pre-call values matches the "what we asked for" semantic.
	provider := p.Client.ProviderName()
	model := p.Client.ModelName()
	llmCtx, span := llmTracer.Start(llmCtx, "llm.classify_batch",
		traceWithAttrs(provider, model, len(prompts)))
	llmStart := time.Now()
	results, err := p.Client.ClassifyBatch(llmCtx, prompts)
	recordLLMRequest(ctx, provider, model, llmStart, err)
	span.End()
	cancel()
	if err != nil {
		msg := truncate(fmt.Sprintf("%v", err), 4000)
		_ = p.Q.MarkLLMJobFailed(ctx, jobID, msg)
		log.Printf("event=batch_failed channel_id=%s job_id=%s error=%v", channelID, jobID, err)
		return
	}

	// Apply confidence-threshold downgrade.
	results = ApplyConfidenceThreshold(results, p.ConfidenceThreshold)

	// Persist results, increment quota, mark job complete.
	if err := p.persistBatch(ctx, batch, results, primary, jobID); err != nil {
		msg := truncate(fmt.Sprintf("%v", err), 4000)
		_ = p.Q.MarkLLMJobFailed(ctx, jobID, msg)
		log.Printf("event=batch_failed phase=persist channel_id=%s job_id=%s error=%v", channelID, jobID, err)
		return
	}

	log.Printf("event=batch_completed channel_id=%s job_id=%s batch_size=%d primary_user_id=%s",
		channelID, jobID, len(batch), primary.ID)
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
	if billing.TracksOverage(primary.SubscriptionPlan) && primary.CommentQuotaMonthly > 0 {
		preUsed := primary.CommentsUsedThisPeriod
		postUsed := preUsed + len(batch)
		if postUsed > primary.CommentQuotaMonthly {
			delta := postUsed - max2(primary.CommentQuotaMonthly, preUsed)
			if delta > 0 {
				// Period boundaries: prefer current_period_end if known,
				// otherwise approximate a month window ending now.
				periodEnd := time.Now().UTC().Add(31 * 24 * time.Hour)
				if primary.CurrentPeriodEnd != nil {
					periodEnd = *primary.CurrentPeriodEnd
				}
				periodStart := periodEnd.Add(-31 * 24 * time.Hour)
				if err := p.Q.InsertCommentUsageLog(ctx, nil, primary.ID, delta, periodStart, periodEnd); err != nil {
					// Non-fatal: usage was already incremented; we log and
					// continue so the job can complete.
					log.Printf("event=overage_log_failed user=%s delta=%d err=%v", primary.ID, delta, err)
				} else {
					log.Printf("event=overage_logged user=%s plan=%s pre_used=%d post_used=%d quota=%d delta=%d",
						primary.ID, primary.SubscriptionPlan, preUsed, postUsed, primary.CommentQuotaMonthly, delta)
				}
			}
		}
	}

	if err := p.Q.MarkLLMJobCompleted(ctx, jobID); err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}
	return nil
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
