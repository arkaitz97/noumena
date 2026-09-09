// Package llm: digest_worker.go houses the weekly-digest cron pipeline.
//
// Lifecycle:
//  1. Worker.Start spins a 60-second ticker; every tick, shouldTrigger
//     evaluates the wall clock against (Monday, 08:00 UTC).
//  2. dispatch fans per-user jobs into a bounded worker pool of size
//     min(8, GOMAXPROCS) (Spec 06: digest worker concurrency bounds).
//  3. runUserDigest is the per-user pipeline:
//     InsertWeeklyDigestPending  →  fetch comments  →
//     skip-if-<10  →  compute aggregates  →  retry-chained LLM call →
//     UpdateDigestContent (completed) | UpdateDigestStatus (deferred)
//  4. On Start, recoverStalePending sweeps any pending row older than 2 h
//     and promotes it to deferred (recovery from a process restart that
//     left an in-flight retry chain orphaned).
//
// The whole pipeline is gated by DIGEST_WORKER_ENABLED=true so deployments
// can roll out the schema change first without firing the worker.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
)

// Spec 06 trigger constants. Held as variables (not consts) so tests with
// fake clocks can adjust the trigger window if needed; production code
// always uses the documented Monday-08:00-UTC schedule.
var (
	digestTriggerWeekday = time.Monday
	digestTriggerHour    = 8
	digestTriggerMinute  = 0
)

// digestRetrySchedule is the cumulative (from t0) wait list for the
// retry-chained LLM attempts: attempt 1 immediate, attempt 2 at +5m,
// attempt 3 at +20m. Total budget ≈ 25 minutes (well under the 2-hour
// resilience target documented in design.md D5; the longer 50m alternative
// from D5 is rejected here in favour of a tighter budget that fits inside
// the same dispatch tick).
var digestRetrySchedule = []time.Duration{
	0,
	5 * time.Minute,
	15 * time.Minute, // delta from previous = 15m → cumulative 20m
}

// digestSkipThreshold is the eligible-comment cutoff below which we skip
// LLM summarisation and persist status='skipped_insufficient_data'
// (Spec 06: insufficient-data skip).
const digestSkipThreshold = 10

// digestStaleAge is the cutoff for the startup recovery sweep: any pending
// row older than this is promoted to deferred (Spec 06 EC: recovery of
// stale pending rows on startup).
const digestStaleAge = 2 * time.Hour

// dispatchDeadline bounds one user's synchronous pipeline so a stuck user
// can't stall the run loop; failures become deferred rows retried async.
const dispatchDeadline = 60 * time.Second

const deferredRedispatchLimit = 50

// Worker owns the digest pipeline state. A single instance per process is
// the expected deployment; the Postgres unique index on
// (user_id, week_start_date) makes accidental concurrent dispatch safe but
// wasteful.
type Worker struct {
	Queries     db.DBQuery
	LLMClient   *opencodeClient
	EmailClient email.Client // optional; reserved for Spec 08
	// Pipeline is the data-lifecycle (12.6) email path. When set AND the
	// user has digest_email_opt_in=true, the worker emits
	// TemplateWeeklyDigest after successfully writing weekly_digests
	// content. On send success the row's digest_email_sent_at is updated.
	// Nil ⇒ skip silently (legacy/mock environments).
	Pipeline *email.Pipeline
	Logger   *log.Logger

	// Now is the wall-clock function. Tests inject a fake clock to drive
	// shouldTrigger deterministically; production leaves it nil and the
	// worker uses time.Now().UTC() under the hood.
	Now func() time.Time

	// TickInterval defaults to 1 minute. Tests may shrink it.
	TickInterval time.Duration

	// dispatchedWeeks tracks (userID, weekStart) tuples already dispatched
	// in the current process to suppress repeats inside the same trigger
	// minute. The DB unique index is the durable enforcement mechanism;
	// this map only saves the round-trip when the trigger fires twice in
	// the same minute (e.g. clock skew across container restart).
	dispatchedMu sync.Mutex
	dispatched   map[string]struct{}

	// stop coordinates Stop() with the run loop. cancel cancels the
	// context handed to Start; wg waits for all in-flight goroutines.
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Start spawns the worker's main loop. Returns immediately. Stop blocks
// until all in-flight goroutines (including scheduled retry chains)
// observe context cancellation and exit.
func (w *Worker) Start(parent context.Context) {
	if w.Logger == nil {
		w.Logger = log.Default()
	}
	if w.Now == nil {
		w.Now = func() time.Time { return time.Now().UTC() }
	}
	if w.TickInterval == 0 {
		w.TickInterval = 1 * time.Minute
	}
	w.dispatched = map[string]struct{}{}

	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel

	if !digestEnabled() {
		w.Logger.Println("digest worker: disabled (DIGEST_WORKER_ENABLED!=true) — Start is a no-op")
		return
	}

	// Startup recovery sweep runs synchronously before the loop so the
	// first trigger after a restart sees a clean table.
	w.recoverStalePending(ctx)
	w.catchUpMissedWeeks(ctx)
	w.redispatchDeferred(ctx)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.run(ctx)
	}()
}

// Stop cancels the worker context and waits for graceful shutdown.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

// digestEnabled reads the rollout flag at Start time. Read-once is fine;
// flipping the env var requires a restart, which is the documented
// operational expectation (design.md Migration Plan, step 2).
func digestEnabled() bool {
	v := os.Getenv("DIGEST_WORKER_ENABLED")
	return v == "true" || v == "1" || v == "yes"
}

// run is the minute-granularity ticker. shouldTrigger gates dispatch on the
// Mon-08:00-UTC window; deferred re-dispatch runs every tick (due filter is cheap).
func (w *Worker) run(ctx context.Context) {
	t := time.NewTicker(w.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			w.Logger.Println("digest worker: stopping")
			return
		case <-t.C:
			w.redispatchDeferred(ctx)
			now := w.Now()
			if !shouldTrigger(now) {
				continue
			}
			weekStart := computeWeekStart(now)
			w.Logger.Printf("digest worker: trigger fired at %s, week_start=%s",
				now.Format(time.RFC3339), weekStart.Format("2006-01-02"))
			w.dispatch(ctx, weekStart)
		}
	}
}

// shouldTrigger returns true exactly once per trigger minute (Mon 08:00 UTC).
// All comparisons are in UTC; never use t.Local() or any local-time variant.
func shouldTrigger(now time.Time) bool {
	n := now.UTC()
	return n.Weekday() == digestTriggerWeekday &&
		n.Hour() == digestTriggerHour &&
		n.Minute() == digestTriggerMinute
}

// computeWeekStart returns the previous Monday 00:00 UTC relative to now.
// At trigger time T (Mon 08:00 UTC week N) this is week N-1's Monday — the
// boundary documented in design.md D2.
func computeWeekStart(now time.Time) time.Time {
	n := now.UTC()
	// Previous Monday: subtract a full 7 days from this Monday's midnight.
	thisMonday := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
	return thisMonday.AddDate(0, 0, -7)
}

// dispatch fans GetUsersForDigest into a bounded worker pool. Each user is
// processed independently; an error or panic in one job does not affect the
// others.
func (w *Worker) dispatch(ctx context.Context, weekStart time.Time) {
	users, err := w.Queries.GetUsersForDigest(ctx)
	if err != nil {
		w.Logger.Printf("digest worker: GetUsersForDigest failed: %v", err)
		return
	}
	if len(users) == 0 {
		w.Logger.Println("digest worker: no eligible users this week")
		return
	}
	w.Logger.Printf("digest worker: dispatching %d users", len(users))

	poolSize := runtime.GOMAXPROCS(0)
	if poolSize > 8 {
		poolSize = 8
	}
	if poolSize < 1 {
		poolSize = 1
	}

	weekEnd := weekStart.AddDate(0, 0, 7)
	jobs := make(chan *db.User)
	var wg sync.WaitGroup
	for i := 0; i < poolSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				if ctx.Err() != nil {
					return
				}
				w.runUserDigest(ctx, u, weekStart, weekEnd)
			}
		}()
	}
	for _, u := range users {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- u:
		}
	}
	close(jobs)
	wg.Wait()
}

// runUserDigest is the per-user pipeline. Idempotent (the pending insert
// is the gate). Logs at every transition for operational visibility.
func (w *Worker) runUserDigest(ctx context.Context, user *db.User, weekStart, weekEnd time.Time) {
	if w.Logger == nil {
		w.Logger = log.Default()
	}
	key := user.ID + ":" + weekStart.Format("2006-01-02")
	w.dispatchedMu.Lock()
	if w.dispatched == nil {
		w.dispatched = map[string]struct{}{}
	}
	if _, seen := w.dispatched[key]; seen {
		w.dispatchedMu.Unlock()
		return
	}
	w.dispatched[key] = struct{}{}
	w.dispatchedMu.Unlock()

	digestID, inserted, err := w.Queries.InsertWeeklyDigestPending(ctx, user.ID, weekStart)
	if err != nil {
		w.Logger.Printf("digest worker: pending insert failed for user %s: %v", user.ID, err)
		return
	}
	if !inserted {
		// A row already exists — either completed earlier or another
		// worker/process is handling it. Either way, nothing for us to do.
		w.Logger.Printf("digest worker: skip user=%s week=%s (already exists, id=%s)",
			user.ID, weekStart.Format("2006-01-02"), digestID)
		return
	}

	comments, err := w.Queries.GetEligibleCommentsForUser(ctx, user.ID, weekStart, weekEnd)
	if err != nil {
		w.Logger.Printf("digest worker: fetch comments failed user=%s: %v", user.ID, err)
		// Leave row pending; the next startup sweep promotes it to deferred.
		return
	}

	if len(comments) < digestSkipThreshold {
		w.Logger.Printf("digest worker: skip user=%s week=%s (count=%d < %d)",
			user.ID, weekStart.Format("2006-01-02"), len(comments), digestSkipThreshold)
		if err := w.Queries.UpdateDigestContent(ctx, digestID, nil, "skipped_insufficient_data"); err != nil {
			w.Logger.Printf("digest worker: status update failed user=%s: %v", user.ID, err)
		}
		return
	}

	truth := w.computeTruth(ctx, user, weekStart, comments)

	prompt := BuildDigestPrompt(DigestPromptInputs{
		Aggregates:     truth,
		TopTopics:      computeTopTopics(comments, 10),
		CategoryCounts: computeCategoryDistribution(comments),
		SampleComments: selectSampleComments(comments, digestSampleLimit),
	})

	// Per-user deadline so one stuck LLM chain can't stall the run loop.
	// Retries beyond the deadline become deferred rows handled async.
	userCtx, cancel := context.WithTimeout(ctx, dispatchDeadline)
	defer cancel()
	parsed, attemptErr := w.callWithRetry(userCtx, prompt, truth)
	if attemptErr != nil {
		w.Logger.Printf("digest worker: deferred user=%s week=%s: %v",
			user.ID, weekStart.Format("2006-01-02"), attemptErr)
		if err := w.Queries.UpdateDigestStatus(ctx, digestID, "deferred"); err != nil {
			w.Logger.Printf("digest worker: defer-status update failed user=%s: %v", user.ID, err)
		}
		if w.Pipeline != nil && user.DigestEmailOptIn {
			msg := email.Message{
				ToUserID:       user.ID,
				ToAddress:      user.Email,
				Template:       "digest_delayed",
				IdempotencyKey: "digest_delayed:" + digestID,
				Variables: map[string]any{
					"WeekRange":      weekStart.Format("Jan 2") + " – " + weekStart.AddDate(0, 0, 6).Format("Jan 2, 2006"),
					"UnsubscribeURL": digestUnsubscribeURL(user.ID, digestID),
				},
				ListUnsubscribe: digestUnsubscribeURL(user.ID, digestID),
			}
			if err := w.Pipeline.Send(ctx, msg); err != nil {
				w.Logger.Printf("digest worker: digest_delayed email failed user=%s: %v", user.ID, err)
			}
		}
		return
	}

	contentJSON, err := json.Marshal(parsed)
	if err != nil {
		w.Logger.Printf("digest worker: marshal failed user=%s: %v", user.ID, err)
		_ = w.Queries.UpdateDigestStatus(ctx, digestID, "deferred")
		return
	}
	if err := w.Queries.UpdateDigestContent(ctx, digestID, contentJSON, "completed"); err != nil {
		w.Logger.Printf("digest worker: completed-update failed user=%s: %v", user.ID, err)
		return
	}
	w.Logger.Printf("digest worker: completed user=%s week=%s id=%s",
		user.ID, weekStart.Format("2006-01-02"), digestID)

	// data-lifecycle (12.6): emit weekly_digest email when the user has
	// opted in. Pipeline gate is checked first so non-pipeline deployments
	// (mock mode, EMAIL_ENABLED=false) skip cleanly. Idempotency key is
	// the digest ID, so retried runs collapse to one delivery.
	if w.Pipeline != nil && user.DigestEmailOptIn {
		weekRange := weekStart.Format("Jan 2") + " – " +
			weekStart.AddDate(0, 0, 6).Format("Jan 2, 2006")
		unsubURL := digestUnsubscribeURL(user.ID, digestID)
		msg := email.Message{
			ToUserID:       user.ID,
			ToAddress:      user.Email,
			Template:       email.TemplateWeeklyDigest,
			IdempotencyKey: "weekly_digest:" + digestID,
			Variables: map[string]any{
				"WeekRange":      weekRange,
				"DigestURL":      strings.TrimRight(config.Default.AppBaseURL, "/") + "/dashboard",
				"TopIdeas":       digestIdeaStrings(parsed.TopIdeas),
				"TopQuestions":   digestQuestionStrings(parsed.TopQuestions),
				"UnsubscribeURL": unsubURL,
			},
			ListUnsubscribe: unsubURL,
		}
		if err := w.Pipeline.Send(ctx, msg); err != nil {
			w.Logger.Printf("digest worker: weekly_digest email enqueue failed user=%s id=%s: %v",
				user.ID, digestID, err)
			return
		}
		if err := w.Queries.MarkDigestEmailed(ctx, digestID, user.ID, time.Now().UTC()); err != nil {
			w.Logger.Printf("digest worker: MarkDigestEmailed failed user=%s id=%s: %v",
				user.ID, digestID, err)
		}
	}
}

// digestIdeaStrings flattens LLM-synthesised ideas into the template's
// string list. Evidence is appended when present so the email carries the
// reasoning, not just the headline.
func digestIdeaStrings(ideas []DigestIdea) []string {
	out := make([]string, 0, len(ideas))
	for _, i := range ideas {
		line := strings.TrimSpace(i.Idea)
		if e := strings.TrimSpace(i.Evidence); e != "" {
			line += " — " + e
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func digestQuestionStrings(questions []DigestQuestion) []string {
	out := make([]string, 0, len(questions))
	for _, q := range questions {
		if s := strings.TrimSpace(q.Question); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// digestUnsubscribeURL builds a self-authenticating unsubscribe link
// (HMAC-signed, reusing email/signer.go) that flips the user's digest
// opt-in off without any login. The digest ID plays the canonical
// "jobID" slot in the signature.
func digestUnsubscribeURL(userID, digestID string) string {
	exp := time.Now().UTC().AddDate(1, 0, 0)
	q := email.SignURL(config.Default.ExportSignedURLSecret, userID, digestID, exp)
	return strings.TrimRight(config.Default.AppBaseURL, "/") +
		"/unsubscribe?user_id=" + url.QueryEscape(userID) +
		"&job=" + url.QueryEscape(digestID) + "&" + q
}

// computeTruth builds the authoritative numeric DigestContent fields. This
// is the value handed to CallDigestLLM as the truth payload that overwrites
// the LLM's echoed numbers.
func (w *Worker) computeTruth(ctx context.Context, user *db.User, weekStart time.Time, comments []*db.Comment) DigestContent {
	breakdown := HealthScoreBreakdown{
		ResponseRate:    computeResponseRate(comments),
		EngagementDepth: computeEngagementDepth(comments),
	}
	topInsights := computeTopActionableInsights(comments, 5)
	return DigestContent{
		TopActionableInsights: topInsights,
		EngagementHealthScore: computeHealthScore(breakdown),
		HealthScoreBreakdown:  breakdown,
		TotalCommentsAnalyzed: len(comments),
		FilteredComments:      0, // toxic + spam exclusion happens in SQL; counter reserved for future use.
	}
}

// callWithRetry implements the digestRetrySchedule. Each attempt waits
// (delta from previous element) before firing, giving the chain its
// cumulative budget. Context cancellation aborts the chain immediately.
func (w *Worker) callWithRetry(ctx context.Context, prompt string, truth DigestContent) (DigestContent, error) {
	var lastErr error
	prev := time.Duration(0)
	for i, when := range digestRetrySchedule {
		delta := when - prev
		prev = when
		if delta > 0 {
			select {
			case <-ctx.Done():
				return DigestContent{}, ctx.Err()
			case <-time.After(delta):
			}
		}
		out, err := CallDigestLLM(ctx, w.LLMClient, prompt, truth)
		if err == nil {
			return out, nil
		}
		lastErr = err
		w.Logger.Printf("digest worker: LLM attempt %d/%d failed: %v", i+1, len(digestRetrySchedule), err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// Stop retrying once the parent cancels; the per-call timeout
			// uses its own derived context, so a per-call deadline does
			// not poison the parent and we keep going on those.
			if ctx.Err() != nil {
				return DigestContent{}, err
			}
		}
	}
	return DigestContent{}, lastErr
}

// recoverStalePending implements the Spec 06 startup recovery scenario.
func (w *Worker) recoverStalePending(ctx context.Context) {
	cutoff := w.Now().Add(-digestStaleAge)
	stale, err := w.Queries.GetStalePendingDigests(ctx, cutoff)
	if err != nil {
		w.Logger.Printf("digest worker: stale sweep query failed: %v", err)
		return
	}
	for _, d := range stale {
		if err := w.Queries.UpdateDigestStatus(ctx, d.ID, "deferred"); err != nil {
			w.Logger.Printf("digest worker: stale promote failed id=%s: %v", d.ID, err)
			continue
		}
		w.Logger.Printf("digest worker: promoted stale pending id=%s user=%s week=%s → deferred",
			d.ID, d.UserID, d.WeekStartDate)
	}
}

// catchUpMissedWeeks dispatches the most recent weeks whose Monday 08:00
// trigger passed while the process was down. Idempotent via pending insert gate.
func (w *Worker) catchUpMissedWeeks(ctx context.Context) {
	now := w.Now()
	base := computeWeekStart(now)
	for k := 0; k < 3; k++ {
		ws := base.AddDate(0, 0, -7*k)
		trigger := ws.AddDate(0, 0, 7).Add(8 * time.Hour)
		if now.Before(trigger) {
			continue
		}
		if shouldTrigger(now) && k == 0 {
			continue
		}
		w.Logger.Printf("digest worker: catch-up week_start=%s", ws.Format("2006-01-02"))
		w.dispatch(ctx, ws)
	}
}

// redispatchDeferred retries deferred rows due per backoff schedule.
func (w *Worker) redispatchDeferred(ctx context.Context) {
	due, err := w.Queries.GetDeferredDigestsDue(ctx, w.Now(), deferredRedispatchLimit)
	if err != nil {
		w.Logger.Printf("digest worker: deferred query failed: %v", err)
		return
	}
	for _, d := range due {
		if ctx.Err() != nil {
			return
		}
		w.retryDeferred(ctx, d)
	}
}

func (w *Worker) retryDeferred(ctx context.Context, d *db.WeeklyDigest) {
	user, err := w.Queries.GetUserByID(ctx, d.UserID)
	if err != nil || user == nil {
		w.Logger.Printf("digest worker: deferred retry skip id=%s: user lookup failed", d.ID)
		return
	}
	weekStart, err := time.Parse("2006-01-02", d.WeekStartDate)
	if err != nil {
		return
	}
	weekStart = weekStart.UTC()
	weekEnd := weekStart.AddDate(0, 0, 7)
	comments, err := w.Queries.GetEligibleCommentsForUser(ctx, user.ID, weekStart, weekEnd)
	if err != nil || len(comments) < digestSkipThreshold {
		if err == nil {
			_ = w.Queries.UpdateDigestContent(ctx, d.ID, nil, "skipped_insufficient_data")
		}
		return
	}
	truth := w.computeTruth(ctx, user, weekStart, comments)
	prompt := BuildDigestPrompt(DigestPromptInputs{
		Aggregates:     truth,
		TopTopics:      computeTopTopics(comments, 10),
		CategoryCounts: computeCategoryDistribution(comments),
		SampleComments: selectSampleComments(comments, digestSampleLimit),
	})
	retryCtx, cancel := context.WithTimeout(ctx, dispatchDeadline)
	defer cancel()
	parsed, err := w.callWithRetry(retryCtx, prompt, truth)
	if err != nil {
		w.Logger.Printf("digest worker: deferred retry failed id=%s: %v", d.ID, err)
		return
	}
	contentJSON, err := json.Marshal(parsed)
	if err != nil {
		return
	}
	if err := w.Queries.UpdateDigestContent(ctx, d.ID, contentJSON, "completed"); err != nil {
		w.Logger.Printf("digest worker: deferred completed-update failed id=%s: %v", d.ID, err)
	}
}
