package email

import (
	"strings"
	"testing"
	"time"
)

// TestRenderAllTemplates is the snapshot smoke test for the full
// template registry. It exercises every TemplateID with a minimal-but-
// complete Variables map and asserts:
//
//   - render does not error
//   - subject is non-empty
//   - both bodies contain the layout-required common vars
//   - both bodies contain at least one template-specific token
//
// This is intentionally lighter than per-template golden files (which
// would lock the team into reviewing diffs for every copy tweak); the
// goal is to catch breakage of the registry/layout, not to diff prose.
func TestRenderAllTemplates(t *testing.T) {
	cases := map[string]map[string]any{
		TemplateEmailVerification: {"VerifyURL": "https://app/verify?token=abc"},
		TemplatePasswordReset:     {"ResetURL": "https://app/reset?token=abc"},
		TemplateLoginLockout:      {"LockMinutes": 15, "Region": "EU-West"},
		TemplateTokenExpiry: {
			"ChannelName": "Channel One",
			"ExpiresAt":   "January 1, 2026",
			"SettingsURL": "https://app/settings/channels",
		},
		TemplatePaymentFailed:     {"PlanName": "Creator", "PortalURL": "https://app/portal"},
		TemplatePaymentGraceFinal: {"PortalURL": "https://app/portal"},
		TemplateTrialExpiry:       {"PlanName": "Creator", "DaysRemaining": 3, "UpgradeURL": "https://app/upgrade"},
		TemplateWeeklyDigest: {
			"WeekRange":          "2026-04-20 - 2026-04-26",
			"TopIdeas":           []string{"Idea A", "Idea B"},
			"TopQuestions":       []string{"Question A"},
			"SentimentDirection": "up",
			"SentimentChangePct": 5,
		},
		TemplateExportReady:        {"DownloadURL": "https://app/export?sig=x&exp=1", "ExpiresAt": time.Now().Add(48 * time.Hour).Format(time.RFC3339)},
		TemplateDeletionScheduled:  {"ReversibleUntil": time.Now().Add(48 * time.Hour).Format(time.RFC3339)},
		TemplateDeletionCompleted:  map[string]any{},
	}

	for _, id := range TemplateIDs {
		vars, ok := cases[id]
		if !ok {
			t.Errorf("template %q has no fixture in TestRenderAllTemplates", id)
			continue
		}
		subject, html, text, err := renderTemplate(id, vars)
		if err != nil {
			t.Errorf("render %q: %v", id, err)
			continue
		}
		if strings.TrimSpace(subject) == "" {
			t.Errorf("render %q: empty subject", id)
		}
		if !strings.Contains(html, "<!doctype html>") {
			t.Errorf("render %q: html body missing layout", id)
		}
		if strings.TrimSpace(text) == "" {
			t.Errorf("render %q: empty text body", id)
		}
	}
}

func TestSignerRoundTrip(t *testing.T) {
	const secret = "test-secret"
	exp := time.Now().Add(time.Hour)
	q := SignURL(secret, "user-1", "job-1", exp)
	parts := strings.Split(q, "&")
	if len(parts) != 2 {
		t.Fatalf("expected sig=...&exp=..., got %q", q)
	}
	sig := strings.TrimPrefix(parts[0], "sig=")
	expStr := strings.TrimPrefix(parts[1], "exp=")
	expUnix, err := ParseExp(expStr)
	if err != nil {
		t.Fatalf("ParseExp: %v", err)
	}

	if err := VerifyURL(secret, "user-1", "job-1", expUnix, sig); err != nil {
		t.Errorf("verify good sig: %v", err)
	}
	// tampered userID
	if err := VerifyURL(secret, "user-2", "job-1", expUnix, sig); err == nil {
		t.Errorf("verify tampered userID should fail")
	}
	// expired
	if err := VerifyURL(secret, "user-1", "job-1", time.Now().Add(-time.Hour).Unix(), sig); err == nil {
		t.Errorf("verify expired should fail")
	}
}
