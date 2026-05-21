// channel_placeholders_test.go covers the 501 placeholder endpoints for
// TikTok and Instagram channel integrations (tasks 12.1-12.6).
//
// Coverage:
//   - GET  /api/channels/connect/tiktok     → 501
//   - POST /api/channels/exchange/tiktok    → 501
//   - GET  /api/channels/connect/instagram  → 501
//   - POST /api/channels/exchange/instagram → 501
//   - PlatformValidationMiddleware rejects unsupported platforms → 400
package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/db"
)

func newPlaceholderRouter(queries db.DBQuery) *chi.Mux {
	r := chi.NewRouter()
	h := &api.ChannelHandler{Queries: queries}
	r.Use(api.PlatformValidationMiddleware)
	r.Route("/api/channels", func(r chi.Router) {
		r.Get("/connect/tiktok", h.ConnectTikTok)
		r.Post("/exchange/tiktok", h.ExchangeTikTok)
		r.Get("/connect/instagram", h.ConnectInstagram)
		r.Post("/exchange/instagram", h.ExchangeInstagram)
	})
	return r
}

func TestConnectTikTok_Returns501(t *testing.T) {
	r := newPlaceholderRouter(&MockQuery{})
	req := httptest.NewRequest(http.MethodGet, "/api/channels/connect/tiktok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

func TestExchangeTikTok_Returns501(t *testing.T) {
	r := newPlaceholderRouter(&MockQuery{})
	req := httptest.NewRequest(http.MethodPost, "/api/channels/exchange/tiktok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

func TestConnectInstagram_Returns501(t *testing.T) {
	r := newPlaceholderRouter(&MockQuery{})
	req := httptest.NewRequest(http.MethodGet, "/api/channels/connect/instagram", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

func TestExchangeInstagram_Returns501(t *testing.T) {
	r := newPlaceholderRouter(&MockQuery{})
	req := httptest.NewRequest(http.MethodPost, "/api/channels/exchange/instagram", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

func TestPlatformValidationMiddleware_RejectsUnsupportedPlatform(t *testing.T) {
	r := newPlaceholderRouter(&MockQuery{})
	req := httptest.NewRequest(http.MethodGet, "/api/channels/connect/tiktok?platform=twitter", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported platform, got %d", w.Code)
	}
}

func TestPlatformValidationMiddleware_PassesSupportedPlatform(t *testing.T) {
	r := newPlaceholderRouter(&MockQuery{})
	req := httptest.NewRequest(http.MethodGet, "/api/channels/connect/tiktok?platform=youtube", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// youtube is supported, so middleware passes through to the handler which returns 501
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 from handler after middleware pass, got %d", w.Code)
	}
}

func TestPlatformValidationMiddleware_PassesWhenNoPlatformParam(t *testing.T) {
	r := newPlaceholderRouter(&MockQuery{})
	req := httptest.NewRequest(http.MethodGet, "/api/channels/connect/tiktok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// no platform param → middleware passes through
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 from handler when no platform param, got %d", w.Code)
	}
}
