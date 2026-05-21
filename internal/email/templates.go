// Package email: templates.go owns the registry of all 11 transactional
// templates, the shared layout, and rendering. Bodies are kept inline as
// constants so the binary is self-contained (no filesystem lookup at
// runtime). This trades a small loss of editor-friendliness for a much
// simpler deployment story — no template path env var, no missing-file
// failure mode, easy snapshot tests against the registry.
package email

import (
	"bytes"
	"fmt"
	html "html/template"
	"strings"
	"sync"
	text "text/template"

	"noumena/internal/config"
)

// All template ids known to the pipeline. Adding a new transactional
// email is a three-line change: add to TemplateIDs, register in init,
// author the body. The pipeline's render path validates the id is in
// the registry before any DB write.
const (
	TemplateEmailVerification  = "email_verification"
	TemplatePasswordReset      = "password_reset"
	TemplateLoginLockout       = "login_lockout"
	TemplateTokenExpiry        = "token_expiry"
	TemplatePaymentFailed      = "payment_failed"
	TemplatePaymentGraceFinal  = "payment_grace_final"
	TemplateTrialExpiry        = "trial_expiry"
	TemplateWeeklyDigest       = "weekly_digest"
	TemplateDigestDelayed      = "digest_delayed"
	TemplateExportReady        = "export_ready"
	TemplateDeletionScheduled  = "deletion_scheduled"
	TemplateDeletionCompleted  = "deletion_completed"
	TemplatePlanDowngrade      = "plan_downgrade"
)

var TemplateIDs = []string{
	TemplateEmailVerification,
	TemplatePasswordReset,
	TemplateLoginLockout,
	TemplateTokenExpiry,
	TemplatePaymentFailed,
	TemplatePaymentGraceFinal,
	TemplateTrialExpiry,
	TemplateWeeklyDigest,
	TemplateDigestDelayed,
	TemplateExportReady,
	TemplateDeletionScheduled,
	TemplateDeletionCompleted,
	TemplatePlanDowngrade,
}

// templateDef is one row in the registry. Subject is also a Go template
// (so e.g. weekly_digest can interpolate the week range into the subject
// line). The pipeline applies the same Variables map to all three.
type templateDef struct {
	subject string
	html    string
	textRaw string
}

var (
	templateMu      sync.RWMutex
	templates       map[string]templateDef
	parsedHTML      map[string]*html.Template
	parsedSubject   map[string]*text.Template
	parsedText      map[string]*text.Template
)

func ensureTemplatesLoaded() error {
	templateMu.Lock()
	defer templateMu.Unlock()
	if templates != nil {
		return nil
	}
	templates = builtinTemplates()
	parsedHTML = make(map[string]*html.Template, len(templates))
	parsedSubject = make(map[string]*text.Template, len(templates))
	parsedText = make(map[string]*text.Template, len(templates))
	for id, def := range templates {
		// Wrap every HTML body in the shared layout so callers don't
		// duplicate the header/footer markup. Templates use the {{ . }}
		// pipeline (the Variables map plus injected CommonVars).
		ht, err := html.New(id).Parse(htmlLayout(def.html))
		if err != nil {
			return fmt.Errorf("parse html %q: %w", id, err)
		}
		parsedHTML[id] = ht

		tx, err := text.New(id + ".txt").Parse(textLayout(def.textRaw))
		if err != nil {
			return fmt.Errorf("parse text %q: %w", id, err)
		}
		parsedText[id] = tx

		st, err := text.New(id + ".subj").Parse(def.subject)
		if err != nil {
			return fmt.Errorf("parse subject %q: %w", id, err)
		}
		parsedSubject[id] = st
	}
	return nil
}

// renderTemplate produces (subject, html, text) for a registered template.
// The variables map is augmented with CommonVars (APP_BASE_URL, support
// email, etc.) so every template can reference them without ceremony.
func renderTemplate(id string, vars map[string]any) (string, string, string, error) {
	if err := ensureTemplatesLoaded(); err != nil {
		return "", "", "", err
	}
	templateMu.RLock()
	defer templateMu.RUnlock()

	st, ok := parsedSubject[id]
	if !ok {
		return "", "", "", fmt.Errorf("unknown template %q", id)
	}
	merged := mergeCommonVars(vars)

	var sb strings.Builder
	if err := st.Execute(&sb, merged); err != nil {
		return "", "", "", fmt.Errorf("execute subject %q: %w", id, err)
	}
	subject := sb.String()

	var hb bytes.Buffer
	if err := parsedHTML[id].Execute(&hb, merged); err != nil {
		return "", "", "", fmt.Errorf("execute html %q: %w", id, err)
	}

	var tb bytes.Buffer
	if err := parsedText[id].Execute(&tb, merged); err != nil {
		return "", "", "", fmt.Errorf("execute text %q: %w", id, err)
	}

	return subject, hb.String(), tb.String(), nil
}

// mergeCommonVars injects layout-required variables into the user-supplied
// map. It returns a new map; the original is left untouched (some callers
// reuse Variables across multiple Send invocations).
func mergeCommonVars(in map[string]any) map[string]any {
	cfg := config.Default
	out := map[string]any{
		"AppBaseURL":      cfg.AppBaseURL,
		"AppSupportEmail": cfg.AppSupportEmail,
		"FromName":        cfg.EmailFromName,
	}
	for k, v := range in {
		out[k] = v
	}
	return out
}

// htmlLayout wraps a template body in a minimal but well-formed HTML
// document. Inline CSS only — no external resources, no images — to keep
// rendering predictable across mail clients.
func htmlLayout(body string) string {
	return `<!doctype html>
<html><head><meta charset="utf-8"><title>{{.Subject}}</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 24px; color: #1a1a1a; line-height: 1.5;">
<div style="border-bottom: 1px solid #e5e5e5; padding-bottom: 16px; margin-bottom: 24px;">
<strong style="font-size: 18px;">{{.FromName}}</strong>
</div>
` + body + `
<div style="margin-top: 32px; padding-top: 16px; border-top: 1px solid #e5e5e5; font-size: 12px; color: #888;">
<p>You received this email because you have an account at <a href="{{.AppBaseURL}}">{{.AppBaseURL}}</a>.</p>
<p>Questions? Reply to this email or contact <a href="mailto:{{.AppSupportEmail}}">{{.AppSupportEmail}}</a>.</p>
</div>
</body></html>`
}

// textLayout is the plain-text counterpart. Mail clients fall back to
// this when HTML is suppressed (corporate filters, accessibility readers).
func textLayout(body string) string {
	return body + `

—
You received this email because you have an account at {{.AppBaseURL}}.
Questions? Reply to this email or contact {{.AppSupportEmail}}.
`
}

// builtinTemplates is the in-binary registry of all 11 templates. Bodies
// are intentionally short — these are transactional emails, not marketing
// — and use only the variables documented in the spec.
//
// Naming convention for variables: PascalCase (Go template convention).
// Required variables per template are documented in the spec and enforced
// only by render-time errors when keys are missing (Go templates default
// to <no value> for absent map keys, so we use {{if}} guards).
func builtinTemplates() map[string]templateDef {
	return map[string]templateDef{
		TemplateEmailVerification: {
			subject: "Verify your account",
			html: `<p>Welcome!</p>
<p>Please verify your email by clicking the link below. The link is valid for 24 hours.</p>
<p><a href="{{.VerifyURL}}" style="display:inline-block;padding:10px 20px;background:#1a73e8;color:white;text-decoration:none;border-radius:4px;">Verify my email</a></p>
<p>Or copy this URL: {{.VerifyURL}}</p>
<p>If you did not create this account, you can ignore this email.</p>`,
			textRaw: `Welcome!

Please verify your email by clicking the link below. The link is valid for 24 hours.

{{.VerifyURL}}

If you did not create this account, you can ignore this email.`,
		},

		TemplatePasswordReset: {
			subject: "Reset your password",
			html: `<p>We received a request to reset your password.</p>
<p>Click the link below to set a new password. The link is valid for 1 hour.</p>
<p><a href="{{.ResetURL}}" style="display:inline-block;padding:10px 20px;background:#1a73e8;color:white;text-decoration:none;border-radius:4px;">Reset my password</a></p>
<p>If you did not request a reset, you can safely ignore this email.</p>`,
			textRaw: `We received a request to reset your password.

Click the link below to set a new password. The link is valid for 1 hour.

{{.ResetURL}}

If you did not request a reset, you can safely ignore this email.`,
		},

		TemplateLoginLockout: {
			subject: "Your account has been temporarily locked",
			html: `<p>We detected several failed login attempts on your account{{if .Region}} from <strong>{{.Region}}</strong>{{end}}.</p>
<p>For your protection, the account has been locked for <strong>{{.LockMinutes}} minutes</strong>.</p>
<p>If this was not you, please reset your password from the <a href="{{.AppBaseURL}}/forgot-password">password reset page</a> as soon as possible.</p>`,
			textRaw: `We detected several failed login attempts on your account{{if .Region}} from {{.Region}}{{end}}.

For your protection, the account has been locked for {{.LockMinutes}} minutes.

If this was not you, please reset your password: {{.AppBaseURL}}/forgot-password`,
		},

		TemplateTokenExpiry: {
			subject: "Reconnect {{.ChannelName}} before its access expires",
			html: `<p>The OAuth access for <strong>{{.ChannelName}}</strong> will expire on <strong>{{.ExpiresAt}}</strong>.</p>
<p>To keep ingestion running, please reconnect the channel from your <a href="{{.SettingsURL}}">channel settings</a>.</p>
<p>If you no longer want to track this channel, you can ignore this email and the channel will simply stop syncing.</p>`,
			textRaw: `The OAuth access for {{.ChannelName}} will expire on {{.ExpiresAt}}.

To keep ingestion running, please reconnect the channel: {{.SettingsURL}}

If you no longer want to track this channel, you can ignore this email.`,
		},

		TemplatePaymentFailed: {
			subject: "We couldn't process your payment",
			html: `<p>Your most recent payment for {{.PlanName}} could not be processed.</p>
<p>You have <strong>3 days</strong> to update your payment method before your account is paused. Your data and channels are preserved during the grace period.</p>
<p><a href="{{.PortalURL}}" style="display:inline-block;padding:10px 20px;background:#1a73e8;color:white;text-decoration:none;border-radius:4px;">Update payment method</a></p>`,
			textRaw: `Your most recent payment for {{.PlanName}} could not be processed.

You have 3 days to update your payment method before your account is paused. Your data and channels are preserved during the grace period.

Update payment method: {{.PortalURL}}`,
		},

		TemplatePaymentGraceFinal: {
			subject: "Your account is paused — update payment to restore access",
			html: `<p>Your account has been <strong>paused</strong> because we were unable to collect payment within the 3-day grace period.</p>
<p>Your data is preserved. To restore service, update your payment method:</p>
<p><a href="{{.PortalURL}}" style="display:inline-block;padding:10px 20px;background:#1a73e8;color:white;text-decoration:none;border-radius:4px;">Update payment method</a></p>`,
			textRaw: `Your account has been paused because we were unable to collect payment within the 3-day grace period.

Your data is preserved. To restore service, update your payment method: {{.PortalURL}}`,
		},

		TemplateTrialExpiry: {
			subject: "Your trial ends in {{.DaysRemaining}} days",
			html: `<p>Your trial of {{.PlanName}} ends in <strong>{{.DaysRemaining}} days</strong>.</p>
<p>To keep your channels syncing without interruption, choose a plan now:</p>
<p><a href="{{.UpgradeURL}}" style="display:inline-block;padding:10px 20px;background:#1a73e8;color:white;text-decoration:none;border-radius:4px;">Choose a plan</a></p>`,
			textRaw: `Your trial of {{.PlanName}} ends in {{.DaysRemaining}} days.

To keep your channels syncing without interruption, choose a plan now: {{.UpgradeURL}}`,
		},

		TemplateWeeklyDigest: {
			subject: "Your weekly digest ({{.WeekRange}})",
			html: `<p>Here's what happened across your tracked channels for the week of <strong>{{.WeekRange}}</strong>:</p>
{{if .TopIdeas}}<h3>Top ideas</h3><ul>{{range .TopIdeas}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .TopQuestions}}<h3>Top questions</h3><ul>{{range .TopQuestions}}<li>{{.}}</li>{{end}}</ul>{{end}}
<p><strong>Sentiment trend:</strong> {{.SentimentDirection}} ({{.SentimentChangePct}}% week-over-week)</p>
<p><a href="{{.AppBaseURL}}/dashboard">View full digest →</a></p>`,
			textRaw: `Here's what happened across your tracked channels for the week of {{.WeekRange}}:

{{if .TopIdeas}}TOP IDEAS:
{{range .TopIdeas}}- {{.}}
{{end}}{{end}}
{{if .TopQuestions}}TOP QUESTIONS:
{{range .TopQuestions}}- {{.}}
{{end}}{{end}}
Sentiment trend: {{.SentimentDirection}} ({{.SentimentChangePct}}% week-over-week)

View full digest: {{.AppBaseURL}}/dashboard`,
		},

		TemplateDigestDelayed: {
			subject: "Your weekly digest is delayed",
			html: `<p>We're still preparing your weekly digest for <strong>{{.WeekRange}}</strong>.</p>
<p>Due to high processing volume, your digest will be available shortly. You'll receive another email when it's ready.</p>
<p><a href="{{.AppBaseURL}}/dashboard">View dashboard →</a></p>`,
			textRaw: `We're still preparing your weekly digest for {{.WeekRange}}.

Due to high processing volume, your digest will be available shortly. You'll receive another email when it's ready.

View dashboard: {{.AppBaseURL}}/dashboard`,
		},

		TemplateExportReady: {
			subject: "Your data export is ready",
			html: `<p>Your data export is ready to download.</p>
<p>The link below is valid until <strong>{{.ExpiresAt}}</strong>:</p>
<p><a href="{{.DownloadURL}}" style="display:inline-block;padding:10px 20px;background:#1a73e8;color:white;text-decoration:none;border-radius:4px;">Download export</a></p>
<p>If the button doesn't work, copy this URL into your browser:<br><span style="word-break:break-all;color:#555">{{.DownloadURL}}</span></p>`,
			textRaw: `Your data export is ready to download.

The link is valid until {{.ExpiresAt}}:

{{.DownloadURL}}`,
		},

		TemplateDeletionScheduled: {
			subject: "Your account is scheduled for deletion",
			html: `<p>We received a request to delete your account.</p>
<p>Your account deletion is reversible for <strong>30 days</strong> until <strong>{{.ReversibleUntil}}</strong>. After that time we will begin permanently deleting your data.</p>
<p>If this was a mistake, you can cancel the deletion from your account settings or contact <a href="mailto:{{.AppSupportEmail}}">{{.AppSupportEmail}}</a> immediately and reference your account email.</p>`,
			textRaw: `We received a request to delete your account.

Your account deletion is reversible for 30 days until {{.ReversibleUntil}}. After that time we will begin permanently deleting your data.

If this was a mistake, you can cancel the deletion from your account settings or contact {{.AppSupportEmail}} immediately and reference your account email.`,
		},

		TemplateDeletionCompleted: {
			subject: "Your account has been deleted",
			html: `<p>Your account has been permanently deleted along with all associated data.</p>
<p>If you previously requested a GDPR data export, that download link remains valid until its original expiry; no further action is required.</p>
<p>We're sorry to see you go.</p>`,
			textRaw: `Your account has been permanently deleted along with all associated data.

If you previously requested a GDPR data export, that download link remains valid until its original expiry; no further action is required.

We're sorry to see you go.`,
		},

		TemplatePlanDowngrade: {
			subject: "Your plan has been downgraded to {{.NewPlan}}",
			html: `<p>Your subscription has been downgraded from <strong>{{.OldPlan}}</strong> to <strong>{{.NewPlan}}</strong>.</p>
{{if .PausedChannels}}<p>Some of your channels have been paused to comply with the {{.NewPlan}} plan limit of <strong>{{.ChannelLimit}}</strong> channels. You can reconnect them at any time by upgrading your plan.</p>{{end}}
<p><a href="{{.AppBaseURL}}/settings/billing">Manage your subscription</a></p>`,
			textRaw: `Your subscription has been downgraded from {{.OldPlan}} to {{.NewPlan}}.
{{if .PausedChannels}}Some of your channels have been paused to comply with the {{.NewPlan}} plan limit of {{.ChannelLimit}} channels. You can reconnect them at any time by upgrading your plan.{{end}}

Manage your subscription: {{.AppBaseURL}}/settings/billing`,
		},
	}
}
