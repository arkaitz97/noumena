package telemetry_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"noumena/internal/config"
	"noumena/internal/telemetry"
)

// TestInitDisabledInstallsNoop verifies the disabled path: Init must
// install the OTel-provided no-op providers and return a non-nil
// shutdown that is itself a no-op. This is the contract every
// instrumentation call site relies on for zero-cost no-ops.
func TestInitDisabledInstallsNoop(t *testing.T) {
	cfg := &config.Config{OTELEnabled: false}
	shutdown, err := telemetry.Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init(disabled): %v", err)
	}
	if shutdown == nil {
		t.Fatalf("Init(disabled): shutdown is nil")
	}
	if _, ok := otel.GetMeterProvider().(metricnoop.MeterProvider); !ok {
		t.Errorf("MeterProvider: want noop, got %T", otel.GetMeterProvider())
	}
	if _, ok := otel.GetTracerProvider().(tracenoop.TracerProvider); !ok {
		t.Errorf("TracerProvider: want noop, got %T", otel.GetTracerProvider())
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown(disabled): %v", err)
	}
}

// TestInitEnabledRequiresEndpoint mirrors the spec scenario "Init fails
// fast on misconfiguration". The function MUST not mutate globals.
func TestInitEnabledRequiresEndpoint(t *testing.T) {
	// Reset to a known noop baseline so we can detect mutation.
	otel.SetMeterProvider(metricnoop.NewMeterProvider())
	otel.SetTracerProvider(tracenoop.NewTracerProvider())

	cfg := &config.Config{OTELEnabled: true, OTELExporterOTLPEndpoint: ""}
	if _, err := telemetry.Init(context.Background(), cfg); err == nil {
		t.Fatalf("Init(enabled, empty endpoint): want error, got nil")
	}
	// Globals must still be the noops we installed above.
	if _, ok := otel.GetMeterProvider().(metricnoop.MeterProvider); !ok {
		t.Errorf("MeterProvider mutated despite Init error: %T", otel.GetMeterProvider())
	}
}

// TestInitEnabledFakeEndpoint exercises the enabled path with a
// syntactically valid (but non-listening) endpoint. The OTLP gRPC
// exporter dials lazily so Init must succeed; shutdown must return
// within a bounded deadline. This is the "Init with a fake OTLP
// endpoint" scenario in tasks 3.11.
func TestInitEnabledFakeEndpoint(t *testing.T) {
	cfg := &config.Config{
		OTELEnabled:              true,
		OTELExporterOTLPEndpoint: "127.0.0.1:1", // unbound port; lazy dial
		OTELServiceName:          "noumena-api-test",
		OTELResourceAttributes:   "deployment.environment=test,team=platform",
	}
	shutdown, err := telemetry.Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init(enabled): %v", err)
	}
	defer func() {
		// Ensure other tests get a clean noop baseline back.
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	_ = shutdown(ctx) // exporter unreachable → may error, must return
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("shutdown took %v; expected bounded by ctx", d)
	}
}

// TestInitMalformedAttributesAreSkipped verifies the spec scenario
// "Resource attributes are merged into the resource" — malformed pairs
// (no `=`, empty key/value) MUST be silently skipped without failing.
func TestInitMalformedAttributesAreSkipped(t *testing.T) {
	cfg := &config.Config{
		OTELEnabled:              true,
		OTELExporterOTLPEndpoint: "127.0.0.1:1",
		OTELServiceName:          "noumena-api-test",
		// "broken" has no '='; "=v" has empty key; ",,," is empty pairs
		OTELResourceAttributes: "broken,=v,k=,good=ok,,,",
	}
	shutdown, err := telemetry.Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init(malformed attrs): want nil error, got %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = shutdown(ctx)
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
	}()
}

// TestAttrsClosedEnums asserts every documented enum value round-trips
// through ValidReason / ValidOutcome. This is the cardinality firewall
// the design.md "Risks" section requires: any future addition to the
// reason/outcome lists MUST update this test in lockstep with the spec.
func TestAttrsClosedEnums(t *testing.T) {
	reasons := []string{
		telemetry.ReasonRateLimited, telemetry.ReasonTokenInvalid,
		telemetry.ReasonDeadlineExceeded, telemetry.ReasonDeadLettered,
		telemetry.ReasonPanic, telemetry.ReasonOther,
	}
	for _, r := range reasons {
		if !telemetry.ValidReason(r) {
			t.Errorf("ValidReason(%q) = false", r)
		}
	}
	if telemetry.ValidReason("not_a_reason") {
		t.Errorf("ValidReason: unknown value accepted")
	}
	outcomes := []string{
		telemetry.OutcomeSuccess, telemetry.OutcomeError, telemetry.OutcomeTimeout,
		telemetry.OutcomeAbortedNewTracker, telemetry.OutcomeSignatureInvalid,
		telemetry.OutcomeHandlerError, telemetry.OutcomeIgnored, telemetry.OutcomeIdempotentDedupe,
	}
	for _, o := range outcomes {
		if !telemetry.ValidOutcome(o) {
			t.Errorf("ValidOutcome(%q) = false", o)
		}
	}
	// Reason() / Outcome() coerce unknown to a safe default rather
	// than panic, per the production-side contract in attrs.go.
	got := telemetry.Reason("free-form-error-string-from-some-handler")
	if v := got.Value.AsString(); !strings.EqualFold(v, telemetry.ReasonOther) {
		t.Errorf("Reason coerce: got %q, want %q", v, telemetry.ReasonOther)
	}
}
