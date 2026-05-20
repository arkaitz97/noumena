package billing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"noumena/internal/telemetry"
)

func sumWebhook(rm *metricdata.ResourceMetrics, event, outcome string) int64 {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "noumena_stripe_webhook_total" {
				continue
			}
			s, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range s.DataPoints {
				ev, _ := dp.Attributes.Value(attribute.Key(telemetry.AttrEvent))
				oc, _ := dp.Attributes.Value(attribute.Key(telemetry.AttrOutcome))
				if ev.AsString() == event && oc.AsString() == outcome {
					return dp.Value
				}
			}
		}
	}
	return 0
}

// RecordStripeWebhook splits the (event, outcome) attribute pair across
// the closed-enum outcome and free-form event name. Verify the basic
// shape: each call increments exactly one (event, outcome) bucket.
func TestRecordStripeWebhookByOutcome(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	ctx := context.Background()
	RecordStripeWebhook(ctx, "invoice.payment_succeeded", telemetry.OutcomeSuccess)
	RecordStripeWebhook(ctx, "invoice.payment_succeeded", telemetry.OutcomeSuccess)
	RecordStripeWebhook(ctx, "invoice.payment_failed", telemetry.OutcomeHandlerError)
	RecordStripeWebhook(ctx, "", telemetry.OutcomeSignatureInvalid)
	RecordStripeWebhook(ctx, "checkout.session.completed", telemetry.OutcomeIdempotentDedupe)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if got := sumWebhook(&rm, "invoice.payment_succeeded", telemetry.OutcomeSuccess); got != 2 {
		t.Errorf("invoice.payment_succeeded/success: want 2, got %d", got)
	}
	if got := sumWebhook(&rm, "invoice.payment_failed", telemetry.OutcomeHandlerError); got != 1 {
		t.Errorf("invoice.payment_failed/handler_error: want 1, got %d", got)
	}
	// Spec §9.6: signature failure is recorded with empty event +
	// signature_invalid outcome.
	if got := sumWebhook(&rm, "", telemetry.OutcomeSignatureInvalid); got != 1 {
		t.Errorf("\"\"/signature_invalid: want 1, got %d", got)
	}
	if got := sumWebhook(&rm, "checkout.session.completed", telemetry.OutcomeIdempotentDedupe); got != 1 {
		t.Errorf("checkout/idempotent_dedupe: want 1, got %d", got)
	}
}

// Unknown outcomes get coerced to "error" by telemetry.Outcome — verify
// the bucket lands at outcome=error rather than the unknown literal.
func TestRecordStripeWebhookCoercesUnknownOutcome(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	ctx := context.Background()
	RecordStripeWebhook(ctx, "x", "totally-made-up")

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := sumWebhook(&rm, "x", telemetry.OutcomeError); got != 1 {
		t.Errorf("coerced outcome: want 1 at outcome=error, got %d", got)
	}
	if got := sumWebhook(&rm, "x", "totally-made-up"); got != 0 {
		t.Errorf("coerced outcome: literal bucket should be empty, got %d", got)
	}
}
