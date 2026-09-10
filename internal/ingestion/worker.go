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
	RecoverStaleIngestionJobs(ctx context.Context, olderThan time.Duration) (int64, error)

	UpdateLastSyncedAt(ctx context.Context, platformChannelID string, t time.Time) error
	UpdateLastSyncedAtIfNull(ctx context.Context, platformChannelID string) error
	UpdateIngestionError(ctx context.Context, platformChannelID, errMsg string, retryAfter time.Duration) error
	ResetIngestionError(ctx context.Context, platformChannelID string) error
	MarkChannelBackoff(ctx context.Context, platformChannelID string, until time.Time) error

	GetPlatformChannelByID(ctx context.Context, platformChannelID string) (*db.PlatformChannel, error)
	GetIngestionToken(ctx context.Context, platformChannelID string) (string, string, error)
	GetIngestionTokenRow(ctx context.Context, platformChannelID string) (*db.UserChannelTracking, bool, error)
	UpdateTokenAndExpiry(ctx context.Context, trackingID, encryptedAccess, encryptedRefresh string, expiresAt time.Time) error
	PromoteNextIngestionToken(ctx context.Context, platformChannelID string) error
	GetMaxBackfillDays(ctx context.Context, platformChannelID string) (int, error)

	InsertComments(ctx context.Context, comments []*db.Comment) (int, error)
}

// SyncStateDB is the optional DB surface for quota-optimization state
// (optimize-youtube-api-quota): the uploads-playlist cache, per-video
// incremental checkpoints, and the adaptive poll interval. Workers fall
// back to stateless behavior when the underlying DB does not implement it.
type SyncStateDB interface {
	SetUploadsPlaylistID(ctx context.Context, platformChannelID, playlistID string) error
	SetPollInterval(ctx context.Context, platformChannelID string, minutes int) error
	GetVideoSyncState(ctx context.Context, platformChannelID, videoID string) (*db.VideoSyncState, error)
	UpsertVideoSyncState(ctx context.Context, st *db.VideoSyncState) error
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
	// Reaper: sweep stale running jobs older than max job duration.
	reaperInterval = 5 * time.Minute
	staleJobAge    = 15 * time.Minute
	llmStaleJobAge = 15 * time.Minute
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

	// Quota ceilings (optimize-youtube-api-quota). Zero disables each.
	// MaxRequestsPerJob bounds YouTube requests per job via the client's
	// context budget; MaxVideos/MaxThreadPages/MaxReplyFetches bound the
	// worker-side loops that issue them.
	MaxRequestsPerJob     int
	MaxVideosPerJob       int
	MaxThreadPagesPerJob  int
	MaxReplyFetchesPerJob int
	// PollIntervalMinMinutes is the floor of the adaptive per-channel poll
	// interval (configured minimum); inactive channels back off to at most
	// PollIntervalMaxMultiplier x the floor.
	PollIntervalMinMinutes    int
	PollIntervalMaxMultiplier int
}

// NewWorker returns a Worker with sensible defaults.
func NewWorker(db WorkerDB, pool *sql.DB) *Worker {
	client := NewYouTubeClient()
	if daily := config.Default.YouTubeDailyQuotaUnits; daily > 0 {
		client.Quota = NewQuotaAccounting(int64(daily))
	}
	return &Worker{
		DB:                        db,
		Pool:                      pool,
		Client:                    client,
		Alerter:                   LogAlerter{},
		JobDeadline:               defaultJobDeadline,
		MaxConcurrency:            8,
		PollEmptyDelay:            1 * time.Second,
		MaxRequestsPerJob:         config.Default.IngestionMaxRequestsPerJob,
		MaxVideosPerJob:           config.Default.IngestionMaxVideosPerJob,
		MaxThreadPagesPerJob:      config.Default.IngestionMaxThreadPagesPerJob,
		MaxReplyFetchesPerJob:     config.Default.IngestionMaxReplyFetchesPerJob,
		PollIntervalMinMinutes:    config.Default.IngestionPollIntervalMinutes,
		PollIntervalMaxMultiplier: config.Default.IngestionPollIntervalMaxMultiplier,
	}
}

// syncDB returns the optional SyncStateDB implemented by w.DB, if any.
func (w *Worker) syncDB() (SyncStateDB, bool) {
	s, ok := w.DB.(SyncStateDB)
	return s, ok
}

// Run claims jobs in a loop until ctx is cancelled. Concurrent execution
// is bounded by MaxConcurrency.
func (w *Worker) Run(ctx context.Context) {
	if w.MaxConcurrency < 1 {
		w.MaxConcurrency = 1
	}
	// Startup reaper sweep so crash-left running jobs resume immediately.
	w.RecoverStaleJobs(ctx)
	go w.runReaper(ctx)
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
	// Per-job request ceiling is enforced in the client via this context.
	jobCtx = WithJobRequestBudget(jobCtx, w.MaxRequestsPerJob)

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

	accessTok := ""
	if w.Client.APIKey == "" {
		tok, terr := w.obtainAccessToken(jobCtx, job.PlatformChannelID)
		if terr != nil {
			// "no token" → job is a noop; mark completed without raising an error
			// on the channel (the channel-management layer surfaces missing tokens).
			if errors.Is(terr, errNoToken) {
				// Initialize last_synced_at if it was NULL so we don't keep
				// re-enqueuing this channel as a backfill on every tick.
				_ = w.DB.UpdateLastSyncedAtIfNull(parent, job.PlatformChannelID)
				_ = w.DB.MarkJobCompleted(parent, job.ID)
				return
			}
			w.failJob(parent, job, terr)
			outcome = telemetry.OutcomeError
			recordIngestionError(parent, classifyIngestionError(terr))
			return
		}
		accessTok = tok
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
	stats, err := w.runIngestion(jobCtx, parent, pc, accessTok, publishedAfter, job)
	if err != nil {
		// Categorize.
		var rate *RateLimitError
		var tokInv *TokenInvalidError
		var quotaEx *QuotaExhaustedError
		var ceiling *JobBudgetExceededError
		switch {
		case errors.As(err, &quotaEx):
			// Daily project budget spent: defer the channel to the next
			// budget window; the job itself is not a failure and must not
			// burn retry budget.
			until := quotaEx.RetryAt
			if until.IsZero() {
				until = time.Now().Add(time.Hour)
			}
			_ = w.DB.MarkChannelBackoff(parent, job.PlatformChannelID, until)
			_ = w.DB.MarkJobCompleted(parent, job.ID)
			recordIngestionError(parent, telemetry.ReasonRateLimited)
			outcome = telemetry.OutcomeError
			log.Printf("worker: quota exhausted job=%s channel=%s spent=%d retry_at=%s — deferred without retry burn",
				job.ID, job.PlatformChannelID, quotaEx.UnitsSpent, until.UTC().Format(time.RFC3339))
			return
		case errors.As(err, &ceiling):
			// Per-job ceiling: clean stop. Checkpoints are durable; the
			// next scheduled poll resumes where we stopped.
			_ = w.DB.MarkJobCompleted(parent, job.ID)
			log.Printf("worker: job ceiling kind=%s limit=%d job=%s channel=%s — remainder deferred to next tick",
				ceiling.Kind, ceiling.Limit, job.ID, job.PlatformChannelID)
			return
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
	w.adaptPollInterval(parent, pc, stats)
}

// ============================================================================
// Adaptive polling (optimize-youtube-api-quota). Channels with fresh
// content halve their interval toward the configured floor; silent channels
// double it up to PollIntervalMaxMultiplier x the floor. Existing backoff
// still wins whenever it is longer.
// ============================================================================

func (w *Worker) adaptPollInterval(ctx context.Context, pc *db.PlatformChannel, st ingestStats) {
	sdb, ok := w.syncDB()
	if !ok || w.PollIntervalMinMinutes <= 0 {
		return
	}
	floor := w.PollIntervalMinMinutes
	mult := w.PollIntervalMaxMultiplier
	if mult < 2 {
		mult = 8
	}
	cur := floor
	if pc.PollIntervalMinutes != nil && *pc.PollIntervalMinutes > cur {
		cur = *pc.PollIntervalMinutes
	}
	next := cur
	if st.newContent() {
		if next > floor {
			if next /= 2; next < floor {
				next = floor
			}
		}
	} else {
		ceiling := floor * mult
		next = cur * 2
		if next > ceiling {
			next = ceiling
		}
	}
	if next == cur {
		return
	}
	if err := sdb.SetPollInterval(ctx, pc.ID, next); err != nil {
		log.Printf("worker: set poll interval channel=%s: %v", pc.ID, err)
		return
	}
	log.Printf("worker: adaptive poll channel=%s %dmin → %dmin (videos=%d comments=%d)",
		pc.ID, cur, next, st.videos, st.comments)
}

// discoverVideos picks the quota-cheapest video source for a job:
// uploads-playlist playlistItems.list (1 unit/page) when the playlist id is
// known or resolvable, otherwise a bounded search.list fallback (100 units)
// with fallback telemetry.
func (w *Worker) discoverVideos(ctx context.Context, pc *db.PlatformChannel, accessTok string, publishedAfter time.Time, job *db.IngestionJob) (func() (VideoRef, bool, error), error) {
	stopBefore := time.Time{}
	if job.JobType != "backfill" {
		stopBefore = publishedAfter // newest-first walk ends at the watermark
	}
	if pc.UploadsPlaylistID != nil && *pc.UploadsPlaylistID != "" {
		recordYoutubeDiscovery(ctx, "playlist")
		return w.Client.ListUploads(ctx, accessTok, *pc.UploadsPlaylistID, stopBefore), nil
	}
	playlistID, err := w.Client.ResolveUploadsPlaylist(ctx, accessTok, pc.PlatformChannelID)
	if err != nil {
		var rate *RateLimitError
		var tokInv *TokenInvalidError
		var ceiling *JobBudgetExceededError
		switch {
		case errors.As(err, &rate), errors.As(err, &tokInv), errors.As(err, &ceiling):
			return nil, err // auth/limits must not be masked by fallback
		}
		// Bounded fallback: one search.list walk per job, telemetry makes
		// the 100-unit spend visible on noumena_youtube_discovery_total.
		log.Printf("worker: uploads-playlist resolve failed channel=%s err=%v — falling back to search.list", pc.ID, err)
		recordYoutubeDiscovery(ctx, "search_fallback")
		return w.Client.ListVideos(ctx, accessTok, pc.PlatformChannelID, publishedAfter), nil
	}
	pc.UploadsPlaylistID = &playlistID
	if sdb, ok := w.syncDB(); ok {
		if err := sdb.SetUploadsPlaylistID(ctx, pc.ID, playlistID); err != nil {
			log.Printf("worker: cache uploads playlist channel=%s: %v", pc.ID, err)
		}
	} else {
		log.Printf("worker: uploads playlist resolved channel=%s playlist=%s (no sync-state store to cache in)", pc.ID, playlistID)
	}
	recordYoutubeDiscovery(ctx, "playlist")
	return w.Client.ListUploads(ctx, accessTok, playlistID, stopBefore), nil
}

// ingestStats carries what one ingestion run did — feeds adaptive polling
// and the per-job ceilings.
type ingestStats struct {
	videos     int // videos discovered for this run
	comments   int // comments queued for insert (post-watermark)
	pages      int // commentThreads pages fetched
	replyWalks int // gated comments.list walks started
}

func (s ingestStats) newContent() bool { return s.videos > 0 || s.comments > 0 }

// runIngestion paginates videos and comment threads, batches inserts, and
// checkpoints `last_synced_at` after each batch. It additionally persists
// per-video watermarks and page checkpoints (video_sync_state) so an
// interrupted walk resumes instead of restarting, and prunes requests to
// threads that actually have missing replies.
func (w *Worker) runIngestion(jobCtx, persistCtx context.Context, pc *db.PlatformChannel, accessTok string, publishedAfter *time.Time, job *db.IngestionJob) (ingestStats, error) {
	var stats ingestStats
	var pa time.Time
	if publishedAfter != nil {
		pa = *publishedAfter
	}
	nextVideo, err := w.discoverVideos(jobCtx, pc, accessTok, pa, job)
	if err != nil {
		return stats, err
	}

	sdb, hasSDB := w.syncDB()
	batch := make([]*db.Comment, 0, commentBatchSize)
	var maxPostedAt time.Time // channel-wide, feeds last_synced_at

	// walk holds the durable position for the video being ingested now.
	type walkState struct {
		videoID    string
		watermark  time.Time // per-video, from video_sync_state
		videoMax   time.Time // newest comment queued this video
		checkpoint string    // token to re-fetch the page being consumed
	}
	var walk *walkState

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
		if hasSDB && walk != nil {
			cp := walk.checkpoint
			st := &db.VideoSyncState{
				PlatformChannelID: pc.ID,
				VideoID:           walk.videoID,
				PageCheckpoint:    &cp,
			}
			if !walk.videoMax.IsZero() {
				wm := walk.videoMax
				st.CommentWatermarkAt = &wm
			}
			if err := sdb.UpsertVideoSyncState(persistCtx, st); err != nil {
				log.Printf("worker: checkpoint upsert video=%s channel=%s: %v", walk.videoID, pc.ID, err)
			}
		}
		batch = batch[:0]
		return nil
	}
	// finishVideo persists the final watermark and clears the page
	// checkpoint — the walk is complete, nothing to resume.
	finishVideo := func() {
		if !hasSDB || walk == nil {
			return
		}
		st := &db.VideoSyncState{PlatformChannelID: pc.ID, VideoID: walk.videoID}
		if !walk.videoMax.IsZero() {
			wm := walk.videoMax
			st.CommentWatermarkAt = &wm
		}
		if err := sdb.UpsertVideoSyncState(persistCtx, st); err != nil {
			log.Printf("worker: checkpoint finish video=%s channel=%s: %v", walk.videoID, pc.ID, err)
		}
	}

	for {
		// Honor soft deadline: stop cleanly with whatever we have. The
		// last durable checkpoint (flushed above) resumes this video.
		if jobCtx.Err() != nil {
			if err := flush(); err != nil {
				return stats, err
			}
			return stats, jobCtx.Err()
		}

		v, ok, err := nextVideo()
		if err != nil {
			return stats, err
		}
		if !ok {
			break
		}
		if w.MaxVideosPerJob > 0 && stats.videos >= w.MaxVideosPerJob {
			return stats, &JobBudgetExceededError{Kind: "videos", Limit: w.MaxVideosPerJob}
		}
		// Backfill keeps the old server-side window: videos older than the
		// backfill horizon were never fetched via search.list either.
		if job.JobType == "backfill" && !v.PublishedAt.IsZero() && v.PublishedAt.Before(pa) {
			continue
		}
		stats.videos++

		walk = &walkState{videoID: v.ID}
		if hasSDB {
			st, serr := sdb.GetVideoSyncState(jobCtx, pc.ID, v.ID)
			switch {
			case serr == nil:
				if st.Status == "skipped" {
					log.Printf("worker: skip video=%s channel=%s: previously marked comments-unavailable", v.ID, pc.ID)
					continue
				}
				if st.CommentWatermarkAt != nil {
					walk.watermark = *st.CommentWatermarkAt
				}
				if st.PageCheckpoint != nil {
					walk.checkpoint = *st.PageCheckpoint
				}
			case errors.Is(serr, db.ErrNotFound):
				// fresh video — full walk
			default:
				return stats, fmt.Errorf("load video sync state: %w", serr)
			}
			if walk.checkpoint != "" {
				log.Printf("worker: resuming video=%s channel=%s from page checkpoint", v.ID, pc.ID)
			}
		}
		resumeFrom := walk.checkpoint
		pageCeilHit := false
		nextThread := w.Client.ListCommentThreadsPaged(jobCtx, accessTok, v.ID, resumeFrom, func(fetchedWith, nextToken string) {
			walk.checkpoint = fetchedWith
			stats.pages++
			if w.MaxThreadPagesPerJob > 0 && stats.pages >= w.MaxThreadPagesPerJob && nextToken != "" {
				pageCeilHit = true
			}
		})

		for {
			if jobCtx.Err() != nil {
				if err := flush(); err != nil {
					return stats, err
				}
				return stats, jobCtx.Err()
			}
			t, ok, err := nextThread()
			if err != nil {
				var cde *CommentsDisabledError
				if errors.As(err, &cde) {
					// Permanent: video owner disabled comments. Skip the
					// video, keep the rest of the ingestion run intact.
					log.Printf("worker: skip video=%s channel=%s: comments disabled", v.ID, job.PlatformChannelID)
					if hasSDB {
						skipped := &db.VideoSyncState{PlatformChannelID: pc.ID, VideoID: v.ID, Status: "skipped"}
						if err := sdb.UpsertVideoSyncState(persistCtx, skipped); err != nil {
							log.Printf("worker: mark video skipped video=%s: %v", v.ID, err)
						}
					}
					break
				}
				if ferr := flush(); ferr != nil {
					return stats, ferr
				}
				return stats, err
			}
			if !ok {
				break
			}
			// Watermark filter: everything at-or-before the durable
			// per-video watermark is already persisted.
			if !walk.watermark.IsZero() && !t.PostedAt.IsZero() && !t.PostedAt.After(walk.watermark) {
				continue
			}
			// Comments API returns videoId but never videoTitle — carry the
			// title from the discovery ref that found this video.
			if t.VideoID == "" {
				t.VideoID = v.ID
			}
			t.VideoTitle = v.Title
			c := t.ToDBComment(pc.ID)
			batch = append(batch, c)
			stats.comments++
			if t.PostedAt.After(maxPostedAt) {
				maxPostedAt = t.PostedAt
			}
			if t.PostedAt.After(walk.videoMax) {
				walk.videoMax = t.PostedAt
			}
			if len(batch) >= commentBatchSize {
				if err := flush(); err != nil {
					return stats, err
				}
			}

			// Deep replies: gate on MISSING replies only. commentThreads
			// already embeds replies inline; comments.list is issued only
			// when totalReplyCount exceeds the embedded count, so zero- and
			// fully-inline threads cost nothing extra.
			if t.ParentCommentID == nil && t.ReplyDepth == 0 && t.TotalReplyCount > t.EmbeddedReplyCount {
				if w.MaxReplyFetchesPerJob > 0 && stats.replyWalks >= w.MaxReplyFetchesPerJob {
					if err := flush(); err != nil {
						return stats, err
					}
					return stats, &JobBudgetExceededError{Kind: "replies", Limit: w.MaxReplyFetchesPerJob}
				}
				stats.replyWalks++
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
					if !walk.watermark.IsZero() && !r.PostedAt.IsZero() && !r.PostedAt.After(walk.watermark) {
						continue
					}
					// Deep replies: comments.list gives videoId, not title —
					// inherit both from the parent thread.
					r.VideoID = t.VideoID
					r.VideoTitle = t.VideoTitle
					rc := r.ToDBComment(pc.ID)
					batch = append(batch, rc)
					stats.comments++
					if r.PostedAt.After(maxPostedAt) {
						maxPostedAt = r.PostedAt
					}
					if r.PostedAt.After(walk.videoMax) {
						walk.videoMax = r.PostedAt
					}
					if len(batch) >= commentBatchSize {
						if err := flush(); err != nil {
							return stats, err
						}
					}
				}
			}
			if pageCeilHit {
				if err := flush(); err != nil {
					return stats, err
				}
				return stats, &JobBudgetExceededError{Kind: "pages", Limit: w.MaxThreadPagesPerJob}
			}
		}
		if err := flush(); err != nil {
			return stats, err
		}
		finishVideo()
		walk = nil
	}
	return stats, flush()
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
	// Persist refreshed token/expiry so each poll doesn't re-refresh.
	if fresh != nil && (fresh.AccessToken != access || !fresh.Expiry.IsZero()) {
		if row, ok, rerr := w.DB.GetIngestionTokenRow(ctx, platformChannelID); rerr == nil && ok {
			expiry := row.TokenExpiresAt
			if !fresh.Expiry.IsZero() {
				expiry = &fresh.Expiry
			}
			encAccess, eerr := auth.Encrypt(fresh.AccessToken)
			if eerr == nil {
				encRefresh := accessEnc
				// oauth2 reuses refresh token unless rotated.
				if fresh.RefreshToken != "" && fresh.RefreshToken != refresh {
					if enc, e2 := auth.Encrypt(fresh.RefreshToken); e2 == nil {
						encRefresh = enc
					}
				} else {
					encRefresh = refreshEnc
				}
				exp := time.Now().UTC().Add(time.Hour)
				if expiry != nil {
					exp = *expiry
				}
				_ = w.DB.UpdateTokenAndExpiry(ctx, row.ID, encAccess, encRefresh, exp)
			}
		}
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

// RecoverStaleJobs resets running jobs older than staleJobAge to pending.
// Age guard exceeds max job duration so active slow jobs are never swept.
// Every reset is logged for auditability.
func (w *Worker) RecoverStaleJobs(ctx context.Context) {
	n, err := w.DB.RecoverStaleIngestionJobs(ctx, staleJobAge)
	if err != nil {
		log.Printf("worker: reaper ingestion err=%v", err)
	} else if n > 0 {
		log.Printf("worker: reaper reset %d stale ingestion jobs to pending", n)
	}
	if llmReaper, ok := w.DB.(interface {
		RecoverStaleLLMJobs(context.Context, time.Duration) (int64, error)
	}); ok {
		if m, err := llmReaper.RecoverStaleLLMJobs(ctx, llmStaleJobAge); err != nil {
			log.Printf("worker: reaper llm err=%v", err)
		} else if m > 0 {
			log.Printf("worker: reaper reset %d stale llm jobs to pending", m)
		}
	}
}

func (w *Worker) runReaper(ctx context.Context) {
	t := time.NewTicker(reaperInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.RecoverStaleJobs(ctx)
		}
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
