package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// MetricsHandler serves Prometheus-format metrics.
// When OTel is disabled, returns a minimal set of manually tracked metrics.
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "# HELP noumena_up Service is up\n# TYPE noumena_up gauge\nnoumena_up 1\n")
}

// RedactEmail masks an email address for logging: first char + "***" + domain.
func RedactEmail(email string) string {
	if email == "" {
		return ""
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	if len(parts[0]) == 0 {
		return "***@" + parts[1]
	}
	return string(parts[0][0]) + "***@" + parts[1]
}

// NewStructuredLogger returns a slog.Logger that redacts email addresses in all output.
func NewStructuredLogger(level slog.Level) *slog.Logger {
	handler := slog.NewTextHandler(nil, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindString {
				s := a.Value.String()
				if strings.Contains(s, "@") && strings.Contains(s, ".") {
					a.Value = slog.StringValue(RedactEmail(s))
				}
			}
			return a
		},
	})
	return slog.New(handler)
}

// SlowRequestMiddleware logs requests that exceed the given threshold.
func SlowRequestMiddleware(threshold time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			elapsed := time.Since(start)
			if elapsed > threshold {
				slog.Warn("slow request",
					"method", r.Method,
					"path", r.URL.Path,
					"duration", elapsed.String(),
				)
			}
		})
	}
}
