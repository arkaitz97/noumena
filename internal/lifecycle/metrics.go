package lifecycle

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"noumena/internal/telemetry"
)

// otel-instrumentation: lifecycle worker instruments. The deletion and
// export workers share a meter scope ("noumena/lifecycle") because the
// spec lists them in a single subsystem table; dashboards group them
// together.
var lifecycleMeter = otel.Meter("noumena/lifecycle")

var (
	deletionsCompletedTotal metric.Int64Counter
	deletionChunkRows       metric.Int64Histogram
	exportsCompletedTotal   metric.Int64Counter
	exportBytesTotal        metric.Int64Counter
)

func init() {
	var err error
	if deletionsCompletedTotal, err = lifecycleMeter.Int64Counter(
		"noumena_deletions_completed_total",
		metric.WithDescription("Account-deletion jobs reaching a terminal state"),
	); err != nil {
		panic("noumena_deletions_completed_total: " + err.Error())
	}
	if deletionChunkRows, err = lifecycleMeter.Int64Histogram(
		"noumena_deletion_chunk_rows",
		metric.WithDescription("Rows deleted per chunk by DeleteCommentsByChannelChunked"),
	); err != nil {
		panic("noumena_deletion_chunk_rows: " + err.Error())
	}
	if exportsCompletedTotal, err = lifecycleMeter.Int64Counter(
		"noumena_exports_completed_total",
		metric.WithDescription("GDPR-export jobs reaching a terminal state"),
	); err != nil {
		panic("noumena_exports_completed_total: " + err.Error())
	}
	if exportBytesTotal, err = lifecycleMeter.Int64Counter(
		"noumena_export_bytes_total",
		metric.WithDescription("Bytes written across all completed export ZIPs"),
		metric.WithUnit("By"),
	); err != nil {
		panic("noumena_export_bytes_total: " + err.Error())
	}
}

func recordDeletionCompleted(ctx context.Context, outcome string) {
	deletionsCompletedTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String(telemetry.AttrOutcome, outcome)))
}

func recordDeletionChunkRows(ctx context.Context, n int64) {
	if n < 0 {
		return
	}
	deletionChunkRows.Record(ctx, n)
}

func recordExportCompleted(ctx context.Context, outcome string) {
	exportsCompletedTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String(telemetry.AttrOutcome, outcome)))
}

func recordExportBytes(ctx context.Context, n int64) {
	if n <= 0 {
		return
	}
	exportBytesTotal.Add(ctx, n)
}
