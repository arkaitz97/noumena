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

var (
	stripeWebhookTotal     metric.Int64Counter
	quotaUsage             metric.Int64UpDownCounter
	webhookDurationSeconds metric.Float64Histogram
)

func init() {
	var err error
	stripeWebhookTotal, err = billingMeter.Int64Counter(
		"noumena_stripe_webhook_total",
		metric.WithDescription("Stripe webhook deliveries received, by event type and outcome"),
	)
	if err != nil {
		panic("noumena_stripe_webhook_total: " + err.Error())
	}
	quotaUsage, err = billingMeter.Int64UpDownCounter(
		"noumena_quota_usage",
		metric.WithDescription("Current comment quota usage by user and plan"),
	)
	if err != nil {
		panic("noumena_quota_usage: " + err.Error())
	}
	webhookDurationSeconds, err = billingMeter.Float64Histogram(
		"noumena_webhook_duration_seconds",
		metric.WithDescription("Webhook handler processing duration"),
		metric.WithUnit("s"),
	)
	if err != nil {
		panic("noumena_webhook_duration_seconds: " + err.Error())
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

// RecordQuotaUsage sets the quota usage gauge for a user.
func RecordQuotaUsage(ctx context.Context, userID, plan string, used, limit int64) {
	quotaUsage.Add(ctx, used, metric.WithAttributes(
		attribute.String("user_id", userID),
		attribute.String("plan", plan),
	))
}

// RecordWebhookDuration records the webhook handler duration.
func RecordWebhookDuration(ctx context.Context, event string, duration float64) {
	webhookDurationSeconds.Record(ctx, duration, metric.WithAttributes(
		attribute.String(telemetry.AttrEvent, event),
	))
}
