package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"noumena/internal/auth"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/telemetry"
)

// ============================================================================
// Worker DB surface (narrow interface so tests can stub).
// ============================================================================

// WorkerDB is the set of DB operations the per-channel worker performs.
type WorkerDB interface {
	ClaimNextIngestionJob(ctx context.Context) (*db.IngestionJob, error)
	ReleaseJobToPending(ctx context.Context, jobID string, delay time.Duration) error
	MarkJobCompleted(ctx context.Context, jobID string) error
	MarkJobFailed(ctx context.Context, jobID, errMsg string, currentRetryCount int) error
	MarkJobDeadLettered(ctx context.Context, jobID, errMsg string) error

	UpdateLastSyncedAt(ctx context.Context, platformChannelID string, t time.Time) error
	UpdateLastSyncedAtIfNull(ctx context.Context, platformChannelID string) error
	UpdateIngestionError(ctx context.Context, platformChannelID, errMsg string, retryAfter time.Duration) error
	ResetIngestionError(ctx context.Context, platformChannelID string) error
	MarkChannelBackoff(ctx context.Context, platformChannelID string, until time.Time) error

	GetPlatformChannelByID(ctx context.Context, platformChannelID string) (*db.PlatformChannel, error)
	GetIngestionToken(ctx context.Context, platformChannelID string) (string, string, error)
	PromoteNextIngestionToken(ctx context.Context, platformChannelID string) error
	GetMaxBackfillDays(ctx context.Context, platformChannelID string) (int, error)

	InsertComments(ctx context.Context, comments []*db.Comment) (int, error)
}

// DeadLetterAlerter is invoked exactly once when a job is dead-lettered.
type DeadLetterAlerter interface {
	Alert(ctx context.Context, platformChannelID, jobID, errMsg string)
}

// LogAlerter is the default DeadLetterAlerter that just logs.
type LogAlerter struct{}

func (LogAlerter) Alert(_ context.Context, platformChannelID, jobID, errMsg string) {
	log.Printf("ALERT dead-letter channel=%s job=%s err=%s", platformChannelID, jobID, errMsg)
}

// ============================================================================
// Worker config
// ============================================================================

const (
	maxRetries           = 5
	defaultJobDeadline   = 5 * time.Minute
	lockBusyRescheduleIn = 30 * time.Second
	commentBatchSize     = 100
	deadLetterBackoff    = 1 * time.Hour
)

// Worker drives the job queue: claims jobs, executes them, and reports
// outcome back to the DB. It is safe to run multiple Worker instances
// (across processes and goroutines) — Postgres FOR UPDATE SKIP LOCKED
// guarantees mutual exclusion on claims and pg_try_advisory_lock guarantees
// per-channel mutual exclusion across the cluster.
type Worker struct {
	DB             WorkerDB
	Pool           *sql.DB // kept for compatibility; not used for SQLite
	Client         *YouTubeClient
	Alerter        DeadLetterAlerter
	JobDeadline    time.Duration
	MaxConcurrency int
	PollEmptyDelay time.Duration // sleep between empty claims
}

// NewWorker returns a Worker with sensible defaults.
func NewWorker(db WorkerDB, pool *sql.DB) *Worker {
	return &Worker{
		DB:             db,
		Pool:           pool,
		Client:         NewYouTubeClient(),
		Alerter:        LogAlerter{},
		JobDeadline:    defaultJobDeadline,
		MaxConcurrency: 8,
		PollEmptyDelay: 1 * time.Second,
	}
}

// Run claims jobs in a loop until ctx is cancelled. Concurrent execution
// is bounded by MaxConcurrency.
func (w *Worker) Run(ctx context.Context) {
	if w.MaxConcurrency < 1 {
		w.MaxConcurrency = 1
	}
	sem := make(chan struct{}, w.MaxConcurrency)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		default:
		}

		job, err := w.DB.ClaimNextIngestionJob(ctx)
		if err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				return
			}
			if err != db.ErrNotFound {
				log.Printf("worker: claim error: %v", err)
				sleepCtx(ctx, 5*time.Second)
				continue
			}
			// No pending jobs — poll normally.
			sleepCtx(ctx, w.PollEmptyDelay)
			continue
		}
		if job == nil {
			sleepCtx(ctx, w.PollEmptyDelay)
			continue
		}

		// Reserve a concurrency slot.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			// Return the job so another worker can pick it up.
			_ = w.DB.ReleaseJobToPending(context.Background(), job.ID, 0)
			wg.Wait()
			return
		}

		wg.Add(1)
		go func(j *db.IngestionJob) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("worker: PANIC job=%s channel=%s: %v", j.ID, j.PlatformChannelID, r)
					_ = w.DB.MarkJobFailed(context.Background(), j.ID, fmt.Sprintf("panic: %v", r), j.RetryCount)
				}
			}()
			w.processJob(ctx, j)
		}(job)
	}
}

// ============================================================================
// processJob — the heart of the per-channel state machine.
// ============================================================================

func (w *Worker) processJob(parent context.Context, job *db.IngestionJob) {
	jobCtx, cancel := context.WithTimeout(parent, w.JobDeadline)
	defer cancel()

	// otel-instrumentation: record fetch_duration on every return
	// path. The outcome is mutated by the closure below as we learn
	// the result (success / error / timeout). The deferred Record
	// reads the final value once just before return.
	start := time.Now()
	outcome := telemetry.OutcomeSuccess
	defer func() {
		recordFetchDuration(parent, start, outcome)
	}()

	// 1. Acquire per-channel advisory lock.
	acquired, release, err := AcquireChannelLock(jobCtx, w.Pool, job.PlatformChannelID)
	if err != nil {
		log.Printf("worker: lock acquire failed job=%s channel=%s: %v", job.ID, job.PlatformChannelID, err)
		_ = w.DB.ReleaseJobToPending(parent, job.ID, lockBusyRescheduleIn)
		outcome = telemetry.OutcomeError
		return
	}
	if !acquired {
		// Another worker is processing this channel; yield without consuming retry budget.
		// Spec: lock contention is NOT an error — record it on its
		// own counter and treat the outcome as success (we did
		// what we were supposed to: defer to the lock holder).
		recordLockContention(parent)
		_ = w.DB.ReleaseJobToPending(parent, job.ID, lockBusyRescheduleIn)
		return
	}
	defer release()

	// 2. Load channel + token.
	pc, err := w.DB.GetPlatformChannelByID(jobCtx, job.PlatformChannelID)
	if err != nil {
		w.failJob(parent, job, fmt.Errorf("load channel: %w", err))
		outcome = telemetry.OutcomeError
		recordIngestionError(parent, telemetry.ReasonOther)
		return
	}
	if pc.Platform != "youtube" {
		// Mark completed; we don't currently support other platforms.
		_ = w.DB.MarkJobCompleted(parent, job.ID)
		return
	}

	accessTok, err := w.obtainAccessToken(jobCtx, job.PlatformChannelID)
	if err != nil {
		// "no token" → job is a noop; mark completed without raising an error
		// on the channel (the channel-management layer surfaces missing tokens).
		if errors.Is(err, errNoToken) {
			// Initialize last_synced_at if it was NULL so we don't keep
			// re-enqueuing this channel as a backfill on every tick.
			_ = w.DB.UpdateLastSyncedAtIfNull(parent, job.PlatformChannelID)
			_ = w.DB.MarkJobCompleted(parent, job.ID)
			return
		}
		w.failJob(parent, job, err)
		outcome = telemetry.OutcomeError
		recordIngestionError(parent, classifyIngestionError(err))
		return
	}

	// 3. Compute publishedAfter window.
	publishedAfter := pc.LastSyncedAt
	if job.JobType == "backfill" || pc.LastSyncedAt == nil {
		days, derr := w.DB.GetMaxBackfillDays(jobCtx, job.PlatformChannelID)
		if derr != nil {
			days = 90
		}
		t := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		publishedAfter = &t
	}

	// 4. Iterate videos → comment threads, batching inserts and checkpointing.
	if err := w.runIngestion(jobCtx, parent, pc, accessTok, publishedAfter, job); err != nil {
		// Categorize.
		var rate *RateLimitError
		var tokInv *TokenInvalidError
		switch {
		case errors.As(err, &rate):
			until := time.Now().Add(rate.RetryAfter)
			_ = w.DB.MarkChannelBackoff(parent, job.PlatformChannelID, until)
			_ = w.DB.MarkJobCompleted(parent, job.ID) // rate-limit is NOT a failure
			recordIngestionError(parent, telemetry.ReasonRateLimited)
			outcome = telemetry.OutcomeError
			return
		case errors.As(err, &tokInv):
			// Try fallback token: promote next, then retry once without consuming retry budget.
			recordIngestionError(parent, telemetry.ReasonTokenInvalid)
			outcome = telemetry.OutcomeError
			if perr := w.DB.PromoteNextIngestionToken(parent, job.PlatformChannelID); perr == nil {
				_ = w.DB.ReleaseJobToPending(parent, job.ID, 5*time.Second)
				return
			}
			// No fallback available; mark completed and move on.
			_ = w.DB.UpdateLastSyncedAtIfNull(parent, job.PlatformChannelID)
			_ = w.DB.MarkJobCompleted(parent, job.ID)
			return
		case errors.Is(err, context.DeadlineExceeded):
			// Soft deadline: not a failure, the next tick will re-enqueue.
			_ = w.DB.MarkJobCompleted(parent, job.ID)
			recordIngestionError(parent, telemetry.ReasonDeadlineExceeded)
			outcome = telemetry.OutcomeTimeout
			return
		}
		w.failJob(parent, job, err)
		recordIngestionError(parent, classifyIngestionError(err))
		outcome = telemetry.OutcomeError
		return
	}

	// 5. Success.
	if err := w.DB.MarkJobCompleted(parent, job.ID); err != nil {
		log.Printf("worker: mark completed job=%s: %v", job.ID, err)
	}
	_ = w.DB.ResetIngestionError(parent, job.PlatformChannelID)
	_ = w.DB.UpdateLastSyncedAt(parent, job.PlatformChannelID, time.Now())
}

// runIngestion paginates videos and comment threads, batches inserts, and
// checkpoints `last_synced_at` after each batch.
func (w *Worker) runIngestion(jobCtx, persistCtx context.Context, pc *db.PlatformChannel, accessTok string, publishedAfter *time.Time, job *db.IngestionJob) error {
	var pa time.Time
	if publishedAfter != nil {
		pa = *publishedAfter
	}
	nextVideo := w.Client.ListVideos(jobCtx, accessTok, pc.PlatformChannelID, pa)

	batch := make([]*db.Comment, 0, commentBatchSize)
	var maxPostedAt time.Time

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		n, err := w.DB.InsertComments(persistCtx, batch)
		if err != nil {
			return fmt.Errorf("insert comments: %w", err)
		}
		// otel-instrumentation §10.5: increment by the count
		// actually persisted (InsertComments may dedupe).
		recordCommentsIngested(persistCtx, n)
		if !maxPostedAt.IsZero() {
			_ = w.DB.UpdateLastSyncedAt(persistCtx, pc.ID, maxPostedAt)
		}
		batch = batch[:0]
		return nil
	}

	for {
		// Honor soft deadline: stop cleanly with whatever we have.
		if jobCtx.Err() != nil {
			if err := flush(); err != nil {
				return err
			}
			return jobCtx.Err()
		}

		v, ok, err := nextVideo()
		if err != nil {
			return err
		}
		if !ok {
			break
		}

		nextThread := w.Client.ListCommentThreads(jobCtx, accessTok, v.ID)
		for {
			if jobCtx.Err() != nil {
				if err := flush(); err != nil {
					return err
				}
				return jobCtx.Err()
			}
			t, ok, err := nextThread()
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			c := t.ToDBComment(pc.ID)
			batch = append(batch, c)
			if t.PostedAt.After(maxPostedAt) {
				maxPostedAt = t.PostedAt
			}
			if len(batch) >= commentBatchSize {
				if err := flush(); err != nil {
					return err
				}
			}

			// Paginate through deep replies for this thread.
			if t.ParentCommentID == nil && t.ReplyDepth == 0 {
				nextReply := w.Client.ListReplies(jobCtx, accessTok, t.CommentID)
				for {
					if jobCtx.Err() != nil {
						break
					}
					r, rok, rerr := nextReply()
					if rerr != nil {
						break
					}
					if !rok {
						break
					}
					rc := r.ToDBComment(pc.ID)
					batch = append(batch, rc)
					if r.PostedAt.After(maxPostedAt) {
						maxPostedAt = r.PostedAt
					}
					if len(batch) >= commentBatchSize {
						if err := flush(); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return flush()
}

// failJob routes a generic error to MarkJobFailed or MarkJobDeadLettered
// based on retry budget, and on dead-letter parks the channel for 1h and
// emits an alert.
func (w *Worker) failJob(ctx context.Context, job *db.IngestionJob, err error) {
	msg := err.Error()
	if job.RetryCount >= maxRetries {
		_ = w.DB.MarkJobDeadLettered(ctx, job.ID, msg)
		_ = w.DB.MarkChannelBackoff(ctx, job.PlatformChannelID, time.Now().Add(deadLetterBackoff))
		_ = w.DB.UpdateIngestionError(ctx, job.PlatformChannelID, msg, deadLetterBackoff)
		// otel-instrumentation: dead-letter is a distinct,
		// auditable outcome — emit it on top of the upstream
		// reason classification so dashboards can split
		// "transient errors" from "exhausted retry budget".
		recordIngestionError(ctx, telemetry.ReasonDeadLettered)
		if w.Alerter != nil {
			w.Alerter.Alert(ctx, job.PlatformChannelID, job.ID, msg)
		}
		log.Printf("worker: dead-letter job=%s channel=%s err=%v", job.ID, job.PlatformChannelID, err)
		return
	}
	_ = w.DB.MarkJobFailed(ctx, job.ID, msg, job.RetryCount)
	log.Printf("worker: retry job=%s channel=%s retry=%d err=%v", job.ID, job.PlatformChannelID, job.RetryCount+1, err)
}

// ============================================================================
// Token plumbing — decrypt + refresh via oauth2.TokenSource.
// ============================================================================

var errNoToken = errors.New("no active ingestion token")

func (w *Worker) obtainAccessToken(ctx context.Context, platformChannelID string) (string, error) {
	accessEnc, refreshEnc, err := w.DB.GetIngestionToken(ctx, platformChannelID)
	if err != nil {
		// Treat any error here as "no token available" for the worker. The
		// channel-management spec governs surfacing this state to users.
		return "", errNoToken
	}
	access, err := auth.Decrypt(accessEnc)
	if err != nil {
		return "", fmt.Errorf("decrypt access token: %w", err)
	}
	refresh, err := auth.Decrypt(refreshEnc)
	if err != nil {
		return "", fmt.Errorf("decrypt refresh token: %w", err)
	}

	cfg := &oauth2.Config{
		ClientID:     config.Default.YouTubeOAuthClientID,
		ClientSecret: config.Default.YouTubeOAuthClientSecret,
		RedirectURL:  config.Default.YouTubeOAuthRedirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/youtube.readonly"},
		Endpoint:     google.Endpoint,
	}
	tok := &oauth2.Token{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		// Force a refresh by setting Expiry to past; the TokenSource will
		// only refresh if needed.
	}
	ts := cfg.TokenSource(ctx, tok)
	fresh, err := ts.Token()
	if err != nil {
		// Refresh failed → treat as token-invalid so the worker promotes
		// the next ingestion token.
		return "", &TokenInvalidError{StatusCode: 401, Body: err.Error()}
	}
	return fresh.AccessToken, nil
}

func sleepCtx(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// ============================================================================
// Legacy `StartWorker` retained for backwards compatibility with
// cmd/api/main.go callers. It now drives the new job-queue worker.
// ============================================================================

// StartWorker is the legacy entrypoint. It now constructs and runs both the
// scheduler and the job-queue worker against the provided DB. The
// `q db.DBQuery` parameter is preserved for source compatibility.
//
// Callers that want fine-grained control should use NewWorker / NewScheduler
// directly from cmd/api/main.go.
func StartWorker(ctx context.Context, q db.DBQuery) {
	sq, ok := q.(*db.SQLiteQuery)
	if !ok {
		log.Println("ingestion: StartWorker requires *db.SQLiteQuery; legacy path disabled")
		return
	}
	pool := sq.DB()
	if pool == nil {
		log.Println("ingestion: StartWorker requires a database connection; legacy path disabled")
		return
	}

	scheduler := NewScheduler(sq, 30)
	worker := NewWorker(sq, pool)

	go scheduler.Run(ctx, 30*time.Second)
	worker.Run(ctx)
}
