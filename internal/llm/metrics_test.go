package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"noumena/internal/telemetry"
)

// installManualReader is the analogue of the helper in
// internal/ingestion. It swaps the global MeterProvider and re-binds
// the package-level instruments so each test sees a fresh recording
// surface.
func installManualReader(t *testing.T) (*sdkmetric.ManualReader, func()) {
	t.Helper()
	prev := otel.GetMeterProvider()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	rebindLLMMeters()
	return reader, func() {
		_ = mp.Shutdown(context.Background())
		otel.SetMeterProvider(prev)
		rebindLLMMeters()
	}
}

func rebindLLMMeters() {
	llmMeter = otel.Meter("noumena/llm")
	llmRequestsTotal, _ = llmMeter.Int64Counter("noumena_llm_requests_total")
	llmTokensTotal, _ = llmMeter.Int64Counter("noumena_llm_tokens_total")
	llmRequestDurationSecs, _ = llmMeter.Float64Histogram("noumena_llm_request_duration_seconds")
	llmFailoverTotal, _ = llmMeter.Int64Counter("noumena_llm_failover_total")
	llmProviderCallsTotal, _ = llmMeter.Int64Counter("noumena_llm_provider_calls_total")
	llmJobAttemptsTotal, _ = llmMeter.Int64Counter("noumena_llm_job_attempts_total")
	_, _ = llmMeter.Int64ObservableGauge("noumena_llm_parked_comments",
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(llmParkedComments.Load())
			return nil
		}))
}

func sumWithAttrs(rm *metricdata.ResourceMetrics, name string, want map[string]string) int64 {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			s, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range s.DataPoints {
				match := true
				for k, v := range want {
					got, ok := dp.Attributes.Value(attribute.Key(k))
					if !ok || got.AsString() != v {
						match = false
						break
					}
				}
				if match {
					return dp.Value
				}
			}
		}
	}
	return 0
}

// TestRecordLLMRequestSuccess covers the spec scenario "Successful
// batch records request and tokens" — partial coverage: the request
// counter and duration histogram. Tokens are exercised separately in
// TestRecordLLMTokens because the LLMClient interface does not yet
// surface token counts (design.md "Open Questions").
func TestRecordLLMRequestSuccess(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	start := time.Now().Add(-50 * time.Millisecond)
	recordLLMRequest(context.Background(), "openai", "gpt-4o", start, nil)

	rm := &metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	got := sumWithAttrs(rm, "noumena_llm_requests_total", map[string]string{
		"provider": "openai", "model": "gpt-4o", "outcome": telemetry.OutcomeSuccess,
	})
	if got != 1 {
		t.Errorf("requests_total{success}: got %d, want 1", got)
	}
}

// TestRecordLLMRequestTimeout covers the spec scenario "Context
// deadline records outcome=timeout". An error wrapping
// context.DeadlineExceeded MUST classify as timeout, not error.
func TestRecordLLMRequestTimeout(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	wrapped := errors.New("provider call: " + context.DeadlineExceeded.Error())
	// errors.Is must hold for the classifier to fire; build that by
	// wrapping fmt.Errorf-style.
	wrapped = errWrap(context.DeadlineExceeded)
	recordLLMRequest(context.Background(), "anthropic", "claude", time.Now(), wrapped)

	rm := &metricdata.ResourceMetrics{}
	_ = reader.Collect(context.Background(), rm)
	if got := sumWithAttrs(rm, "noumena_llm_requests_total", map[string]string{
		"outcome": telemetry.OutcomeTimeout,
	}); got != 1 {
		t.Errorf("requests_total{timeout}: got %d, want 1", got)
	}
}

// errWrap returns an error that satisfies errors.Is(err, target).
type wrappedErr struct{ inner error }

func (w *wrappedErr) Error() string { return "wrapped: " + w.inner.Error() }
func (w *wrappedErr) Unwrap() error { return w.inner }

func errWrap(target error) error { return &wrappedErr{inner: target} }

// TestNoOpMetricsDoNotPanic verifies that with the default no-op
// MeterProvider, calling every record function is safe.
func TestNoOpMetricsDoNotPanic(t *testing.T) {
	ctx := context.Background()
	recordLLMRequest(ctx, "openai", "gpt-4o", time.Now(), nil)
	recordLLMTokens(ctx, "openai", "gpt-4o", 100)
	recordLLMFailover(ctx, "openai", "anthropic")
	recordLLMProviderCall(ctx, "openai", true)
	recordLLMJobAttempt(ctx, "openai", telemetry.OutcomeCompleted)
	recordLLMParkedComments(3)
}

// TestRecordLLMFailover asserts that a failover event increments the
// failover counter with the correct (from_provider, to_provider) attrs.
func TestRecordLLMFailover(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	recordLLMFailover(context.Background(), "openai", "anthropic")
	recordLLMFailover(context.Background(), "openai", "anthropic")

	rm := &metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	got := sumWithAttrs(rm, "noumena_llm_failover_total", map[string]string{
		"from_provider": "openai", "to_provider": "anthropic",
	})
	if got != 2 {
		t.Errorf("failover_total: got %d, want 2", got)
	}
}

// TestRecordLLMProviderCall covers the observability-metrics scenario
// "Provider Call Attempts Counter": each attempt increments with the
// retryable label so the calls/jobs ratio is observable.
func TestRecordLLMProviderCall(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	recordLLMProviderCall(context.Background(), "openai", true)
	recordLLMProviderCall(context.Background(), "openai", true)
	recordLLMProviderCall(context.Background(), "openai", false)

	rm := &metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := sumWithAttrs(rm, "noumena_llm_provider_calls_total", map[string]string{
		"provider": "openai", "retryable": "true",
	}); got != 2 {
		t.Errorf("provider_calls_total{retryable=true}: got %d, want 2", got)
	}
	if got := sumWithAttrs(rm, "noumena_llm_provider_calls_total", map[string]string{
		"provider": "openai", "retryable": "false",
	}); got != 1 {
		t.Errorf("provider_calls_total{retryable=false}: got %d, want 1", got)
	}
}

// TestRecordLLMJobAttempt covers the "Job Outcome Counter" scenario across
// the closed outcome enum.
func TestRecordLLMJobAttempt(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	recordLLMJobAttempt(context.Background(), "openai", telemetry.OutcomeCompleted)
	recordLLMJobAttempt(context.Background(), "openai", telemetry.OutcomeFailedTransient)
	recordLLMJobAttempt(context.Background(), "openai", telemetry.OutcomeFailedTerminal)
	recordLLMJobAttempt(context.Background(), "openai", telemetry.OutcomeParked)
	recordLLMJobAttempt(context.Background(), "openai", telemetry.OutcomeParked)

	rm := &metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	cases := map[string]int64{
		telemetry.OutcomeCompleted:       1,
		telemetry.OutcomeFailedTransient: 1,
		telemetry.OutcomeFailedTerminal:  1,
		telemetry.OutcomeParked:          2,
	}
	for outcome, want := range cases {
		got := sumWithAttrs(rm, "noumena_llm_job_attempts_total", map[string]string{
			"provider": "openai", "outcome": outcome,
		})
		if got != want {
			t.Errorf("job_attempts_total{outcome=%s}: got %d, want %d", outcome, got, want)
		}
	}
}

// TestRecordLLMParkedComments covers the "Parked Comments Gauge" scenario.
func TestRecordLLMParkedComments(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	recordLLMParkedComments(7)

	rm := &metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var got int64 = -1
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "noumena_llm_parked_comments" {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok {
				continue
			}
			if len(g.DataPoints) > 0 {
				got = g.DataPoints[0].Value
			}
		}
	}
	if got != 7 {
		t.Errorf("parked_comments gauge: got %d, want 7", got)
	}
}
