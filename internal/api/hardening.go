package api

import (
	"log/slog"
	"net/http"
	"time"
)

// MaxBytesMiddleware caps every request body with http.MaxBytesReader so a
// client cannot stream unbounded payloads into memory or disk. The Stripe
// webhook keeps its own stricter cap (see WebhookStripe); nested readers
// compose safely because both are enforced at read time.
func MaxBytesMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requestHTTPS reports whether the request arrived over TLS, including
// when TLS is terminated at a reverse proxy that sets X-Forwarded-Proto.
func requestHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}

// SecurityHeadersMiddleware emits the hardening response headers:
// HSTS (only when the request is HTTPS), nosniff, frame-deny,
// no-referrer, and a configurable Content-Security-Policy.
func SecurityHeadersMiddleware(csp string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			if requestHTTPS(r) {
				h.Set("Strict-Transport-Security", "max-age=15552000; includeSubDomains")
			}
			if csp != "" {
				h.Set("Content-Security-Policy", csp)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AccessLogMiddleware logs one line per request with the method, the
// URL path only (never the query string, so ?token= can't leak), and the
// duration. Replaces chi's middleware.Logger which logs the full request
// URI including query parameters.
func AccessLogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"duration", time.Since(start).String(),
			)
		})
	}
}
