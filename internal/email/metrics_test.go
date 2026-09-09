package email

// otel-instrumentation: §9.7 verifies that an idempotent duplicate
// Send increments emails_sent_total{outcome=idempotent_dedupe} and
// does NOT call SMTP.

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/telemetry"
)

func installEmailManualReader(t *testing.T) (*sdkmetric.ManualReader, func()) {
	t.Helper()
	prev := otel.GetMeterProvider()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	rebindEmailMeters()
	return reader, func() {
		_ = mp.Shutdown(context.Background())
		otel.SetMeterProvider(prev)
		rebindEmailMeters()
	}
}

func rebindEmailMeters() {
	emailMeter = otel.Meter("noumena/email")
	emailsSentTotal, _ = emailMeter.Int64Counter("noumena_emails_sent_total")
	emailRetryTotal, _ = emailMeter.Int64Counter("noumena_email_retry_total")
}

func sumEmailSent(rm *metricdata.ResourceMetrics, template, outcome string) int64 {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "noumena_emails_sent_total" {
				continue
			}
			s, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range s.DataPoints {
				tpl, _ := dp.Attributes.Value(attribute.Key(telemetry.AttrTemplate))
				oc, _ := dp.Attributes.Value(attribute.Key(telemetry.AttrOutcome))
				if tpl.AsString() == template && oc.AsString() == outcome {
					return dp.Value
				}
			}
		}
	}
	return 0
}

// fakeEmailStore implements db.EmailLogStore with a single-row
// in-memory table just rich enough to drive Send's dedupe path.
type fakeEmailStore struct {
	usedKey string // when non-empty, IsEmailIdempotencyKeyUsed returns true for this key
	logged  bool   // tripped if LogEmailAttempt is reached (test fails — dedupe was bypassed)
}

func (f *fakeEmailStore) IsEmailIdempotencyKeyUsed(_ context.Context, k string) (bool, error) {
	return f.usedKey != "" && k == f.usedKey, nil
}

func (f *fakeEmailStore) LogEmailAttempt(_ context.Context, _ db.EmailAttemptInput) (string, error) {
	f.logged = true
	return "row-1", nil
}

func (f *fakeEmailStore) MarkEmailSent(_ context.Context, _ string, _ time.Time) error { return nil }
func (f *fakeEmailStore) IncrementEmailAttempt(_ context.Context, _ string, _ int, _ string) error {
	return nil
}
func (f *fakeEmailStore) MarkEmailFailed(_ context.Context, _ string, _ string) error { return nil }
func (f *fakeEmailStore) MarkEmailAttemptError(_ context.Context, _ string, _ string) error {
	return nil
}
func (f *fakeEmailStore) ClaimPendingEmails(_ context.Context, _ time.Time, _ int) ([]*db.EmailLogRow, error) {
	return nil, nil
}
func (f *fakeEmailStore) GetEmailRowByIdempotencyKey(_ context.Context, _ string) (*db.EmailLogRow, error) {
	return nil, nil
}

type recordingSender struct{ called bool }

func (s *recordingSender) Send(_ context.Context, _, _, _, _, _ string, _ []string) error {
	s.called = true
	return nil
}

func TestSendIdempotentDedupeRecordsMetricNoSMTP(t *testing.T) {
	reader, restore := installEmailManualReader(t)
	defer restore()

	store := &fakeEmailStore{usedKey: "k1"}
	sender := &recordingSender{}

	p := &Pipeline{
		Queries: store,
		Sender:  sender,
		Cfg: &config.Config{
			EmailEnabled:     true,
			EmailFromAddress: "test@example.com",
			EmailFromName:    "Test",
		},
	}

	// Use a template we know exists in templates/ — pick the first
	// one rendered by TestRenderAllTemplates. ExportReady takes
	// DownloadURL+ExpiresAt; render must succeed BEFORE the dedupe
	// check fires (Send renders up-front by design).
	err := p.Send(context.Background(), Message{
		Template:       TemplateExportReady,
		ToAddress:      "u@example.com",
		IdempotencyKey: "k1",
		Variables: map[string]any{
			"DownloadURL": "https://example.com/x",
			"ExpiresAt":   "soon",
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if sender.called {
		t.Fatalf("dedupe path must not call SMTP")
	}
	if store.logged {
		t.Fatalf("dedupe path must not insert email_send_log row")
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := sumEmailSent(&rm, TemplateExportReady, telemetry.OutcomeIdempotentDedupe); got != 1 {
		t.Errorf("idempotent_dedupe: want 1, got %d", got)
	}
	if got := sumEmailSent(&rm, TemplateExportReady, telemetry.OutcomeSuccess); got != 0 {
		t.Errorf("success bucket should be 0 on dedupe, got %d", got)
	}
}
