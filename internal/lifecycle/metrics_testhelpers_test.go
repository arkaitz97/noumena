package lifecycle

// Test-only helpers mirroring internal/ingestion/metrics_testhelpers_test.go.
// See that file for the rationale on why we swap the global
// MeterProvider and rebind package-level instruments.

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func installManualReader(t *testing.T) (*sdkmetric.ManualReader, func()) {
	t.Helper()
	prev := otel.GetMeterProvider()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	rebindLifecycleMeters()
	return reader, func() {
		_ = mp.Shutdown(context.Background())
		otel.SetMeterProvider(prev)
		rebindLifecycleMeters()
	}
}

func rebindLifecycleMeters() {
	lifecycleMeter = otel.Meter("noumena/lifecycle")
	deletionsCompletedTotal, _ = lifecycleMeter.Int64Counter("noumena_deletions_completed_total")
	deletionChunkRows, _ = lifecycleMeter.Int64Histogram("noumena_deletion_chunk_rows")
	exportsCompletedTotal, _ = lifecycleMeter.Int64Counter("noumena_exports_completed_total")
	exportBytesTotal, _ = lifecycleMeter.Int64Counter("noumena_export_bytes_total", metric.WithUnit("By"))
}

func histCountWithAttr(rm *metricdata.ResourceMetrics, name, key, val string) (count uint64, sum int64) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[int64])
			if !ok {
				continue
			}
			for _, dp := range h.DataPoints {
				if key == "" {
					count += dp.Count
					sum += dp.Sum
					continue
				}
				v, ex := dp.Attributes.Value(attribute.Key(key))
				if ex && v.AsString() == val {
					count += dp.Count
					sum += dp.Sum
				}
			}
		}
	}
	return
}

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
				if key == "" {
					return dp.Value
				}
				v, ex := dp.Attributes.Value(attribute.Key(key))
				if ex && v.AsString() == val {
					return dp.Value
				}
			}
		}
	}
	return 0
}

func sumInt64Total(rm *metricdata.ResourceMetrics, name string) int64 {
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			if s, ok := m.Data.(metricdata.Sum[int64]); ok {
				for _, dp := range s.DataPoints {
					total += dp.Value
				}
			}
		}
	}
	return total
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("reader.Collect: %v", err)
	}
	return rm
}
