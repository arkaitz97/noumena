package api

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// EventHandler — POST /api/events. UI telemetry sink for product
// questions that need real usage data (first consumer: weekly-digest
// section open-rate behind the sidebar widget-order decision).
//
// Deliberately minimal: validate against a closed allowlist, bump an
// OTel counter, return 204. Events are NOT persisted — counts with
// closed-set attributes answer the open-rate question; anything that
// would need per-event storage starts a design conversation first.
// Auth + per-user rate limiting come from the middleware chain
// (cmd/api/main.go); this handler can assume UserIDKey is set.
type EventHandler struct{}

// uiEventRequest is the wire format: {name, props}. Props are free-form
// strings on the wire but filtered through the per-event enum below
// before they reach the metrics backend — the cardinality firewall
// pattern from internal/telemetry/attrs.go.
type uiEventRequest struct {
	Name  string            `json:"name"`
	Props map[string]string `json:"props"`
}

// eventSections is the allowlist: event name → valid values of its one
// tracked prop, "section". Unknown event or unknown section → 400, no
// metric. Adding an event is a one-line change here plus the client
// call site.
var eventSections = map[string]map[string]struct{}{
	"digest_section_toggled": {
		"ideas":     {},
		"questions": {},
		"insights":  {},
		"health":    {},
	},
}

var uiMeter = otel.Meter("noumena/ui")

var uiEventsTotal metric.Int64Counter

func init() {
	var err error
	uiEventsTotal, err = uiMeter.Int64Counter(
		"noumena_ui_events_total",
		metric.WithDescription("UI events accepted by /api/events, by event and section"),
	)
	if err != nil {
		panic("noumena_ui_events_total: " + err.Error())
	}
}

// Track handles POST /api/events.
func (h *EventHandler) Track(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req uiEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_event")
		return
	}
	sections, known := eventSections[req.Name]
	if !known {
		writeJSONError(w, http.StatusBadRequest, "unknown_event")
		return
	}
	section := req.Props["section"]
	if _, valid := sections[section]; !valid {
		writeJSONError(w, http.StatusBadRequest, "unknown_section")
		return
	}

	uiEventsTotal.Add(r.Context(), 1, metric.WithAttributes(
		attribute.String("event", req.Name),
		attribute.String("section", section),
	))
	w.WriteHeader(http.StatusNoContent)
}
