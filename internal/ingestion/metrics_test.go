package ingestion

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"noumena/internal/telemetry"
)

// TestRecordCommentsIngested exercises the spec scenario "Successful
// batch increments comments_ingested_total by batch size" without
// running the full worker — recordCommentsIngested is the call site the
// worker uses, and asserting on it isolates the test from the
// YouTube/DB plumbing. The package-level instruments bind to the global
// MeterProvider at init() time; we use a helper to swap in an SDK
// MeterProvider with a ManualReader for the duration of the test.
func TestRecordCommentsIngested(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	recordCommentsIngested(context.Background(), 7)

	rm := &metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	got := sumInt64(rm, "noumena_comments_ingested_total")
	if got != 7 {
		t.Errorf("comments_ingested_total: got %d, want 7", got)
	}
}

// TestClassifyIngestionError covers the closed reason enum from the
// spec: every error type the worker can encounter MUST map to one of
// the documented reasons (rate_limited / token_invalid /
// deadline_exceeded / other). Unknown errors fall through to "other"
// rather than panicking — see metrics.go classifyIngestionError.
func TestClassifyIngestionError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"rate_limited", &RateLimitError{}, telemetry.ReasonRateLimited},
		{"token_invalid", &TokenInvalidError{}, telemetry.ReasonTokenInvalid},
		{"deadline", context.DeadlineExceeded, telemetry.ReasonDeadlineExceeded},
		{"unknown", &dummyErr{}, telemetry.ReasonOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyIngestionError(tc.err); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRecordIngestionErrorRateLimited asserts the spec scenario
// "Rate-limit error is recorded with reason=rate_limited". We call the
// recording helper directly; the worker's processJob calls into the
// same helper, so this also covers the worker path by reduction.
func TestRecordIngestionErrorRateLimited(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	recordIngestionError(context.Background(), telemetry.ReasonRateLimited)

	rm := &metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := sumInt64WithAttr(rm, "noumena_ingestion_errors_total", "reason", telemetry.ReasonRateLimited); got != 1 {
		t.Errorf("ingestion_errors_total{reason=rate_limited}: got %d, want 1", got)
	}
}

type dummyErr struct{}

func (dummyErr) Error() string { return "dummy" }

// TestNoOpMetricsDoNotPanic verifies that with the default no-op
// MeterProvider (no SDK initialized), calling every record function is
// safe and does not panic. This is the zero-cost guarantee from the
// spec: when OTEL_ENABLED=false, instrumentation calls short-circuit
// inside the OTel SDK without any measurable overhead.
func TestNoOpMetricsDoNotPanic(t *testing.T) {
	ctx := context.Background()
	// Default global MeterProvider is no-op; these calls must not panic.
	recordCommentsIngested(ctx, 5)
	recordIngestionError(ctx, telemetry.ReasonRateLimited)
	recordFetchDuration(ctx, time.Now().Add(-50*time.Millisecond), telemetry.OutcomeSuccess)
	recordLockContention(ctx)
}
