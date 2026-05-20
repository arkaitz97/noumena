package llm

import (
	"context"
	"errors"
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
	llmFailoverSwitchTotal metric.Int64Counter
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
	if llmFailoverSwitchTotal, err = llmMeter.Int64Counter(
		"noumena_llm_failover_switch_total",
		metric.WithDescription("Failover-wrapper provider switches"),
	); err != nil {
		panic("noumena_llm_failover_switch_total: " + err.Error())
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

// recordLLMFailoverSwitch is invoked by failover.go when the wrapper
// transitions between primary and fallback providers.
func recordLLMFailoverSwitch(ctx context.Context, fromProvider, toProvider string) {
	llmFailoverSwitchTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String(telemetry.AttrFromProvider, fromProvider),
		attribute.String(telemetry.AttrToProvider, toProvider),
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
