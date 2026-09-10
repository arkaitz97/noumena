// Package lifecycle: deletion_worker.go drains the
// `account_deletion_jobs` queue. It runs as a single goroutine per
// process (idempotent claim via SELECT ... FOR UPDATE SKIP LOCKED, but
// horizontal scale isn't needed at our volumes).
//
// Stage machine — mirrors the spec's status enum:
//
//	pending (grace window, > DELETION_REVERSIBLE_HOURS)
//	   ↓ ClaimNextDeletionJob (atomic UPDATE→exporting)
//	exporting (request GDPR bundle, wait up to EXPORT_PRE_DELETE_TIMEOUT)
//	   ↓ MarkDeletionStage('deleting')
//	deleting (per-channel detach + chunked comment delete + final wipe)
//	   ↓ MarkDeletionCompletedJobOnly
//	completed
//
// Failure paths transition to 'failed' with last_error populated; the
// row stays around for human triage. We deliberately do NOT auto-retry
// failed jobs — the destructive surface is too large to retry blindly.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"noumena/internal/billing"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
	"noumena/internal/telemetry"
)

// DeletionWorker is the long-lived ticker. Construct via NewDeletionWorker;
// call Start(ctx) once. The worker exits when ctx is cancelled.
type DeletionWorker struct {
	Queries  db.DBQuery
	Pool     db.TxBeginner
	Pipeline *email.Pipeline
	Billing  billing.Client
	Cfg      *config.Config
	Logger   *log.Logger

	// Tick controls how often we poll for new jobs. Defaults to 60s;
	// tests shrink it.
	Tick time.Duration

	// Now is the wall clock function. Defaults to time.Now().UTC().
	Now func() time.Time

	stopOnce sync.Once
	done     chan struct{}
}

// Start spawns the worker loop. Returns immediately. The caller owns
// ctx and is responsible for cancelling it on shutdown.
func (w *DeletionWorker) Start(ctx context.Context) {
	if w.Logger == nil {
		w.Logger = log.Default()
	}
	if w.Tick == 0 {
		w.Tick = 60 * time.Second
	}
	if w.Now == nil {
		w.Now = func() time.Time { return time.Now().UTC() }
	}
	if w.Cfg == nil {
		w.Cfg = config.Default
	}
	w.done = make(chan struct{})
	go w.run(ctx)
}

// Stop blocks until the run loop has exited. Safe to call multiple times.
func (w *DeletionWorker) Stop() {
	w.stopOnce.Do(func() {
		if w.done != nil {
			<-w.done
		}
	})
}

func (w *DeletionWorker) run(ctx context.Context) {
	defer close(w.done)
	t := time.NewTicker(w.Tick)
	defer t.Stop()
	w.Logger.Printf("deletion_worker_started tick=%s grace=%s", w.Tick, time.Duration(w.Cfg.DeletionReversibleHours)*time.Hour)
	for {
		select {
		case <-ctx.Done():
			w.Logger.Printf("deletion_worker_stopping err=%v", ctx.Err())
			return
		case <-t.C:
			w.processOne(ctx)
		}
	}
}

// processOne claims one job (if any) and runs it to terminal state.
// Errors are logged and recorded on the job; we never panic out of the
// loop so the worker keeps draining other users' jobs.
func (w *DeletionWorker) processOne(ctx context.Context) {
	grace := time.Duration(w.Cfg.DeletionReversibleHours) * time.Hour
	job, err := w.Queries.ClaimNextDeletionJob(ctx, grace)
	if err != nil {
		w.Logger.Printf("deletion_worker_claim_err err=%v", err)
		return
	}
	if job == nil {
		return // idle
	}
	w.Logger.Printf("deletion_worker_claimed job=%s user=%s", job.ID, job.UserID)

	if err := w.runJob(ctx, job); err != nil {
		w.Logger.Printf("deletion_worker_failed job=%s err=%v", job.ID, err)
		_ = w.Queries.MarkDeletionFailed(ctx, job.ID, err.Error())
		// otel-instrumentation: terminal-state counter, error path.
		recordDeletionCompleted(ctx, telemetry.OutcomeError)
		return
	}
	w.Logger.Printf("deletion_worker_completed job=%s user=%s", job.ID, job.UserID)
	// otel-instrumentation: terminal-state counter, success path.
	recordDeletionCompleted(ctx, telemetry.OutcomeSuccess)
}

// runJob is the per-job state machine. Returns nil on success; any
// returned error gets persisted via MarkDeletionFailed.
func (w *DeletionWorker) runJob(ctx context.Context, job *db.AccountDeletionJob) error {
	user, err := w.Queries.GetUserByID(ctx, job.UserID)
	if err != nil {
		// User row gone — this can happen if a previous run hard-deleted
		// the user but the job-completion update failed. Treat as
		// success and close out the job.
		if errors.Is(err, db.ErrNotFound) {
			w.Logger.Printf("deletion_worker_user_already_gone job=%s user=%s", job.ID, job.UserID)
			return w.Queries.MarkDeletionCompletedJobOnly(ctx, job.ID)
		}
		return fmt.Errorf("load user: %w", err)
	}

	// Stage 1: GDPR pre-export. Best-effort within timeout — if the
	// export worker is slow or down we proceed anyway (the user can
	// still request the export later via the URL we email; but if we
	// have already deleted them they cannot, so we PREFER to wait).
	if err := w.runExportStage(ctx, job, user); err != nil {
		// Non-fatal: log and continue. Spec D8 explicitly allows
		// timeout-then-proceed.
		w.Logger.Printf("deletion_worker_export_warn job=%s err=%v", job.ID, err)
	}

	// Stage 2: per-channel detach + comment cleanup.
	if err := w.Queries.MarkDeletionStage(ctx, job.ID, "deleting"); err != nil {
		return fmt.Errorf("mark stage deleting: %w", err)
	}
	if err := w.runDeletingStage(ctx, job); err != nil {
		return fmt.Errorf("deleting stage: %w", err)
	}

	// Stage 3: final wipe + email + hard delete.
	return w.runFinalStage(ctx, job, user)
}

// runExportStage requests a GDPR export and polls until it completes
// or EXPORT_PRE_DELETE_TIMEOUT elapses. We mark
// `gdpr_export_generated=true` on success; on timeout we log a warning
// and move on.
func (w *DeletionWorker) runExportStage(ctx context.Context, job *db.AccountDeletionJob, _ *db.User) error {
	export, err := w.Queries.EnqueueDataExport(ctx, job.UserID)
	if err != nil {
		return fmt.Errorf("enqueue export: %w", err)
	}
	return w.pollExport(ctx, job, export.ID)
}

// pollExport waits for the GDPR export to finish, ticking every 5s until
// done or EXPORT_PRE_DELETE_TIMEOUT elapses.
func (w *DeletionWorker) pollExport(ctx context.Context, job *db.AccountDeletionJob, exportID string) error {
	deadline := w.Now().Add(w.Cfg.ExportPreDeleteTimeout)
	poll := time.NewTicker(5 * time.Second)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-poll.C:
			done, err := w.exportTick(ctx, job, exportID, deadline)
			if done || err != nil {
				return err
			}
		}
	}
}

// exportTick inspects one poll; done=true ends the wait loop.
func (w *DeletionWorker) exportTick(ctx context.Context, job *db.AccountDeletionJob, exportID string, deadline time.Time) (bool, error) {
	cur, err := w.Queries.GetDataExportJob(ctx, exportID)
	if err != nil {
		return true, fmt.Errorf("poll export: %w", err)
	}
	if cur.Status == "completed" {
		w.markExportGenerated(ctx, job.ID)
		return true, nil
	}
	if cur.Status == "failed" {
		return true, fmt.Errorf("export failed: %s", deref(cur.ErrorMessage))
	}
	if w.Now().After(deadline) {
		return true, fmt.Errorf("export timeout after %s", w.Cfg.ExportPreDeleteTimeout)
	}
	return false, nil
}

// markExportGenerated flips gdpr_export_generated on the deletion job. We
// piggyback on a direct UPDATE to avoid a dedicated method for one boolean.
func (w *DeletionWorker) markExportGenerated(ctx context.Context, jobID string) {
	_, _ = w.Pool.ExecContext(ctx, `UPDATE account_deletion_jobs SET gdpr_export_generated=true WHERE id=?`, jobID)
}

// runDeletingStage iterates the user's tracked channels and removes
// each one. The expensive pieces (chunked comment delete, OAuth
// revocation) are wrapped in their own transactions/retries so a
// partial failure on one channel doesn't poison the rest.
func (w *DeletionWorker) runDeletingStage(ctx context.Context, job *db.AccountDeletionJob) error {
	channels, err := w.Queries.GetUserTrackingChannels(ctx, job.UserID)
	if err != nil {
		return fmt.Errorf("list tracking: %w", err)
	}
	for _, ch := range channels {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.deleteOneChannel(ctx, job, ch); err != nil {
			// Log + continue: a single channel's failure shouldn't
			// orphan the user. The job's last_error captures the most
			// recent.
			w.Logger.Printf("deletion_worker_channel_err job=%s channel=%s err=%v", job.ID, ch.PlatformChannelID, err)
		}
		_ = w.Queries.IncrementChannelsProcessed(ctx, job.ID)
	}
	return nil
}

// revokeAndRecord revokes the channel's OAuth tokens and records the outcome
// on the job; a revoke failure is logged but never blocks the deletion.
func (w *DeletionWorker) revokeAndRecord(ctx context.Context, job *db.AccountDeletionJob, ch *db.UserChannelTrackingRow) {
	rerr := revokeChannelTokens(ctx, ch, w.Logger)
	if rerr != nil {
		w.Logger.Printf("deletion_worker_revoke_failed channel=%s err=%v", ch.PlatformChannelID, rerr)
	}
	_ = w.Queries.MarkDeletionRevokeStatus(ctx, job.ID, true, revokeErrorText(rerr))
}

func revokeErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (w *DeletionWorker) deleteOneChannel(ctx context.Context, job *db.AccountDeletionJob, ch *db.UserChannelTrackingRow) error {
	// 1. OAuth revocation (bounded best-effort — refresh token first,
	//    then access). Failure is recorded on the job for the out-of-band
	//    sweeper (sweeper.go) and does not block the deletion.
	if ch.AccessTokenEnc != "" || ch.RefreshTokenEnc != "" {
		w.revokeAndRecord(ctx, job, ch)
	}

	// 2. Detach the user's tracker row inside a tx that also queries
	//    the remaining tracker count under FOR UPDATE.
	others, err := w.detachTracker(ctx, job, ch)
	if err != nil {
		return err
	}

	// 3. If anyone else is still tracking, promote the next ingestion-
	//    token owner (in case our deleted user was primary) and stop
	//    here — leave their comments alone.
	if others > 0 {
		return w.promoteNextTracker(ctx, ch.PlatformChannelID)
	}

	// 4. We were the sole tracker — chunk-delete comments, then drop
	//    the platform_channels row.
	return w.drainChannelComments(ctx, job, ch)
}

// detachTracker deletes the user's stars + tracking row and reports how many
// trackers remain, all under one FOR UPDATE transaction.
func (w *DeletionWorker) detachTracker(ctx context.Context, job *db.AccountDeletionJob, ch *db.UserChannelTrackingRow) (int, error) {
	tx, err := w.Pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := w.Queries.DeleteUserStarsForChannel(ctx, tx, job.UserID, ch.PlatformChannelID); err != nil {
		return 0, fmt.Errorf("delete stars: %w", err)
	}
	if err := w.Queries.DeleteUserChannelTracking(ctx, tx, job.UserID, ch.PlatformChannelID); err != nil {
		return 0, fmt.Errorf("delete tracking: %w", err)
	}
	others, err := w.Queries.GetActiveTrackerCountForUpdate(ctx, tx, ch.PlatformChannelID, job.UserID)
	if err != nil {
		return 0, fmt.Errorf("count trackers: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return others, nil
}

// promoteNextTracker hands the primary token slot to the next owner.
func (w *DeletionWorker) promoteNextTracker(ctx context.Context, platformChannelID string) error {
	if err := w.Queries.PromoteNextIngestionToken(ctx, platformChannelID); err != nil {
		w.Logger.Printf("deletion_worker_promote_warn channel=%s err=%v", platformChannelID, err)
	}
	return nil
}

// drainChannelComments chunk-deletes the comments we own, then drops the
// shared platform_channels row.
func (w *DeletionWorker) drainChannelComments(ctx context.Context, job *db.AccountDeletionJob, ch *db.UserChannelTrackingRow) error {
	deleted, err := w.Queries.DeleteCommentsByChannelChunked(ctx, ch.PlatformChannelID, w.Cfg.DeletionChunkSize, job.UserID)
	// otel-instrumentation: record the chunk size on every call,
	// even on the new-tracker race below — partial deletes still
	// represent disk-bound work the operators want to see.
	recordDeletionChunkRows(ctx, int64(deleted))
	if errors.Is(err, db.ErrNewTrackerAppeared) {
		w.logNewTrackerRace(ctx, ch.PlatformChannelID, deleted)
		return nil
	}
	if err != nil {
		return fmt.Errorf("chunked delete: %w", err)
	}
	w.Logger.Printf("deletion_worker_channel_drained channel=%s comments_deleted=%d", ch.PlatformChannelID, deleted)
	return w.dropPlatformChannel(ctx, ch.PlatformChannelID)
}

// logNewTrackerRace: another user started tracking mid-delete. Bail out of
// channel-level cleanup; the new tracker keeps the remaining comments.
func (w *DeletionWorker) logNewTrackerRace(ctx context.Context, platformChannelID string, deleted int) {
	w.Logger.Printf("deletion_worker_channel_aborted_new_tracker channel=%s deleted=%d", platformChannelID, deleted)
	// Per-channel abort surfaces as a discrete outcome on the deletions
	// counter — the per-job outcome may still resolve to "success" if other
	// channels complete cleanly.
	recordDeletionCompleted(ctx, telemetry.OutcomeAbortedNewTracker)
}

// dropPlatformChannel removes the shared channel row (defensive count check
// inside).
func (w *DeletionWorker) dropPlatformChannel(ctx context.Context, platformChannelID string) error {
	tx2, err := w.Pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := w.Queries.DeletePlatformChannel(ctx, tx2, platformChannelID); err != nil {
		_ = tx2.Rollback()
		return fmt.Errorf("delete platform_channels: %w", err)
	}
	return tx2.Commit()
}

// runFinalStage: drop digests, remaining stars, auth tokens; emit the
// deletion_completed email; hard-delete the user; close the job.
func (w *DeletionWorker) runFinalStage(ctx context.Context, job *db.AccountDeletionJob, user *db.User) error {
	tx, err := w.Pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := w.Queries.DeleteUserGDPRData(ctx, tx, job.UserID); err != nil {
		return fmt.Errorf("GDPR wipe: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	// Send the deletion_completed email BEFORE hard-deleting the user
	// row so we still have a recipient address to log against and the
	// email_send_log row's user_id FK points somewhere real (it is
	// ON DELETE SET NULL but having the link is preferable for ops).
	if w.Pipeline != nil {
		_ = w.Pipeline.Send(ctx, email.Message{
			Template:       email.TemplateDeletionCompleted,
			ToUserID:       user.ID,
			ToAddress:      user.Email,
			IdempotencyKey: "deletion_completed:" + user.ID,
			Variables:      map[string]any{},
		})
	}

	// Hard delete inside its own transaction. If this fails the user
	// row remains; the next worker tick will see the still-active
	// account_deletion_jobs row, attempt to re-load the user, find it
	// present, and re-run the final stage. The deletion_completed email
	// is idempotency-keyed by user-id so it won't re-send.

	// Audit log before hard delete (user_id FK will be SET NULL after delete).
	_ = w.Queries.InsertAuditLog(ctx, user.ID, "deletion_completed", `{"user_id":"`+user.ID+`"}`, "")

	tx2, err := w.Pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := w.Queries.HardDeleteUser(ctx, tx2, user.ID); err != nil {
		_ = tx2.Rollback()
		return fmt.Errorf("hard delete user: %w", err)
	}
	if err := tx2.Commit(); err != nil {
		return err
	}

	return w.Queries.MarkDeletionCompletedJobOnly(ctx, job.ID)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
