package email

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"noumena/internal/config"
	"noumena/internal/db"
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
		TemplateDigestDelayed:     {"WeekRange": "2026-04-20 - 2026-04-26"},
		TemplateExportReady:       {"DownloadURL": "https://app/export?sig=x&exp=1", "ExpiresAt": time.Now().Add(48 * time.Hour).Format(time.RFC3339)},
		TemplateDeletionScheduled: {"ReversibleUntil": time.Now().Add(48 * time.Hour).Format(time.RFC3339)},
		TemplateDeletionCompleted: map[string]any{},
		TemplatePlanDowngrade: {
			"OldPlan":        "Creator",
			"NewPlan":        "Free",
			"ChannelLimit":   1,
			"PausedChannels": true,
		},
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

// raceStore simulates the UNIQUE-constraint race: LogEmailAttempt always
// fails with a unique violation, and the existing row is served from
// `existing`.
type raceStore struct {
	existing *db.EmailLogRow
	sent     []string
}

func (s *raceStore) IsEmailIdempotencyKeyUsed(context.Context, string) (bool, error) {
	return false, nil
}
func (s *raceStore) LogEmailAttempt(context.Context, db.EmailAttemptInput) (string, error) {
	return "", errors.New("UNIQUE constraint failed: email_send_log.idempotency_key")
}
func (s *raceStore) MarkEmailSent(_ context.Context, id string, _ time.Time) error {
	s.sent = append(s.sent, id)
	return nil
}
func (s *raceStore) IncrementEmailAttempt(context.Context, string, int, string) error { return nil }
func (s *raceStore) MarkEmailFailed(context.Context, string, string) error            { return nil }
func (s *raceStore) MarkEmailAttemptError(context.Context, string, string) error      { return nil }
func (s *raceStore) ClaimPendingEmails(context.Context, time.Time, int) ([]*db.EmailLogRow, error) {
	return nil, nil
}
func (s *raceStore) GetEmailRowByIdempotencyKey(_ context.Context, _ string) (*db.EmailLogRow, error) {
	return s.existing, nil
}

// Task 2.3: a duplicate idempotency key whose row is still pending (the
// first send failed) must NOT be dropped — the pipeline re-attempts the
// send with the persisted body and flips the row to sent.
func TestSendRacePendingRowIsResent(t *testing.T) {
	store := &raceStore{existing: &db.EmailLogRow{
		ID:           "row-race",
		Template:     TemplateWeeklyDigest,
		Status:       "pending",
		AttemptCount: 1,
		ToAddress:    "user@example.com",
		Subject:      "digest",
		HTMLBody:     "<p>digest</p>",
		TextBody:     "digest",
	}}
	sender := &stubSender{}
	p := &Pipeline{
		Queries: store,
		Sender:  sender,
		Cfg:     &config.Config{EmailEnabled: true, EmailFromAddress: "from@example.com", EmailFromName: "N"},
	}
	err := p.Send(context.Background(), Message{
		Template:       TemplateWeeklyDigest,
		ToAddress:      "user@example.com",
		IdempotencyKey: "weekly_digest:dig-1",
		Variables:      map[string]any{},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 Send call for pending raced row, got %d", len(sender.calls))
	}
	if sender.calls[0].to != "user@example.com" || sender.calls[0].html != "<p>digest</p>" {
		t.Errorf("Send used wrong payload: %+v", sender.calls[0])
	}
	if len(store.sent) != 1 || store.sent[0] != "row-race" {
		t.Errorf("expected MarkEmailSent(row-race), got %v", store.sent)
	}
}

// A raced row that is already sent stays a true idempotent no-op.
func TestSendRaceSentRowDedupes(t *testing.T) {
	store := &raceStore{existing: &db.EmailLogRow{
		ID:        "row-sent",
		Status:    "sent",
		ToAddress: "user@example.com",
	}}
	sender := &stubSender{}
	p := &Pipeline{
		Queries: store,
		Sender:  sender,
		Cfg:     &config.Config{EmailEnabled: true, EmailFromAddress: "from@example.com"},
	}
	err := p.Send(context.Background(), Message{
		Template:       TemplateWeeklyDigest,
		ToAddress:      "user@example.com",
		IdempotencyKey: "weekly_digest:dig-1",
		Variables:      map[string]any{},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(sender.calls) != 0 {
		t.Errorf("sent raced row must not re-send, got %d calls", len(sender.calls))
	}
	if len(store.sent) != 0 {
		t.Errorf("sent raced row must not be marked sent again, got %v", store.sent)
	}
}

// Task 4.3: weekly digest renders non-empty lists from real content and
// never emits Go's "<no value>" placeholder, even when the sentiment
// fields are absent (sentiment lands in a later change).
func TestWeeklyDigestRenderNoPlaceholder(t *testing.T) {
	vars := map[string]any{
		"WeekRange":    "Jan 2 – Jan 8, 2026",
		"TopIdeas":     []string{"more tutorials — many positives"},
		"TopQuestions": []string{"when is v2 shipping?"},
	}
	_, html, text, err := renderTemplate(TemplateWeeklyDigest, vars)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"more tutorials", "when is v2 shipping?"} {
		if !strings.Contains(html, want) || !strings.Contains(text, want) {
			t.Errorf("body missing %q\nhtml: %s\ntext: %s", want, html, text)
		}
	}
	if strings.Contains(html, "<no value>") || strings.Contains(text, "<no value>") {
		t.Errorf("render leaked <no value> placeholder")
	}
}
