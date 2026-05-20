package ingestion

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"noumena/internal/telemetry"
)

// otel-instrumentation: package-level meter + lazy instruments. Resolved
// at package init against whatever MeterProvider is global at the time;
// telemetry.Init runs in main() before NewWorker is invoked, so the
// no-op vs SDK provider distinction propagates cleanly.
//
// Holding the instruments at package scope (not on the Worker struct)
// matches the spec scenario "Instrumentation is unconditional at call
// sites" — every package-level call short-circuits inside the meter
// when telemetry is disabled.
var ingestionMeter = otel.Meter("noumena/ingestion")

var (
	commentsIngestedTotal      metric.Int64Counter
	ingestionErrorsTotal       metric.Int64Counter
	ingestionLockContentionTot metric.Int64Counter
	fetchDurationSeconds       metric.Float64Histogram
)

func init() {
	var err error
	if commentsIngestedTotal, err = ingestionMeter.Int64Counter(
		"noumena_comments_ingested_total",
		metric.WithDescription("Comments persisted via InsertComments"),
	); err != nil {
		panic("noumena_comments_ingested_total: " + err.Error())
	}
	if ingestionErrorsTotal, err = ingestionMeter.Int64Counter(
		"noumena_ingestion_errors_total",
		metric.WithDescription("Ingestion errors classified by closed-enum reason"),
	); err != nil {
		panic("noumena_ingestion_errors_total: " + err.Error())
	}
	if ingestionLockContentionTot, err = ingestionMeter.Int64Counter(
		"noumena_ingestion_lock_contention_total",
		metric.WithDescription("Per-channel advisory-lock contention events"),
	); err != nil {
		panic("noumena_ingestion_lock_contention_total: " + err.Error())
	}
	if fetchDurationSeconds, err = ingestionMeter.Float64Histogram(
		"noumena_fetch_duration_seconds",
		metric.WithDescription("Wall-clock duration of a single ingestion job"),
		metric.WithUnit("s"),
	); err != nil {
		panic("noumena_fetch_duration_seconds: " + err.Error())
	}
}

// platformAttr is the canonical platform attribute. Today we only run
// against YouTube, but recording the attribute now keeps dashboards
// stable when TikTok / Twitch land — see telemetry.PlatformYouTube.
var platformAttr = attribute.String(telemetry.AttrPlatform, telemetry.PlatformYouTube)

// classifyIngestionError maps the worker's error taxonomy onto the
// closed reason enum from the spec. The default for an unknown error is
// "other" (NOT a panic) — losing label fidelity is preferable to
// crashing the ingestion worker on a novel error shape.
func classifyIngestionError(err error) string {
	if err == nil {
		return telemetry.OutcomeSuccess
	}
	var rate *RateLimitError
	if errors.As(err, &rate) {
		return telemetry.ReasonRateLimited
	}
	var tokInv *TokenInvalidError
	if errors.As(err, &tokInv) {
		return telemetry.ReasonTokenInvalid
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return telemetry.ReasonDeadlineExceeded
	}
	return telemetry.ReasonOther
}

// recordFetchDuration emits the noumena_fetch_duration_seconds
// histogram with the (platform, outcome) attributes. `outcome` is a
// closed enum of success / error / timeout — see spec.
func recordFetchDuration(ctx context.Context, start time.Time, outcome string) {
	fetchDurationSeconds.Record(ctx, time.Since(start).Seconds(),
		metric.WithAttributes(platformAttr, attribute.String(telemetry.AttrOutcome, outcome)))
}

// recordIngestionError increments noumena_ingestion_errors_total{reason=...}.
func recordIngestionError(ctx context.Context, reason string) {
	ingestionErrorsTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String(telemetry.AttrReason, reason)))
}

// recordCommentsIngested increments the counter by n.
func recordCommentsIngested(ctx context.Context, n int) {
	if n <= 0 {
		return
	}
	commentsIngestedTotal.Add(ctx, int64(n),
		metric.WithAttributes(platformAttr))
}

// recordLockContention is invoked when AcquireChannelLock returns
// acquired=false (another worker holds the per-channel lock).
func recordLockContention(ctx context.Context) {
	ingestionLockContentionTot.Add(ctx, 1,
		metric.WithAttributes(platformAttr))
}
