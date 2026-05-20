package billing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"noumena/internal/telemetry"
)

// otel-instrumentation: billing webhook counter.
//
// Scope: "noumena/billing" — the handler itself lives under
// internal/api/billing_handlers.go, but the metric belongs to the
// billing subsystem in dashboards. We expose a record helper instead
// of the instrument directly so the handler doesn't depend on OTel.
var billingMeter = otel.Meter("noumena/billing")

var stripeWebhookTotal metric.Int64Counter

func init() {
	var err error
	stripeWebhookTotal, err = billingMeter.Int64Counter(
		"noumena_stripe_webhook_total",
		metric.WithDescription("Stripe webhook deliveries received, by event type and outcome"),
	)
	if err != nil {
		panic("noumena_stripe_webhook_total: " + err.Error())
	}
}

// RecordStripeWebhook increments the webhook counter. Outcomes are
// the closed enum {success, signature_invalid, handler_error, ignored,
// idempotent_dedupe}; unknown values are coerced via telemetry.Outcome.
//
// `event` is the Stripe event type (e.g. "invoice.payment_succeeded")
// or "" when signature verification fails before the body parses —
// dashboards should treat empty event as a generic "unverified" bucket.
func RecordStripeWebhook(ctx context.Context, event, outcome string) {
	stripeWebhookTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String(telemetry.AttrEvent, event),
		telemetry.Outcome(outcome),
	))
}
