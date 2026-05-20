package ingestion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWebhookAlerter_PostsJSON(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected json content-type, got %s", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := NewWebhookAlerter(srv.URL)
	a.HTTP = srv.Client()
	a.Alert(context.Background(), "pc1", "j1", "boom")
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected 1 webhook hit, got %d", hits)
	}
}

func TestWebhookAlerter_EmptyURLFallsBackToLog(t *testing.T) {
	a := NewWebhookAlerter("")
	// Should not panic; log output not asserted here.
	a.Alert(context.Background(), "pc1", "j1", "boom")
}

func TestPickAlerter(t *testing.T) {
	if _, ok := PickAlerter("").(LogAlerter); !ok {
		t.Fatal("expected LogAlerter for empty URL")
	}
	if _, ok := PickAlerter("https://example.com/hook").(*WebhookAlerter); !ok {
		t.Fatal("expected WebhookAlerter for non-empty URL")
	}
}
