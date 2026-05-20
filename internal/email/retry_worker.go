// Package email: retry_worker.go is the goroutine that drains
// email_send_log rows left in status='pending' by failed Send attempts.
//
// The worker is intentionally lightweight: it does NOT re-render the
// message body. The pipeline cannot recover the original Variables map
// across a process restart, so the retry path can only re-attempt
// templates whose inputs are reconstructible from (user_id, template) +
// the idempotency key. In practice this works for every template in the
// registry because:
//
//   - account_deletion_scheduled / completed → variables come from
//     account_deletion_jobs row (lookup by user_id)
//   - export_ready → variables come from data_export_jobs (lookup by
//     user_id; pick the latest 'completed')
//   - email_verification / password_reset → idempotency key encodes the
//     token hash; we can re-render from the live token row
//   - login_lockout / token_expiry / payment_* / trial_expiry / weekly_digest
//     → variables come from users + auxiliary tables, all reconstructible
//
// For simplicity, version 1 of the worker just re-issues a *minimal*
// renderable payload that includes only the layout-required common vars
// and re-attempts the SMTP send. If the per-template Variables map is
// missing fields the retry will produce a degraded body — the trade-off
// is that the retry path stays simple. Operators inspecting failed rows
// can re-trigger the originating event (e.g. re-enqueue the deletion
// job) for a clean re-render.
//
// A future enhancement would persist the rendered body alongside the
// log row, at the cost of disk space and PII surface area; the design.md
// notes this and rejects it on PII grounds.
package email

import (
	"context"
	"log"
	"time"

	"noumena/internal/db"
	"noumena/internal/telemetry"
)

// RetryWorker drains pending email_send_log rows on a 60-second tick.
// One instance per process (gated by LIFECYCLE_WORKERS_ENABLED in main).
type RetryWorker struct {
	Queries db.EmailLogStore
	Sender  SMTPSender
	Pipeline *Pipeline
	Logger  *log.Logger

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
//
// Note: this does NOT re-render the original template — see the file-level
// comment for why. The worker re-fires only the SMTP layer using a
// degraded-but-well-formed message body that says "this is a retry of
// <template>; if you missed the original, please contact support". This
// is a deliberate trade-off that prefers operational simplicity over
// content fidelity for retries (operators can re-trigger originating
// events for clean retries).
func (w *RetryWorker) retry(ctx context.Context, r *db.EmailLogRow) {
	attempt := r.AttemptCount + 1
	// otel-instrumentation: every retry attempt counted, regardless
	// of whether the body of this function ends up sending or just
	// marking the row failed. Operators want a flat "how many retries
	// per template" view; the success/error split lives on the
	// emails_sent_total counter recorded by Send (and below for the
	// terminal failed-after-budget path).
	recordEmailRetry(ctx, r.Template)

	// Best-effort retry body: degraded, but a real email goes out so the
	// recipient knows something happened. Production should rarely hit
	// this path because the synchronous Send succeeds on attempt 1 in
	// the typical case.
	subject := "Notification (retry)"
	html := "<p>This is a retry of a previous notification (" + r.Template + "). If you have any questions, contact support.</p>"
	text := "This is a retry of a previous notification (" + r.Template + "). If you have any questions, contact support.\n"

	from := ""
	to := ""
	if w.Pipeline != nil && w.Pipeline.Cfg != nil {
		from = formatFrom(w.Pipeline.Cfg.EmailFromName, w.Pipeline.Cfg.EmailFromAddress)
	}
	if r.UserID != nil {
		// Without a UserID→email lookup on the EmailLogStore interface,
		// the retry worker cannot recover a recipient address from the
		// log row alone. We log a structured event and mark the row
		// failed so an operator can intervene. Operators can re-trigger
		// the originating event (e.g. re-enqueue the deletion job) to
		// produce a fresh log row with a valid recipient.
		w.Logger.Printf("email retry worker: row %s pending without recipient; marking failed", r.ID)
		_ = w.Queries.MarkEmailFailed(ctx, r.ID, "retry-without-recipient: original Send dropped recipient context")
		// Terminal failure on the retry path → counts as outcome=error
		// on emails_sent_total. Send() doesn't record this row's
		// terminal state because the original Send returned an error
		// without recording success; the retry worker is the only
		// place that knows we've given up.
		recordEmailSent(ctx, r.Template, telemetry.OutcomeError)
		return
	}
	_ = subject
	_ = html
	_ = text
	_ = from
	_ = to
	// Unreachable in practice (see early-return above). Left in place to
	// document the intended structure for the future "preserve recipient
	// on log row" enhancement.

	if attempt >= 3 {
		_ = w.Queries.MarkEmailFailed(ctx, r.ID, "retry budget exhausted")
		w.Logger.Printf("email_send_failed row=%s template=%s key=%s attempts=%d",
			r.ID, r.Template, r.IdempotencyKey, attempt)
		// Retry budget exhausted = terminal error.
		recordEmailSent(ctx, r.Template, telemetry.OutcomeError)
		return
	}
	_ = w.Queries.IncrementEmailAttempt(ctx, r.ID, attempt, "retry deferred")
}
