package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// httpMeter is the package-level meter handle for HTTP server metrics.
// Resolved at package init: when telemetry.Init has not yet run, this
// returns the default global no-op MeterProvider's meter, so test
// binaries that pull in the api package don't need to bootstrap OTel.
// telemetry.Init swaps the global before any handler runs, and the
// instruments below resolve their final providers lazily at first use
// via the meter's caching layer.
var httpMeter = otel.Meter("noumena/http")

// httpTracerName is the tracer name used for HTTP server spans. The
// tracer is resolved per-request (not bound at init) so tests that swap
// the global TracerProvider after package init still get recording spans.
const httpTracerName = "noumena/http"

var (
	httpRequestsTotal   metric.Int64Counter
	httpRequestDuration metric.Float64Histogram
)

func init() {
	// Errors from the meter are unrecoverable here (would mean a
	// malformed instrument name). Panic at startup is acceptable
	// because nothing else in the package can run until init returns.
	var err error
	httpRequestsTotal, err = httpMeter.Int64Counter(
		"noumena_http_requests_total",
		metric.WithDescription("Total HTTP server requests handled by chi routes"),
	)
	if err != nil {
		panic("noumena_http_requests_total: " + err.Error())
	}
	httpRequestDuration, err = httpMeter.Float64Histogram(
		"noumena_http_request_duration_seconds",
		metric.WithDescription("HTTP server request duration"),
		metric.WithUnit("s"),
	)
	if err != nil {
		panic("noumena_http_request_duration_seconds: " + err.Error())
	}
}

// statusRecorder captures the HTTP status code written to the response
// so the otel middleware can attach it as a metric attribute. The chi
// router hands us a plain http.ResponseWriter, so we wrap it. WriteHeader
// is the only mutation point — Write() implies status 200 if WriteHeader
// was never called, which we handle in ServeHTTP after next returns.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.written {
		sr.status = code
		sr.written = true
	}
	sr.ResponseWriter.WriteHeader(code)
}

// Write triggers an implicit 200 if no header was written yet. We mirror
// net/http's behaviour rather than re-deriving it.
func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.written {
		sr.status = http.StatusOK
		sr.written = true
	}
	return sr.ResponseWriter.Write(b)
}

// routeFor resolves the chi route pattern after the handler ran; chi
// populates RouteContext during routing.
func routeFor(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
		return rc.RoutePattern()
	}
	return "unknown"
}

// OtelMiddleware records noumena_http_requests_total and
// noumena_http_request_duration_seconds for every chi-routed request,
// and emits one HTTP server span per request. The `route` attribute is
// the chi route pattern (e.g. `/api/channels/{id}`), never the raw URL.
//
// Query-string redaction (fix-crypto-http-hardening): the span's
// `http.target` attribute is set to the URL path only (never the query
// string), so `?token=...` from verify/reset links cannot reach OTLP.
// This middleware is the single source of HTTP spans — it replaces the
// upstream otelhttp.NewMiddleware mount which would otherwise record the
// raw request target. When telemetry is disabled the underlying
// instruments are no-ops and the tracer is non-recording, so the only
// overhead is the statusRecorder allocation.
func OtelMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sr := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(sr, r)
			elapsed := time.Since(start)
			elapsedSec := elapsed.Seconds()

			// Slow request logging (>1000ms threshold).
			if elapsed > 1000*time.Millisecond {
				log.Printf("[slow-request] %s %s took %s", r.Method, routeFor(r), elapsed)
			}

			route := routeFor(r)
			status := sr.status
			if status == 0 {
				// Handler returned without writing anything; this is
				// effectively a 200 per net/http semantics.
				status = http.StatusOK
			}

			// HTTP server span. Created after the handler so the route
			// pattern is available; StartTimestamp/EndTimestamp keep the
			// timing honest. http.target is the path only — no query.
			_, span := otel.Tracer(httpTracerName).Start(
				r.Context(),
				fmt.Sprintf("%s %s", r.Method, route),
				trace.WithTimestamp(start),
				trace.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.route", route),
					attribute.String("http.target", r.URL.Path),
					attribute.Int("http.status_code", status),
				),
			)
			if status >= 500 {
				span.SetStatus(codes.Error, "http status >= 500")
			}
			span.End(trace.WithTimestamp(time.Now()))

			attrs := metric.WithAttributes(
				attribute.String("method", r.Method),
				attribute.String("route", route),
				attribute.String("status", strconv.Itoa(status)),
			)
			httpRequestsTotal.Add(r.Context(), 1, attrs)
			httpRequestDuration.Record(r.Context(), elapsedSec, attrs)
		})
	}
}
