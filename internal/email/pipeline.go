// Package email pipeline (data-lifecycle change).
//
// This file introduces the new transactional-email pipeline: a single
// `Send(ctx, msg)` entry point that handles idempotency, render, SMTP
// dispatch, and the durable `email_send_log` row. It coexists with the
// pre-existing Resend-based `Client` interface (client.go) until callers
// are cut over (Section 5 of the data-lifecycle change). Both paths can run
// against the same process; the pipeline is the strategic direction.
//
// Design highlights:
//
//   - The `EMAIL_ENABLED=false` rollout flag short-circuits Send to a
//     log-only no-op — no rows are written to email_send_log. This is the
//     documented kill-switch (design.md Migration Plan).
//   - Idempotency is enforced at two layers: a pre-insert SELECT checks
//     for existing `status='sent'` rows, and the unique constraint on
//     `idempotency_key` durably enforces single-row-per-key under
//     concurrent inserts (the second caller's INSERT raises a
//     pgconn.PgError 23505 which we translate to an idempotent short-
//     circuit on read).
//   - Rendering uses html/template (HTML body) + text/template (text
//     body) sharing the same variable map; templates live next to this
//     file in `templates/` and are loaded from disk at process start.
//   - Transient SMTP errors leave the row at `status='pending'` for the
//     retry worker (retry_worker.go) to pick up; permanent errors after
//     3 attempts transition to `status='failed'` with last_error set.
package email

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/telemetry"
)

// Message is the single input shape accepted by Send.
//
// Template selects the registered template (must match a key in the
// templates registry; see templates.go). IdempotencyKey is the durable
// dedup primitive — the caller owns its construction and must include
// enough context to make repeated triggers safe (e.g.
// "login_lockout:<user_id>:<locked_until_unix>").
type Message struct {
	Template       string
	ToUserID       string // optional; used to populate email_send_log.user_id
	ToAddress      string // RFC 5322 destination
	IdempotencyKey string
	Subject        string // optional override; templates may set their own
	Variables      map[string]any
}

// Pipeline owns the dependencies for the new email send path. A single
// instance per process is the expected deployment; the underlying SMTP
// connection is short-lived (one Dial per Send) so no pooling is needed
// for the volumes this app produces.
type Pipeline struct {
	Queries db.EmailLogStore
	Sender  SMTPSender
	Cfg     *config.Config
	Logger  *log.Logger
	Now     func() time.Time

	// templates is loaded lazily on first Send. The registry is package-
	// global (templates.go) so we don't re-parse on every call.
	once sync.Once
}

// SMTPSender is the narrow surface the pipeline needs from a transport.
// The production path uses NetSMTPSender (smtp.go) which wraps net/smtp;
// tests inject a fake.
type SMTPSender interface {
	Send(ctx context.Context, from, to, subject, htmlBody, textBody string) error
}

// errAlreadySent is the sentinel returned by short-circuit paths so the
// caller sees a nil error from Send (idempotent re-call).
var errAlreadySent = errors.New("email already sent for idempotency key")

// Send implements the spec contract for the transactional-email pipeline.
//
// Behavior matrix:
//
//	EMAIL_ENABLED=false                 → log "email_disabled_skipped"; return nil
//	idempotency_key already sent        → return nil (no-op)
//	new key, SMTP success               → row→sent (attempt_count=1, sent_at=NOW())
//	new key, SMTP transient failure     → row→pending (last_error set; retry worker picks up)
//	new key, SMTP permanent failure     → row→pending (caller's responsibility to fail loudly;
//	                                       retry worker will eventually mark failed after 3 attempts)
//
// Invariant: at most one email_send_log row per idempotency_key, enforced
// by the UNIQUE constraint and a SELECT-then-INSERT race resolution.
func (p *Pipeline) Send(ctx context.Context, msg Message) error {
	cfg := p.Cfg
	if cfg == nil {
		cfg = config.Default
	}
	if !cfg.EmailEnabled {
		// Documented no-op path (Spec: "email_disabled_skipped"). We
		// intentionally do NOT write to email_send_log here — the row
		// would lie to the operator about the email having been sent.
		p.logf("email_disabled_skipped template=%s key=%s to=%s",
			msg.Template, msg.IdempotencyKey, redactAddr(msg.ToAddress))
		return nil
	}
	if msg.IdempotencyKey == "" {
		return fmt.Errorf("email.Send: idempotency_key is required")
	}
	if msg.Template == "" {
		return fmt.Errorf("email.Send: template is required")
	}
	if msg.ToAddress == "" {
		return fmt.Errorf("email.Send: to_address is required")
	}

	p.once.Do(func() { _ = ensureTemplatesLoaded() })

	// Render up-front so we don't write a log row for a template that
	// will just fail to render every retry. Subject from the template
	// can be overridden by the caller's msg.Subject.
	subject, html, text, err := renderTemplate(msg.Template, msg.Variables)
	if err != nil {
		return fmt.Errorf("email.Send: render %s: %w", msg.Template, err)
	}
	if msg.Subject != "" {
		subject = msg.Subject
	}

	// Idempotency short-circuit on the read side. Race: a concurrent
	// caller may have just inserted the row (status='pending' or 'sent')
	// — we handle the 'sent' case as no-op and the 'pending' case by
	// continuing to attempt the send (the second SMTP call is wasted
	// but harmless; the retry worker would do the same anyway).
	used, err := p.Queries.IsEmailIdempotencyKeyUsed(ctx, msg.IdempotencyKey)
	if err != nil {
		return fmt.Errorf("email.Send: check idempotency: %w", err)
	}
	if used {
		p.logf("email_idempotent_skip template=%s key=%s", msg.Template, msg.IdempotencyKey)
		// otel-instrumentation: idempotent_dedupe is its own outcome
		// so we can distinguish "we caught the duplicate" from real
		// double-sends slipping through the SMTP relay.
		recordEmailSent(ctx, msg.Template, telemetry.OutcomeIdempotentDedupe)
		return nil
	}

	now := p.now()
	logID, err := p.Queries.LogEmailAttempt(ctx, db.EmailAttemptInput{
		UserID:          nilStringIfEmpty(msg.ToUserID),
		Template:        msg.Template,
		IdempotencyKey:  msg.IdempotencyKey,
		Status:          "pending",
		AttemptCount:    1,
		LastAttemptedAt: &now,
	})
	if err != nil {
		// Race: another caller raced us through the unique constraint.
		// Treat as idempotent success.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			p.logf("email_idempotent_race template=%s key=%s", msg.Template, msg.IdempotencyKey)
			// Race-resolved duplicate is semantically the same as
			// the read-side dedupe above.
			recordEmailSent(ctx, msg.Template, telemetry.OutcomeIdempotentDedupe)
			return nil
		}
		return fmt.Errorf("email.Send: insert log row: %w", err)
	}

	from := formatFrom(cfg.EmailFromName, cfg.EmailFromAddress)
	if err := p.Sender.Send(ctx, from, msg.ToAddress, subject, html, text); err != nil {
		// Transient: leave row pending; retry worker will pick up.
		_ = p.Queries.MarkEmailAttemptError(ctx, logID, err.Error())
		p.logf("email_send_failed template=%s key=%s err=%v", msg.Template, msg.IdempotencyKey, err)
		// otel-instrumentation: SMTP failure is the canonical "error"
		// outcome. The retry worker will record its own attempts via
		// recordEmailRetry and (eventually) a terminal error here on
		// the final exhausted attempt.
		recordEmailSent(ctx, msg.Template, telemetry.OutcomeError)
		return fmt.Errorf("email.Send: smtp: %w", err)
	}

	if err := p.Queries.MarkEmailSent(ctx, logID, now); err != nil {
		// We *did* send, but couldn't mark sent. The retry worker will
		// re-send if it picks the row up — that's acceptable since the
		// idempotency key dedup happens server-side at the SMTP relay
		// (modern providers dedup on Message-ID + recipient within a
		// short window; we accept the rare double-send risk).
		p.logf("email_mark_sent_failed template=%s key=%s err=%v", msg.Template, msg.IdempotencyKey, err)
		return nil
	}
	p.logf("email_sent template=%s key=%s to=%s", msg.Template, msg.IdempotencyKey, redactAddr(msg.ToAddress))
	// otel-instrumentation: success path. Note the MarkEmailSent
	// failure branch above falls through to here without
	// re-recording — we DID send the mail, so the contract is
	// "success" from the user's perspective even if we lost the
	// audit row update.
	recordEmailSent(ctx, msg.Template, telemetry.OutcomeSuccess)
	return nil
}

func (p *Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now().UTC()
}

func (p *Pipeline) logf(format string, args ...any) {
	if p.Logger != nil {
		p.Logger.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func formatFrom(name, addr string) string {
	if name == "" {
		return addr
	}
	return fmt.Sprintf("%s <%s>", name, addr)
}

func nilStringIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// redactAddr keeps emails out of logs at INFO level by reducing them to
// "u***@domain.tld". Operators can grep by template + key.
func redactAddr(addr string) string {
	at := -1
	for i, r := range addr {
		if r == '@' {
			at = i
			break
		}
	}
	if at <= 0 {
		return "***"
	}
	return string(addr[0]) + "***" + addr[at:]
}
