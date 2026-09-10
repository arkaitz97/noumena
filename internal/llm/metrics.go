package llm

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"noumena/internal/telemetry"
)

// otel-instrumentation: LLM-subsystem instruments. Resolved at package
// init time against the global MeterProvider; telemetry.Init runs in
// main() before any LLM call so the no-op vs SDK provider routing is
// settled by the time recordLLMRequest fires.
var llmMeter = otel.Meter("noumena/llm")
var llmTracer = otel.Tracer("noumena/llm")

var (
	llmRequestsTotal       metric.Int64Counter
	llmTokensTotal         metric.Int64Counter
	llmRequestDurationSecs metric.Float64Histogram
	llmFailoverTotal       metric.Int64Counter
	llmProviderCallsTotal  metric.Int64Counter
	llmJobAttemptsTotal    metric.Int64Counter
	llmParkedComments      atomic.Int64
	llmBatchesInFlight     atomic.Int64
)

func init() {
	var err error
	if llmRequestsTotal, err = llmMeter.Int64Counter(
		"noumena_llm_requests_total",
		metric.WithDescription("Total LLM ClassifyBatch invocations"),
	); err != nil {
		panic("noumena_llm_requests_total: " + err.Error())
	}
	if llmTokensTotal, err = llmMeter.Int64Counter(
		"noumena_llm_tokens_total",
		metric.WithDescription("Total LLM tokens reported by provider responses"),
	); err != nil {
		panic("noumena_llm_tokens_total: " + err.Error())
	}
	if llmRequestDurationSecs, err = llmMeter.Float64Histogram(
		"noumena_llm_request_duration_seconds",
		metric.WithDescription("LLM request duration"),
		metric.WithUnit("s"),
	); err != nil {
		panic("noumena_llm_request_duration_seconds: " + err.Error())
	}
	if llmFailoverTotal, err = llmMeter.Int64Counter(
		"noumena_llm_failover_total",
		metric.WithDescription("Provider failover events, by from/to provider pair"),
	); err != nil {
		panic("noumena_llm_failover_total: " + err.Error())
	}
	if llmProviderCallsTotal, err = llmMeter.Int64Counter(
		"noumena_llm_provider_calls_total",
		metric.WithDescription("HTTP attempts against a provider, including in-retry attempts (duplicate-work canary)"),
	); err != nil {
		panic("noumena_llm_provider_calls_total: " + err.Error())
	}
	if llmJobAttemptsTotal, err = llmMeter.Int64Counter(
		"noumena_llm_job_attempts_total",
		metric.WithDescription("Classification job terminal states, by outcome"),
	); err != nil {
		panic("noumena_llm_job_attempts_total: " + err.Error())
	}
	if _, err := llmMeter.Int64ObservableGauge(
		"noumena_llm_parked_comments",
		metric.WithDescription("Unprocessed comments excluded from claiming at the attempt cap"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(llmParkedComments.Load())
			return nil
		}),
	); err != nil {
		panic("noumena_llm_parked_comments: " + err.Error())
	}
	if _, err := llmMeter.Int64ObservableGauge(
		"noumena_llm_batches_in_flight",
		metric.WithDescription("Channel batches currently being processed (effective concurrency)"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(llmBatchesInFlight.Load())
			return nil
		}),
	); err != nil {
		panic("noumena_llm_batches_in_flight: " + err.Error())
	}
}

// llmOutcome maps an LLM call's result onto the closed outcome enum
// from the spec: success / error / timeout. context.DeadlineExceeded is
// the only timeout signal we recognise — provider-side timeouts that
// don't wrap it surface as plain "error".
func llmOutcome(err error) string {
	if err == nil {
		return telemetry.OutcomeSuccess
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return telemetry.OutcomeTimeout
	}
	return telemetry.OutcomeError
}

// recordLLMRequest emits the (request_total, request_duration) pair
// with the (provider, model, outcome) attributes documented in the
// spec. Tokens are recorded separately via recordLLMTokens because the
// LLMClient interface does not currently carry a token count — once it
// does (design.md "Open Questions"), the call site will pass tokens.
func recordLLMRequest(ctx context.Context, provider, model string, start time.Time, err error) {
	outcome := llmOutcome(err)
	attrs := metric.WithAttributes(
		attribute.String(telemetry.AttrProvider, provider),
		attribute.String(telemetry.AttrModel, model),
		attribute.String(telemetry.AttrOutcome, outcome),
	)
	llmRequestsTotal.Add(ctx, 1, attrs)
	llmRequestDurationSecs.Record(ctx, time.Since(start).Seconds(), attrs)
}

// recordLLMTokens increments noumena_llm_tokens_total by `tokens`. Safe
// to call with 0 (the spec contract: "or 0 if unreported").
func recordLLMTokens(ctx context.Context, provider, model string, tokens int64) {
	if tokens <= 0 {
		return
	}
	llmTokensTotal.Add(ctx, tokens, metric.WithAttributes(
		attribute.String(telemetry.AttrProvider, provider),
		attribute.String(telemetry.AttrModel, model),
	))
}

// traceWithAttrs builds the StartOption set for the LLM span. Centralised
// so the attribute keys stay consistent with the metric labels.
func traceWithAttrs(provider, model string, batchSize int) trace.SpanStartOption {
	return trace.WithAttributes(
		attribute.String("llm.provider", provider),
		attribute.String("llm.model", model),
		attribute.Int("llm.batch_size", batchSize),
	)
}

// recordLLMFailover increments noumena_llm_failover_total with the
// (from_provider, to_provider) attributes. Called when the failover
// wrapper switches from a failing primary to the fallback provider.
func recordLLMFailover(ctx context.Context, fromProvider, toProvider string) {
	llmFailoverTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String(telemetry.AttrFromProvider, fromProvider),
		attribute.String(telemetry.AttrToProvider, toProvider),
	))
}

// recordLLMProviderCall increments noumena_llm_provider_calls_total for one
// HTTP attempt (including in-retry attempts). The ratio to distinct jobs is
// the duplicate-work canary.
func recordLLMProviderCall(ctx context.Context, provider string, retryable bool) {
	llmProviderCallsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String(telemetry.AttrProvider, provider),
		attribute.String(telemetry.AttrRetryable, strconv.FormatBool(retryable)),
	))
}

// recordLLMJobAttempt increments noumena_llm_job_attempts_total with the
// closed outcome enum: completed|failed_transient|failed_terminal|parked.
func recordLLMJobAttempt(ctx context.Context, provider, outcome string) {
	llmJobAttemptsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String(telemetry.AttrProvider, provider),
		telemetry.Outcome(outcome),
	))
}

// recordLLMParkedComments stores the sampled dead-letter depth; the
// observable gauge reads it on scrape.
func recordLLMParkedComments(n int64) {
	llmParkedComments.Store(n)
}

// recordLLMBatchInFlight tracks how many channel batches are actively being
// processed. Call with +1 on acquire and -1 on release; the observable gauge
// reads the running total on scrape.
func recordLLMBatchInFlight(_ context.Context, delta int64) {
	llmBatchesInFlight.Add(delta)
}
