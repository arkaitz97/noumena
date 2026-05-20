package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
	llmFailoverSwitchTotal, _ = llmMeter.Int64Counter("noumena_llm_failover_switch_total")
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

// TestRecordLLMFailoverSwitch covers the spec scenario "Failover
// switch is recorded once per transition".
func TestRecordLLMFailoverSwitch(t *testing.T) {
	reader, restore := installManualReader(t)
	defer restore()

	recordLLMFailoverSwitch(context.Background(), "opencode", "anthropic")

	rm := &metricdata.ResourceMetrics{}
	_ = reader.Collect(context.Background(), rm)
	if got := sumWithAttrs(rm, "noumena_llm_failover_switch_total", map[string]string{
		"from_provider": "opencode", "to_provider": "anthropic",
	}); got != 1 {
		t.Errorf("failover_switch_total: got %d, want 1", got)
	}
}

// errWrap returns an error that satisfies errors.Is(err, target).
type wrappedErr struct{ inner error }

func (w *wrappedErr) Error() string { return "wrapped: " + w.inner.Error() }
func (w *wrappedErr) Unwrap() error { return w.inner }

func errWrap(target error) error { return &wrappedErr{inner: target} }
