// Package email: pipeline_adapter.go bridges the legacy email.Client
// interface (Send*Email per-feature methods) onto the new Pipeline.Send
// path. Production deployments that have flipped EMAIL_ENABLED=true wire
// up a PipelineAdapter so all transactional mail goes through the
// idempotency log without rewriting every caller.
//
// This is a transitional shim — the long-term direction is for callers
// to invoke email.Send directly with explicit idempotency keys. Until
// every caller is migrated (and the legacy templates are retired in
// favour of the registry copy), the adapter constructs a deterministic
// key from the recipient + payload so concurrent triggers dedupe.
//
// Idempotency keys here favour SAFETY over PRECISION: a hash of
// (template, to_address, primary_payload) means re-calling the same
// trigger within the same minute will dedup, but a re-trigger after
// (say) the recipient acts on the email will produce a fresh key only
// if the underlying state has changed. For verification + reset emails
// this is the desired property; for lockout/digest the caller already
// times its triggers.
package email

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
)

// NewPipelineAdapter returns a Client that routes every Send*Email call
// through the supplied Pipeline. The Pipeline must already be wired with
// Queries + Sender + Cfg.
func NewPipelineAdapter(p *Pipeline) Client {
	return &pipelineAdapter{p: p}
}

type pipelineAdapter struct{ p *Pipeline }

// idempotencyKey constructs a deterministic key from the template id,
// recipient, and a stable per-call payload. SHA-256 keeps the key length
// fixed so it fits cleanly in the email_send_log.idempotency_key TEXT
// column without surprises in indexes.
func idempotencyKey(template, to string, payload ...string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s", template, to)
	for _, p := range payload {
		fmt.Fprintf(h, "|%s", p)
	}
	return template + ":" + hex.EncodeToString(h.Sum(nil))[:32]
}

func (a *pipelineAdapter) ctx() (context.Context, context.CancelFunc) {
	// The legacy interface predates context.Context propagation. We give
	// each adapter call a short-lived background context with a 30s
	// deadline so a stuck SMTP relay can't pin a goroutine indefinitely.
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func (a *pipelineAdapter) lookupUserID(addr string) string {
	if a.p == nil || a.p.Queries == nil {
		return ""
	}
	if dq, ok := a.p.Queries.(db.DBQuery); ok {
		ctx, cancel := a.ctx()
		defer cancel()
		if u, err := dq.GetUserByEmail(ctx, addr); err == nil && u != nil {
			return u.ID
		}
	}
	return ""
}

func (a *pipelineAdapter) SendVerificationEmail(to, link string) error {
	ctx, cancel := a.ctx()
	defer cancel()
	return a.p.Send(ctx, Message{
		Template:       TemplateEmailVerification,
		ToUserID:       a.lookupUserID(to),
		ToAddress:      to,
		IdempotencyKey: idempotencyKey(TemplateEmailVerification, to, link),
		Variables:      map[string]any{"VerifyURL": link},
	})
}

func (a *pipelineAdapter) SendPasswordResetEmail(to, link string) error {
	ctx, cancel := a.ctx()
	defer cancel()
	return a.p.Send(ctx, Message{
		Template:       TemplatePasswordReset,
		ToUserID:       a.lookupUserID(to),
		ToAddress:      to,
		IdempotencyKey: idempotencyKey(TemplatePasswordReset, to, link),
		Variables:      map[string]any{"ResetURL": link},
	})
}

func (a *pipelineAdapter) SendLoginLockoutEmail(to string) error {
	// One key per minute-bucket so a flapping lockout on the same account
	// doesn't double-fire. The bucket key keeps idempotency without
	// blocking a *different* lockout 16 minutes later.
	bucket := time.Now().UTC().Format("2006-01-02T15:04")
	ctx, cancel := a.ctx()
	defer cancel()
	return a.p.Send(ctx, Message{
		Template:       TemplateLoginLockout,
		ToUserID:       a.lookupUserID(to),
		ToAddress:      to,
		IdempotencyKey: idempotencyKey(TemplateLoginLockout, to, bucket),
		Variables:      map[string]any{"LockMinutes": 15},
	})
}

func (a *pipelineAdapter) SendGoogleOnlyAccountResetNotice(to string) error {
	bucket := time.Now().UTC().Format("2006-01-02T15:04")
	// Reuses password_reset template visually; safe because the
	// recipient is asking for a reset and the body explains the Google
	// flow. Keeping a dedicated template would be a copy-only change.
	ctx, cancel := a.ctx()
	defer cancel()
	return a.p.Send(ctx, Message{
		Template:       TemplatePasswordReset,
		ToUserID:       a.lookupUserID(to),
		ToAddress:      to,
		IdempotencyKey: idempotencyKey("google_only_reset", to, bucket),
		Variables: map[string]any{
			"ResetURL": config.Default.AppBaseURL + "/login",
		},
	})
}

func (a *pipelineAdapter) SendWelcomeEmail(to string) error {
	// Welcome email is not in the registry (Spec scope is transactional
	// only). The adapter swallows it as a no-op + log so callers
	// continue to compile; if the welcome email becomes a product
	// requirement, add a registry entry.
	if a.p != nil && a.p.Logger != nil {
		a.p.Logger.Printf("welcome_email_skipped to=%s (not in template registry)", redactAddr(to))
	}
	return nil
}

func (a *pipelineAdapter) SendDigestEmail(to, content string) error {
	// Legacy SendDigestEmail takes a pre-rendered HTML blob; the new
	// pipeline wants structured Variables. The adapter swallows it as
	// a no-op + log; callers SHOULD switch to direct pipeline.Send with
	// TemplateWeeklyDigest. The weekly-digest worker (Section 12.6)
	// already does this.
	if a.p != nil && a.p.Logger != nil {
		a.p.Logger.Printf("legacy_digest_email_skipped to=%s len=%d (use TemplateWeeklyDigest directly)",
			redactAddr(to), len(content))
	}
	return nil
}

func (a *pipelineAdapter) SendLockedAccountEmail(to string) error {
	return a.SendLoginLockoutEmail(to)
}

func (a *pipelineAdapter) SendTokenExpiryEmail(to, channelName string, expiresAt time.Time, settingsURL string) error {
	ctx, cancel := a.ctx()
	defer cancel()
	return a.p.Send(ctx, Message{
		Template:  TemplateTokenExpiry,
		ToUserID:  a.lookupUserID(to),
		ToAddress: to,
		// Idempotency key includes the expiry day so we re-send only
		// when the date itself changes (i.e. token was rotated).
		IdempotencyKey: idempotencyKey(TemplateTokenExpiry, to, channelName, expiresAt.UTC().Format("2006-01-02")),
		Variables: map[string]any{
			"ChannelName": channelName,
			"ExpiresAt":   expiresAt.UTC().Format("January 2, 2006"),
			"SettingsURL": settingsURL,
		},
	})
}
