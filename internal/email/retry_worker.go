// Package email: retry_worker.go is the goroutine that drains
// email_send_log rows left in status='pending' by failed Send attempts.
//
// The worker re-sends the fully rendered body persisted on the row at
// enqueue time (pipeline.go) — deterministic, and immune to template or
// variable drift across restarts. Rows without a persisted recipient
// (pre-migration legacy rows) are marked failed for operator triage.
package email

import (
	"context"
	"log"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/telemetry"
)

// RetryWorker drains pending email_send_log rows on a 60-second tick.
// One instance per process (gated by LIFECYCLE_WORKERS_ENABLED in main).
type RetryWorker struct {
	Queries  db.EmailLogStore
	Sender   SMTPSender
	Pipeline *Pipeline
	Logger   *log.Logger

	// TickInterval defaults to 60s. Tests shrink it.
	TickInterval time.Duration

	// MaxBatch caps rows claimed per tick. Default 50.
	MaxBatch int

	// Now is the wall clock function. Defaults to time.Now().UTC().
	Now func() time.Time
}

// Start spawns the worker loop. Returns immediately. The loop exits when
// ctx is cancelled. There is no Stop() — the caller owns the context.
func (w *RetryWorker) Start(ctx context.Context) {
	if w.Logger == nil {
		w.Logger = log.Default()
	}
	if w.TickInterval == 0 {
		w.TickInterval = 60 * time.Second
	}
	if w.MaxBatch == 0 {
		w.MaxBatch = 50
	}
	if w.Now == nil {
		w.Now = func() time.Time { return time.Now().UTC() }
	}
	go w.run(ctx)
}

func (w *RetryWorker) run(ctx context.Context) {
	t := time.NewTicker(w.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			w.Logger.Println("email retry worker: stopping")
			return
		case <-t.C:
			w.tickOnce(ctx)
		}
	}
}

func (w *RetryWorker) tickOnce(ctx context.Context) {
	rows, err := w.Queries.ClaimPendingEmails(ctx, w.Now(), w.MaxBatch)
	if err != nil {
		w.Logger.Printf("email retry worker: claim err=%v", err)
		return
	}
	for _, r := range rows {
		if ctx.Err() != nil {
			return
		}
		w.retry(ctx, r)
	}
}

// retry attempts a single row. On success, marks sent. On failure, either
// increments attempt_count (if more attempts remain) or marks failed.
func (w *RetryWorker) retry(ctx context.Context, r *db.EmailLogRow) {
	// otel-instrumentation: every retry attempt counted, regardless of
	// outcome — operators get a flat "how many retries per template" view.
	recordEmailRetry(ctx, r.Template)
	deliverRow(ctx, w.Queries, w.Sender, pipelineCfg(w.Pipeline), w.Logger.Printf, r)
}

// pipelineCfg returns the pipeline's config when present (used to derive
// the From header on retry), or nil so deliverRow falls back to defaults.
func pipelineCfg(p *Pipeline) *config.Config {
	if p != nil {
		return p.Cfg
	}
	return nil
}

// deliverRow re-attempts delivery of a pending email_send_log row using
// the body persisted at enqueue time (fix-email-delivery). This is the
// shared send-on-retry path used by both the retry worker and the
// pipeline's idempotency-race branch, so a transient SMTP failure can
// never silently drop mail: the row is eventually sent or marked failed
// after the attempt cap.
//
// Attempt accounting: the original synchronous Send already counted
// attempt 1 (pipeline inserts with attempt_count=1). Each retry bumps the
// count; a row is marked failed when the next attempt would exceed the
// cap. Rows without a persisted recipient (legacy, pre-migration) are
// marked failed so an operator can intervene rather than looping forever.
func deliverRow(ctx context.Context, queries db.EmailLogStore, sender SMTPSender, cfg *config.Config, logf func(string, ...any), r *db.EmailLogRow) {
	attempt := r.AttemptCount + 1

	if r.ToAddress == "" {
		// Legacy rows have no recipient or body; we cannot re-send.
		// Mark failed so an operator can re-trigger the originating
		// event for a fresh row.
		logf("email retry worker: row %s pending without recipient; marking failed", r.ID)
		_ = queries.MarkEmailFailed(ctx, r.ID, "retry-without-recipient: original Send dropped recipient context")
		recordEmailSent(ctx, r.Template, telemetry.OutcomeError)
		return
	}

	from := formatFrom(emailFromName(cfg), emailFromAddress(cfg))
	if err := sender.Send(ctx, from, r.ToAddress, r.Subject, r.HTMLBody, r.TextBody, nil); err != nil {
		// Failure: bump the attempt. Terminal failure at the cap marks
		// the row failed + emits the terminal metric/alert.
		if attempt >= 3 {
			_ = queries.MarkEmailFailed(ctx, r.ID, "retry budget exhausted")
			logf("email_send_failed row=%s template=%s key=%s attempts=%d err=%v",
				r.ID, r.Template, r.IdempotencyKey, attempt, err)
			recordEmailSent(ctx, r.Template, telemetry.OutcomeError)
			return
		}
		_ = queries.IncrementEmailAttempt(ctx, r.ID, attempt, err.Error())
		logf("email_retry_deferred row=%s template=%s key=%s attempts=%d err=%v",
			r.ID, r.Template, r.IdempotencyKey, attempt, err)
		return
	}

	if err := queries.MarkEmailSent(ctx, r.ID, time.Now().UTC()); err != nil {
		logf("email_retry_mark_sent_failed row=%s template=%s key=%s err=%v",
			r.ID, r.Template, r.IdempotencyKey, err)
		return
	}
	logf("email_retry_sent row=%s template=%s key=%s", r.ID, r.Template, r.IdempotencyKey)
	recordEmailSent(ctx, r.Template, telemetry.OutcomeSuccess)
}

func emailFromName(cfg *config.Config) string {
	if cfg == nil {
		return config.Default.EmailFromName
	}
	return cfg.EmailFromName
}

func emailFromAddress(cfg *config.Config) string {
	if cfg == nil {
		return config.Default.EmailFromAddress
	}
	return cfg.EmailFromAddress
}
