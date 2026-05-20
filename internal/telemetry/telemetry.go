// Package telemetry wires the OpenTelemetry MeterProvider and
// TracerProvider for the noumena-api process. The single entry point is
// Init(ctx, cfg) which returns a shutdown callback. When
// cfg.OTELEnabled is false (the documented default), Init installs the
// SDK's no-op providers so instrumentation call sites are zero-cost,
// allocation-free no-ops — see design.md decision 2.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"noumena/internal/config"
)

// ShutdownFunc flushes pending telemetry and tears down providers. Safe
// to invoke multiple times; second and subsequent calls are no-ops.
type ShutdownFunc func(context.Context) error

// LLMDurationBoundaries is the explicit bucket boundary set used for
// noumena_fetch_duration_seconds and noumena_llm_request_duration_seconds.
// The spec calls for an exponential layout from 50 ms to 2 minutes
// because YouTube fetches and LLM calls regularly span two orders of
// magnitude (design.md Decision 5).
var LLMDurationBoundaries = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120}

// Init configures the global OpenTelemetry providers based on cfg. When
// cfg.OTELEnabled == false, the no-op providers are installed and the
// returned shutdown is a no-op. When true, an OTLP gRPC exporter is
// wired with a PeriodicReader (metrics) and a batch span processor
// (traces); the providers are set as OTel globals so subsystems can
// resolve their meter via otel.Meter("noumena/<subsystem>") without
// importing this package.
func Init(ctx context.Context, cfg *config.Config) (ShutdownFunc, error) {
	if cfg == nil {
		return nil, errors.New("telemetry.Init: cfg is nil")
	}
	if !cfg.OTELEnabled {
		// Disabled path: install OTel-provided no-op providers. The
		// no-op SDK is documented allocation-free and lock-free so
		// every Counter.Add / Histogram.Record at a call site
		// short-circuits in the meter handle itself.
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		return noopShutdown, nil
	}
	if cfg.OTELExporterOTLPEndpoint == "" {
		// Defence-in-depth: config.Load already enforces this, but a
		// caller building a Config struct in code (tests, fixtures)
		// could bypass the validation. Fail fast and do NOT mutate
		// the global providers — Init must be atomic per the spec.
		return nil, errors.New("OTEL_ENABLED=true but OTEL_EXPORTER_OTLP_ENDPOINT is empty")
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	// --- Metrics ----------------------------------------------------------
	// otlpmetricgrpc.New dials lazily, so an unreachable collector at
	// startup does not fail Init — see design.md "Risks / Trade-offs".
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTELExporterOTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: otlp metric exporter: %w", err)
	}
	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExp)),
		// Custom buckets only for the two histograms documented in the
		// spec; everything else inherits the SDK defaults (Decision 5).
		metric.WithView(metric.NewView(
			metric.Instrument{Name: "noumena_fetch_duration_seconds"},
			metric.Stream{Aggregation: metric.AggregationExplicitBucketHistogram{Boundaries: LLMDurationBoundaries}},
		)),
		metric.WithView(metric.NewView(
			metric.Instrument{Name: "noumena_llm_request_duration_seconds"},
			metric.Stream{Aggregation: metric.AggregationExplicitBucketHistogram{Boundaries: LLMDurationBoundaries}},
		)),
	)
	otel.SetMeterProvider(mp)

	// --- Traces -----------------------------------------------------------
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTELExporterOTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		// Roll back the meter provider so we don't leave the process
		// in a half-configured state.
		_ = mp.Shutdown(context.Background())
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		return nil, fmt.Errorf("telemetry: otlp trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExp),
	)
	otel.SetTracerProvider(tp)

	shutdown := func(ctx context.Context) error {
		// Bound shutdown by the caller's deadline. main.go applies a
		// 5 s deadline (graceful-shutdown sequence); spec says losing
		// the last buffer is preferable to hanging SIGTERM.
		var errs []error
		if err := mp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider: %w", err))
		}
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer provider: %w", err))
		}
		// After shutdown subsequent instrument calls become no-ops
		// because the underlying readers/processors reject new data.
		return errors.Join(errs...)
	}
	return shutdown, nil
}

func noopShutdown(context.Context) error { return nil }

// buildResource constructs the OTel Resource carrying service.name and
// the merged OTEL_RESOURCE_ATTRIBUTES pairs. Malformed pairs (no `=`
// separator, empty key) are skipped with no error — the spec requires
// best-effort merge so a single typo doesn't poison the deployment.
func buildResource(ctx context.Context, cfg *config.Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.OTELServiceName),
	}
	for _, pair := range strings.Split(cfg.OTELResourceAttributes, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.IndexByte(pair, '=')
		if idx <= 0 || idx == len(pair)-1 {
			continue // malformed: no `=`, empty key, or empty value
		}
		k := strings.TrimSpace(pair[:idx])
		v := strings.TrimSpace(pair[idx+1:])
		if k == "" || v == "" {
			continue
		}
		attrs = append(attrs, attribute.String(k, v))
	}
	// resource.New merges with the SDK defaults (process.runtime.* etc).
	// We pass a short context so a slow detector cannot stall startup.
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return resource.New(rctx,
		resource.WithAttributes(attrs...),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
}
