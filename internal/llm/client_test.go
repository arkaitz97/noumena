package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_GenerateDigest(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"content": "{\"top_ideas\":[\"Idea 1\"],\"health_score\":95}"
				}
			}]
		}`))
	}))
	defer mockServer.Close()

	client := NewClient("test-key")
	client.baseURL = mockServer.URL

	res, err := client.GenerateDigest(context.Background(), "[]")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.HealthScore != 95 {
		t.Errorf("expected HealthScore 95, got %d", res.HealthScore)
	}
	if len(res.TopIdeas) != 1 {
		t.Errorf("expected 1 idea")
	}
}

func TestClient_doRequestErrors(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	client := NewClient("test-key")
	client.baseURL = mockServer.URL
	client.httpClient.Timeout = 1 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.GenerateDigest(ctx, "[]")
	if err == nil {
		t.Fatalf("expected error on 500 status")
	}
}
