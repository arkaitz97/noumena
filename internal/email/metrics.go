package email

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"noumena/internal/telemetry"
)

// otel-instrumentation: email pipeline meter ("noumena/email").
//
// Two instruments live here:
//   - emails_sent_total{template,outcome}: every terminal Send outcome
//   - email_retry_total{template}: every retry-worker attempt
//
// The "template" attribute is unbounded in principle but bounded in
// practice (every TemplateXxx constant in templates.go); we accept
// the attribute cardinality.
var emailMeter = otel.Meter("noumena/email")

var (
	emailsSentTotal metric.Int64Counter
	emailRetryTotal metric.Int64Counter
)

func init() {
	var err error
	emailsSentTotal, err = emailMeter.Int64Counter(
		"noumena_emails_sent_total",
		metric.WithDescription("Email Send terminal outcomes, by template and outcome"),
	)
	if err != nil {
		panic("noumena_emails_sent_total: " + err.Error())
	}
	emailRetryTotal, err = emailMeter.Int64Counter(
		"noumena_email_retry_total",
		metric.WithDescription("Retry-worker attempts, by template"),
	)
	if err != nil {
		panic("noumena_email_retry_total: " + err.Error())
	}
}

// recordEmailSent increments the terminal-outcome counter. We accept
// "" template defensively (rendering errors before the template is
// validated would otherwise drop the metric).
func recordEmailSent(ctx context.Context, template, outcome string) {
	emailsSentTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String(telemetry.AttrTemplate, template),
		telemetry.Outcome(outcome),
	))
}

func recordEmailRetry(ctx context.Context, template string) {
	emailRetryTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String(telemetry.AttrTemplate, template),
	))
}
