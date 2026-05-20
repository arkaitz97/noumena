package ingestion

// Test-only helpers for the otel-instrumentation contract. Lives in
// _test.go so production builds don't pull in the SDK aggregation deps.

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// installManualReader swaps the global MeterProvider for a ManualReader-
// backed SDK provider for the duration of a test. The package's
// instruments resolve their underlying aggregator lazily through the
// global, so swapping here re-wires every counter / histogram in the
// package without re-running init().
//
// IMPORTANT: this mutates a package-global. Tests that depend on it
// must be in the same package (not _test) to share state, and they
// must not run in parallel — t.Parallel() would race the swap.
func installManualReader(t *testing.T) (*sdkmetric.ManualReader, func()) {
	t.Helper()
	prev := otel.GetMeterProvider()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	// Re-resolve package-level instruments against the new provider.
	// The OTel global pattern documents this as required when the
	// provider changes after init: existing handles keep pointing at
	// the previous provider's pipelines.
	rebindIngestionMeters()
	return reader, func() {
		_ = mp.Shutdown(context.Background())
		otel.SetMeterProvider(prev)
		rebindIngestionMeters()
	}
}

// rebindIngestionMeters re-resolves the package-level instruments from
// the current global MeterProvider. Called from installManualReader's
// install + teardown so each test sees a fresh recording surface.
func rebindIngestionMeters() {
	ingestionMeter = otel.Meter("noumena/ingestion")
	commentsIngestedTotal, _ = ingestionMeter.Int64Counter("noumena_comments_ingested_total")
	ingestionErrorsTotal, _ = ingestionMeter.Int64Counter("noumena_ingestion_errors_total")
	ingestionLockContentionTot, _ = ingestionMeter.Int64Counter("noumena_ingestion_lock_contention_total")
	fetchDurationSeconds, _ = ingestionMeter.Float64Histogram("noumena_fetch_duration_seconds")
}

// sumInt64 returns the total Int64 sum across all attribute sets for
// the named instrument, or 0 if it does not exist in rm.
func sumInt64(rm *metricdata.ResourceMetrics, name string) int64 {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			if s, ok := m.Data.(metricdata.Sum[int64]); ok {
				var total int64
				for _, dp := range s.DataPoints {
					total += dp.Value
				}
				return total
			}
		}
	}
	return 0
}

// sumInt64WithAttr returns the value of the data point whose attribute
// `key` equals `val`. Returns 0 if absent.
func sumInt64WithAttr(rm *metricdata.ResourceMetrics, name, key, val string) int64 {
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
				v, exists := dp.Attributes.Value(attribute.Key(key))
				if exists && v.AsString() == val {
					return dp.Value
				}
			}
		}
	}
	return 0
}
