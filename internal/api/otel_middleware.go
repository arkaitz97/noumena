package api

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// httpMeter is the package-level meter handle for HTTP server metrics.
// Resolved at package init: when telemetry.Init has not yet run, this
// returns the default global no-op MeterProvider's meter, so test
// binaries that pull in the api package don't need to bootstrap OTel.
// telemetry.Init swaps the global before any handler runs, and the
// instruments below resolve their final providers lazily at first use
// via the meter's caching layer.
var httpMeter = otel.Meter("noumena/http")

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

// OtelMiddleware records noumena_http_requests_total and
// noumena_http_request_duration_seconds for every chi-routed request.
// The `route` attribute is the chi route pattern (e.g.
// `/api/channels/{id}`), never the raw URL — see spec
// "HTTP server instruments and spans". When telemetry is disabled the
// underlying instruments are no-ops, so the only overhead is the
// time.Since() call and the statusRecorder allocation.
//
// Spans (when traces are enabled) are NOT started here because the
// recommended `otelhttp.NewMiddleware` integration runs BEFORE chi has
// resolved the route, which makes the chi route pattern unavailable to
// the span. Operators who want HTTP server spans can mount
// `otelhttp.NewMiddleware("noumena-api")` themselves; this middleware
// focuses on the metric contract documented in the spec.
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
				route := "unknown"
				if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
					route = rc.RoutePattern()
				}
				log.Printf("[slow-request] %s %s took %s", r.Method, route, elapsed)
			}

			// Resolve the chi route pattern AFTER the handler ran;
			// chi populates RouteContext during routing.
			route := "unknown"
			if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
				route = rc.RoutePattern()
			}
			status := sr.status
			if status == 0 {
				// Handler returned without writing anything; this is
				// effectively a 200 per net/http semantics.
				status = http.StatusOK
			}
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
