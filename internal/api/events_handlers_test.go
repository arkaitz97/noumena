package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
)

// newEventsRouter mirrors newDigestRouter: inject UserIDKey directly so
// the test exercises handler logic, not the auth middleware chain.
func newEventsRouter(h *api.EventHandler, userID string) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if userID != "" {
					ctx := context.WithValue(req.Context(), api.UserIDKey, userID)
					req = req.WithContext(ctx)
				}
				next.ServeHTTP(w, req)
			})
		})
		r.Post("/events", h.Track)
	})
	return r
}

func postEvent(t *testing.T, r *chi.Mux, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader(b))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestEvents_Track_HappyPath(t *testing.T) {
	r := newEventsRouter(&api.EventHandler{}, "user-1")
	for _, section := range []string{"ideas", "questions", "insights", "health"} {
		w := postEvent(t, r, map[string]any{
			"name":  "digest_section_toggled",
			"props": map[string]string{"section": section},
		})
		if w.Code != http.StatusNoContent {
			t.Fatalf("section %q: expected 204 got %d: %s", section, w.Code, w.Body.String())
		}
	}
}

func TestEvents_Track_Unauthorized(t *testing.T) {
	r := newEventsRouter(&api.EventHandler{}, "")
	w := postEvent(t, r, map[string]any{
		"name":  "digest_section_toggled",
		"props": map[string]string{"section": "ideas"},
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d: %s", w.Code, w.Body.String())
	}
}

func TestEvents_Track_UnknownEvent(t *testing.T) {
	r := newEventsRouter(&api.EventHandler{}, "user-1")
	w := postEvent(t, r, map[string]any{
		"name":  "arbitrary_exfil_event",
		"props": map[string]string{"section": "ideas"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body["error"] != "unknown_event" {
		t.Fatalf("expected unknown_event code, got %q", body["error"])
	}
}

func TestEvents_Track_UnknownSection(t *testing.T) {
	r := newEventsRouter(&api.EventHandler{}, "user-1")
	w := postEvent(t, r, map[string]any{
		"name":  "digest_section_toggled",
		"props": map[string]string{"section": "secrets"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d: %s", w.Code, w.Body.String())
	}
}

func TestEvents_Track_MissingSection(t *testing.T) {
	r := newEventsRouter(&api.EventHandler{}, "user-1")
	w := postEvent(t, r, map[string]any{"name": "digest_section_toggled"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d: %s", w.Code, w.Body.String())
	}
}

func TestEvents_Track_MalformedJSON(t *testing.T) {
	r := newEventsRouter(&api.EventHandler{}, "user-1")
	req := httptest.NewRequest(http.MethodPost, "/api/events", bytes.NewReader([]byte("{")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d: %s", w.Code, w.Body.String())
	}
}
