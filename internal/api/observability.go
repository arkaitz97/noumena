package api

import (
	"fmt"
	"net/http"
)

// MetricsHandler serves Prometheus-format metrics.
// When OTel is disabled, returns a minimal set of manually tracked metrics.
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "# HELP noumena_up Service is up\n# TYPE noumena_up gauge\nnoumena_up 1\n")
}
