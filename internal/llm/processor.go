package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"noumena/internal/billing"
	"noumena/internal/db"
	"noumena/internal/telemetry"
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
	MaxConcurrency      int
	BatchTimeout        time.Duration
	MaxAttempts         int
	RetryBackoff        []time.Duration
	ShutdownGrace       time.Duration
	ConfidenceThreshold float64

	// now is the clock used for backoff timestamps; injectable for tests.
	now func() time.Time

	// inflight tracks Tx2 work-in-progress so graceful shutdown can wait.
	inflight sync.WaitGroup
}

// NewProcessor returns a Processor with sensible defaults applied.
func NewProcessor(client LLMClient, q db.DBQuery) *Processor {
	return &Processor{
		Client:              client,
		Q:                   q,
		TickInterval:        10 * time.Second,
		BatchSize:           15,
		MaxConcurrency:      4,
		BatchTimeout:        30 * time.Second,
		MaxAttempts:         3,
		RetryBackoff:        []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute},
		ShutdownGrace:       30 * time.Second,
		ConfidenceThreshold: 0.7,
		now:                 time.Now,
	}
}

// Run blocks until ctx is cancelled, running a continuous pool of
// MaxConcurrency workers. A freed worker is handed the next already-claimed
// channel batch immediately, so a fast channel never waits behind a slow one;
// claims are durable (llm_processing_jobs pending/processing rows are excluded
// by the claim query) which makes concurrent/rapid reclaiming safe. On
// cancellation workers finish their in-flight batch (bounded by ShutdownGrace)
// before Run returns.
func (p *Processor) Run(ctx context.Context) {
	if p.Client == nil || p.Q == nil {
		log.Println("LLM Processor: missing client or db, exiting")
		return
	}
	// Boot sweep: a restart can leave 'processing' rows that shadow the
	// dedupe forever (event=batch_deduped). Single processor instance, so
	// olderThan=0 is safe — no live batch can exist before Run starts.
	p.sweepOrphanedLLMJobs(ctx, 0)

	limit := p.MaxConcurrency
	if limit < 1 {
		limit = 1
	}
	log.Printf("LLM Processor: started provider=%s model=%s max_concurrency=%d batch_size=%d threshold=%.2f",
		p.Client.ProviderName(), p.Client.ModelName(), limit, p.BatchSize, p.ConfidenceThreshold)

	jobs := make(chan claimedBatch, limit)
	permits := make(chan struct{}, limit)
	for i := 0; i < limit; i++ {
		permits <- struct{}{}
	}
	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.worker(ctx, jobs, permits)
		}()
	}
	p.claimLoop(ctx, jobs, permits)
	close(jobs)
	p.shutdown()
	wg.Wait()
}

// claimedBatch is a per-channel batch that has been durably reserved via an
// llm_processing_jobs row. The primary holder is resolved at claim time so the
// worker does no extra lookup and a skipped channel never reserves a job.
type claimedBatch struct {
	channelID string
	batch     []*db.Comment
	primary   *db.User
	jobID     string
}

// worker drains claimed batches until jobs is closed. Each processed batch
// returns one permit, unblocking the claimer to reserve the next batch.
func (p *Processor) worker(ctx context.Context, jobs <-chan claimedBatch, permits chan struct{}) {
	for cb := range jobs {
		p.runBatchGuarded(ctx, cb)
		permits <- struct{}{}
	}
}

// runBatchGuarded runs one claimed batch with panic isolation and in-flight
// accounting so graceful shutdown waits for it.
func (p *Processor) runBatchGuarded(ctx context.Context, cb claimedBatch) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("LLM Processor: PANIC recovered in batch channel=%s: %v", cb.channelID, r)
			recordLLMRequest(ctx, "unknown", "unknown", time.Now(), errors.New("panic"))
		}
	}()
	p.inflight.Add(1)
	defer p.inflight.Done()
	recordLLMBatchInFlight(ctx, 1)
	defer recordLLMBatchInFlight(ctx, -1)
	p.processClaimedBatch(ctx, cb)
}

// claimLoop keeps the worker pool fed. It reserves at most MaxConcurrency
// batches at a time: a permit is consumed per reserved batch and returned by
// the worker once that batch finishes, so `batch_started` logs and
// llm_processing_jobs rows never exceed the configured concurrency. It sleeps
// only when there is nothing claimable.
func (p *Processor) claimLoop(ctx context.Context, jobs chan<- claimedBatch, permits chan struct{}) {
	sweep := time.NewTicker(10 * time.Minute)
	defer sweep.Stop()
	acquire := func() bool {
		select {
		case <-permits:
			return true
		case <-ctx.Done():
			return false
		}
	}
	release := func() { permits <- struct{}{} }
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-sweep.C:
			p.sweepOrphanedLLMJobs(ctx, 10*time.Minute)
		default:
		}
		grouped, err := p.Q.GetUnprocessedCommentsByChannel(ctx, p.BatchSize, p.MaxAttempts)
		if err != nil {
			log.Printf("LLM Processor: claim failed: %v", err)
		}
		p.sampleParkedComments(ctx)
		if len(grouped) == 0 {
			if !sleepCtx(ctx, p.TickInterval) {
				return
			}
			continue
		}
		for channelID, batch := range grouped {
			if ctx.Err() != nil {
				return
			}
			if !acquire() {
				return
			}
			cb, ok := p.reserveBatch(ctx, channelID, batch)
			if !ok {
				release()
				continue
			}
			select {
			case jobs <- cb:
			case <-ctx.Done():
				release()
				return
			}
		}
	}
}

// reserveBatch resolves the primary token holder and durably reserves one
// channel batch (job row + processing mark). false means the batch was skipped
// (no primary / paused / quota) or deduped, so the caller keeps no permit.
func (p *Processor) reserveBatch(ctx context.Context, channelID string, batch []*db.Comment) (claimedBatch, bool) {
	primary, ok := p.requireActivePrimary(ctx, channelID, batch)
	if !ok {
		return claimedBatch{}, false
	}
	jobID, ok := p.startJob(ctx, channelID, batch)
	if !ok {
		return claimedBatch{}, false
	}
	return claimedBatch{channelID: channelID, batch: batch, primary: primary, jobID: jobID}, true
}

// claimBatches selects and reserves every currently claimable channel batch
// (used by the deterministic tickOnce seam; production streams via claimLoop).
func (p *Processor) claimBatches(ctx context.Context) ([]claimedBatch, error) {
	grouped, err := p.Q.GetUnprocessedCommentsByChannel(ctx, p.BatchSize, p.MaxAttempts)
	if err != nil {
		return nil, err
	}
	p.sampleParkedComments(ctx)
	if len(grouped) == 0 {
		return nil, nil
	}
	out := make([]claimedBatch, 0, len(grouped))
	for channelID, batch := range grouped {
		if ctx.Err() != nil {
			break
		}
		if cb, ok := p.reserveBatch(ctx, channelID, batch); ok {
			out = append(out, cb)
		}
	}
	return out, nil
}

// sleepCtx sleeps for d unless ctx is cancelled; false means cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		d = time.Second
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
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

// tickOnce reserves and processes one round of claimable batches with a
// barrier: it is retained as a deterministic seam for tests. Production uses
// Run's continuous pool, which does not wait for the slowest batch.
func (p *Processor) tickOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("LLM Processor: PANIC recovered in tick: %v", r)
			recordLLMRequest(ctx, "unknown", "unknown", time.Now(), errors.New("panic"))
		}
	}()
	claimed, err := p.claimBatches(ctx)
	if err != nil {
		log.Printf("LLM Processor: claim failed: %v", err)
		return
	}
	if len(claimed) == 0 {
		return
	}

	limit := p.MaxConcurrency
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, cb := range claimed {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(cb claimedBatch) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			p.runBatchGuarded(ctx, cb)
		}(cb)
	}
	wg.Wait()
}

// processClaimedBatch runs the full lifecycle for a single reserved batch.
func (p *Processor) processClaimedBatch(ctx context.Context, cb claimedBatch) {
	channelID, batch, primary, jobID := cb.channelID, cb.batch, cb.primary, cb.jobID
	results, missingIndexes, usage, err := p.classify(ctx, p.buildPrompts(ctx, channelID, primary, batch))
	if err != nil {
		p.failBatch(ctx, channelID, jobID, "", err)
		terminal := IsTerminal(err)
		parked := p.recordRetryState(ctx, channelID, jobID, commentIDs(batch), terminal)
		recordLLMJobAttempt(ctx, p.Client.ProviderName(), failureOutcome(terminal, parked))
		return
	}
	// Apply confidence-threshold downgrade.
	results = ApplyConfidenceThreshold(results, p.ConfidenceThreshold)
	// Persist valid results, increment quota by persisted count, mark job.
	persisted, missing, err := p.persistBatch(ctx, batch, results, missingIndexes, primary, jobID)
	if err != nil {
		p.failBatch(ctx, channelID, jobID, "persist", err)
		parked := p.recordRetryState(ctx, channelID, jobID, commentIDs(batch), IsTerminal(err))
		recordLLMJobAttempt(ctx, p.Client.ProviderName(), failureOutcome(IsTerminal(err), parked))
		return
	}
	if persisted > 0 {
		p.recordBatchCost(ctx, primary, channelID, usage)
	}
	if len(missing) > 0 {
		log.Printf("event=batch_partial channel_id=%s job_id=%s persisted=%d missing_indexes=%v",
			channelID, jobID, persisted, missing)
		// Only the missing/invalid comments retry; a parse miss is transient.
		parked := p.recordRetryState(ctx, channelID, jobID, commentIDsAt(batch, missing), false)
		recordLLMJobAttempt(ctx, p.Client.ProviderName(), failureOutcome(false, parked))
		return
	}
	recordLLMJobAttempt(ctx, p.Client.ProviderName(), telemetry.OutcomeCompleted)
	log.Printf("event=batch_completed channel_id=%s job_id=%s batch_size=%d primary_user_id=%s",
		channelID, jobID, len(batch), primary.ID)
}

// failureOutcome maps a failure's class onto the job-attempt outcome enum.
// Exactly one increment per job attempt: terminal → failed_terminal, cap
// reached → parked, otherwise failed_transient.
func failureOutcome(terminal, parked bool) string {
	switch {
	case terminal:
		return telemetry.OutcomeFailedTerminal
	case parked:
		return telemetry.OutcomeParked
	default:
		return telemetry.OutcomeFailedTransient
	}
}

// sampleParkedComments records the current dead-letter depth. Optional DB
// method: embedded-nil test doubles have no CountParkedComments and stay
// no-ops.
func (p *Processor) sampleParkedComments(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
		}
	}()
	if c, ok := p.Q.(interface {
		CountParkedComments(context.Context, int) (int64, error)
	}); ok {
		if n, err := c.CountParkedComments(ctx, p.MaxAttempts); err == nil {
			recordLLMParkedComments(n)
		}
	}
}

// commentIDs returns the ids for a comment slice.
func commentIDs(batch []*db.Comment) []string {
	out := make([]string, 0, len(batch))
	for _, c := range batch {
		out = append(out, c.ID)
	}
	return out
}

// commentIDsAt returns the ids at the given batch indexes.
func commentIDsAt(batch []*db.Comment, indexes []int) []string {
	out := make([]string, 0, len(indexes))
	for _, i := range indexes {
		if i >= 0 && i < len(batch) {
			out = append(out, batch[i].ID)
		}
	}
	return out
}

// recordRetryState bumps the per-comment retry state for a failed (or
// partially failed) batch and reports whether the comments are now parked.
// Terminal errors park immediately (attempts at cap, no backoff); transient
// errors increment and schedule the next backoff slot. Logs event=batch_parked
// when comments cross the cap. Optional DB methods are reached by type
// assertion so embedded-nil test doubles stay no-ops.
func (p *Processor) recordRetryState(ctx context.Context, channelID, jobID string, ids []string, terminal bool) (parked bool) {
	if len(ids) == 0 {
		return false
	}
	defer func() {
		if r := recover(); r != nil {
			// Nil-embedded db.DBQuery test double: treat as no retry store.
			parked = false
		}
	}()
	inc, ok := p.Q.(interface {
		IncrementLLMAttempts(context.Context, []string, *time.Time, bool, int) error
	})
	if !ok {
		return false
	}
	if terminal {
		if err := inc.IncrementLLMAttempts(ctx, ids, nil, true, p.MaxAttempts); err != nil {
			log.Printf("LLM Processor: park attempts failed channel=%s: %v", channelID, err)
			return false
		}
		log.Printf("event=batch_parked channel_id=%s job_id=%s comment_ids=%v reason=terminal", channelID, jobID, ids)
		return true
	}
	cur := p.maxPendingAttempts(ctx, ids)
	next := p.clock().UTC().Add(p.backoffFor(cur))
	if err := inc.IncrementLLMAttempts(ctx, ids, &next, false, p.MaxAttempts); err != nil {
		log.Printf("LLM Processor: increment attempts failed channel=%s: %v", channelID, err)
		return false
	}
	if cur+1 >= p.MaxAttempts {
		log.Printf("event=batch_parked channel_id=%s job_id=%s comment_ids=%v reason=attempt_cap", channelID, jobID, ids)
		return true
	}
	return false
}

// maxPendingAttempts returns the highest llm_attempts among ids, or 0 when the
// query is unavailable (test double).
func (p *Processor) maxPendingAttempts(ctx context.Context, ids []string) int {
	defer func() {
		if r := recover(); r != nil {
		}
	}()
	if m, ok := p.Q.(interface {
		MaxLLMAttempts(context.Context, []string) (int, error)
	}); ok {
		n, err := m.MaxLLMAttempts(ctx, ids)
		if err == nil {
			return n
		}
	}
	return 0
}

// clock returns the processor's clock, defaulting to time.Now.
func (p *Processor) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// backoffFor returns the backoff slot for a comment whose *new* attempt count
// is cur+1 (0-indexed by the prior count), clamped to the schedule length.
func (p *Processor) backoffFor(cur int) time.Duration {
	if len(p.RetryBackoff) == 0 {
		return time.Minute
	}
	i := cur
	if i >= len(p.RetryBackoff) {
		i = len(p.RetryBackoff) - 1
	}
	if i < 0 {
		i = 0
	}
	return p.RetryBackoff[i]
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

// classify runs the LLM call under the batch timeout. The caller holds the
// inflight counter for the whole batch so graceful shutdown waits for it.
func (p *Processor) classify(ctx context.Context, prompts []PromptComment) ([]LLMResult, []int, TokenUsage, error) {
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
	results, missingIndexes, usage, err := p.Client.ClassifyBatch(traced, prompts)
	recordLLMRequest(llmCtx, provider, model, llmStart, err)
	recordLLMTokens(llmCtx, provider, model, usage.Input+usage.Output)
	return results, missingIndexes, usage, err
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

// persistBatch writes valid classification results (partial acceptance: a
// single invalid/omitted index no longer discards the whole batch), increments
// the quota by the persisted count, and marks the job. When any indexes are
// missing/invalid the job is marked failed naming them and the returned
// missing list drives the bounded-retry state. A zero-valid response leaves
// missing = all indexes and persists nothing (full failure).
func (p *Processor) persistBatch(ctx context.Context, batch []*db.Comment, results []LLMResult, missingIndexes []int, primary *db.User, jobID string) (int, []int, error) {
	byIdx := make(map[int]LLMResult, len(results))
	for _, r := range results {
		byIdx[r.CommentIndex] = r
	}
	missingSet := make(map[int]struct{}, len(missingIndexes))
	for _, idx := range missingIndexes {
		missingSet[idx] = struct{}{}
	}

	persisted := 0
	for i, c := range batch {
		r, ok := byIdx[i]
		if !ok {
			missingSet[i] = struct{}{}
			continue
		}
		if err := p.Q.UpdateCommentLLMData(ctx, c.ID, r.Category, r.Topic, p.Client.ProviderName(), p.Client.ModelName(), mustMarshalInsights(r.ActionableInsights), r.Confidence); err != nil {
			return persisted, nil, fmt.Errorf("update comment %s: %w", c.ID, err)
		}
		if err := p.updateCommentSentiment(ctx, c.ID, SentimentScore(r.Sentiment)); err != nil {
			return persisted, nil, fmt.Errorf("update comment %s sentiment: %w", c.ID, err)
		}
		persisted++
	}

	missing := sortedInts(missingSet)

	// Charge only what was persisted; partial batches bill exactly once.
	if persisted > 0 {
		// Use the true post-batch total for overage so concurrent batches for
		// the same user (two channels, same owner) cannot bill from a stale
		// snapshot; fall back explicitly when the concrete method is absent.
		postUsed, err := p.incrementCommentsUsed(ctx, primary, persisted)
		if err != nil {
			return persisted, missing, fmt.Errorf("increment quota: %w", err)
		}
		// Overage logging (billing change, task 7.5). When this batch crosses
		// the user's quota boundary AND their plan tracks overage (creator,
		// studio), insert a comment_usage_logs row with the delta beyond the
		// quota.
		p.logOverage(ctx, primary, persisted, postUsed)
	}

	if len(missing) > 0 {
		msg := fmt.Sprintf("partial classification: missing/invalid comment indexes %v", missing)
		if err := p.Q.MarkLLMJobFailed(ctx, jobID, msg); err != nil {
			return persisted, missing, fmt.Errorf("mark job failed: %w", err)
		}
		return persisted, missing, nil
	}

	if err := p.Q.MarkLLMJobCompleted(ctx, jobID); err != nil {
		return persisted, missing, fmt.Errorf("mark job completed: %w", err)
	}
	return persisted, missing, nil
}

// recordBatchCost prices a completed batch's reported usage against the
// active price card and accumulates it on the primary's billing-period
// rollup (llm-cost-analytics 1.3). Attribution follows the quota-
// decrementing user, matching the overage rule. Non-fatal.
//
// ponytail: only successful classifications are billed; a failed batch
// still burned provider tokens but rolls up zero cost. Upgrade path:
// persist usage inside each provider client on every 2xx response if
// failed-attempt spend starts mattering.
func (p *Processor) recordBatchCost(ctx context.Context, primary *db.User, channelID string, usage TokenUsage) {
	cost := CostMicroUSD(p.Client.ProviderName(), p.Client.ModelName(), usage)
	recordCost(ctx, primary.ID, channelID, cost)
	periodStart, periodEnd := overagePeriod(primary, p.clock())
	persistLLMCost(ctx, p.Q, primary.ID, periodStart, periodEnd, cost)
}

// sortedInts returns the map's keys in ascending order for stable logs/tests.
func sortedInts(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// logOverage records usage beyond quota when the plan tracks overage.
// postUsed is the DB-authoritative post-batch total (see
// incrementCommentsUsed). Non-fatal: usage was already incremented; we log
// and continue so the job can complete.
func (p *Processor) logOverage(ctx context.Context, primary *db.User, batchLen, postUsed int) {
	if !billing.TracksOverage(primary.SubscriptionPlan) || primary.CommentQuotaMonthly <= 0 {
		return
	}
	preUsed := postUsed - batchLen
	delta := postUsed - max(primary.CommentQuotaMonthly, preUsed)
	if delta > 0 {
		p.recordOverageDelta(ctx, primary, delta, preUsed, postUsed)
	}
}

// incrementCommentsUsed adds n to the user's current-period usage and returns
// the true post-batch total. The DBQuery interface does not declare an
// increment-and-return method, so we type-assert the concrete SQLiteQuery
// (mirroring updateCommentSentiment). Mocks/test fakes that only implement
// DBQuery take the explicit fallback: increment, then derive the total from
// the caller's snapshot. That fallback is best-effort and only used when the
// atomic method is unavailable.
func (p *Processor) incrementCommentsUsed(ctx context.Context, primary *db.User, n int) (int, error) {
	if inc, ok := p.Q.(interface {
		IncrementCommentsUsedReturning(context.Context, string, int) (int, error)
	}); ok {
		return inc.IncrementCommentsUsedReturning(ctx, primary.ID, n)
	}
	if err := p.Q.IncrementCommentsUsed(ctx, primary.ID, n); err != nil {
		return 0, err
	}
	return primary.CommentsUsedThisPeriod + n, nil
}

func (p *Processor) recordOverageDelta(ctx context.Context, primary *db.User, delta, preUsed, postUsed int) {
	periodStart, periodEnd := overagePeriod(primary, p.clock())
	if err := p.Q.InsertCommentUsageLog(ctx, nil, primary.ID, delta, periodStart, periodEnd); err != nil {
		log.Printf("event=overage_log_failed user=%s delta=%d err=%v", primary.ID, delta, err)
		return
	}
	log.Printf("event=overage_logged user=%s plan=%s pre_used=%d post_used=%d quota=%d delta=%d",
		primary.ID, primary.SubscriptionPlan, preUsed, postUsed, primary.CommentQuotaMonthly, delta)
}

// overagePeriod bounds the usage log deterministically. It prefers the durable
// current_period_end anchor, then quota_reset_at; when neither is known it
// falls back to the current UTC calendar-month window derived from now, never
// from now itself. Two batches in the same period therefore always produce the
// same period_start/period_end, so ListOverageGroups cannot fragment a user's
// overage into separate rounded-up tiny groups.
func overagePeriod(primary *db.User, now time.Time) (time.Time, time.Time) {
	if primary != nil && primary.CurrentPeriodEnd != nil {
		periodEnd := primary.CurrentPeriodEnd.UTC()
		return periodEnd.AddDate(0, -1, 0), periodEnd
	}
	if primary != nil && primary.QuotaResetAt != nil {
		periodEnd := primary.QuotaResetAt.UTC()
		return periodEnd.AddDate(0, -1, 0), periodEnd
	}
	y, m, _ := now.UTC().Date()
	monthStart := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	return monthStart.Add(-time.Nanosecond), monthStart.AddDate(0, 1, 0).Add(-time.Nanosecond)
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
