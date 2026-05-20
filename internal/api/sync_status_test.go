package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/db"
)

type stubSyncStatusReader struct {
	out *db.SyncStatus
	err error
}

func (s *stubSyncStatusReader) GetSyncStatus(_ context.Context, _, _ string) (*db.SyncStatus, error) {
	return s.out, s.err
}

func newSyncStatusRouter(h *api.ChannelHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		// Inject userID directly into context (skip auth middleware).
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
				next.ServeHTTP(w, req.WithContext(ctx))
			})
		})
		r.Get("/channels/{id}/sync-status", h.SyncStatus)
	})
	return r
}

func TestSyncStatus_NotConfigured(t *testing.T) {
	h := &api.ChannelHandler{}
	r := newSyncStatusRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/channels/pc1/sync-status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 got %d", w.Code)
	}
}

func TestSyncStatus_NotFound(t *testing.T) {
	h := &api.ChannelHandler{SyncStatusReader: &stubSyncStatusReader{err: db.ErrNotFound}}
	r := newSyncStatusRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/channels/pc1/sync-status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", w.Code)
	}
}

func TestSyncStatus_OK(t *testing.T) {
	last := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	next := last.Add(30 * time.Minute)
	pct := 42.5
	h := &api.ChannelHandler{SyncStatusReader: &stubSyncStatusReader{
		out: &db.SyncStatus{
			LastSyncedAt:            &last,
			NextSyncAt:              &next,
			BackfillProgressPercent: &pct,
			RecentError:             &db.SyncError{At: last, Message: "transient"},
		},
	}}
	r := newSyncStatusRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/channels/pc1/sync-status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["last_synced_at"] == nil {
		t.Fatalf("expected last_synced_at present")
	}
	if v, ok := body["backfill_progress_percent"].(float64); !ok || v != 42.5 {
		t.Fatalf("expected progress 42.5, got %v", body["backfill_progress_percent"])
	}
	re, ok := body["recent_error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected recent_error object, got %v", body["recent_error"])
	}
	if re["message"] != "transient" {
		t.Fatalf("expected message=transient, got %v", re["message"])
	}
}

func TestSyncStatus_InternalError(t *testing.T) {
	h := &api.ChannelHandler{SyncStatusReader: &stubSyncStatusReader{err: errors.New("boom")}}
	r := newSyncStatusRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/channels/pc1/sync-status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 got %d", w.Code)
	}
}
