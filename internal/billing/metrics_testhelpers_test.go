package billing

// Test-only helpers. Lives in _test.go so production builds don't
// re-pull the SDK aggregation deps through this package's import
// surface (the deps are already pulled in by internal/telemetry,
// but keeping the boundary clean helps `go list -deps` audits).

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func installManualReader(t *testing.T) (*sdkmetric.ManualReader, func()) {
	t.Helper()
	prev := otel.GetMeterProvider()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	rebindBillingMeters()
	return reader, func() {
		_ = mp.Shutdown(context.Background())
		otel.SetMeterProvider(prev)
		rebindBillingMeters()
	}
}

func rebindBillingMeters() {
	billingMeter = otel.Meter("noumena/billing")
	stripeWebhookTotal, _ = billingMeter.Int64Counter("noumena_stripe_webhook_total")
	quotaUsage, _ = billingMeter.Int64UpDownCounter("noumena_quota_usage")
	webhookDurationSeconds, _ = billingMeter.Float64Histogram("noumena_webhook_duration_seconds")
}
