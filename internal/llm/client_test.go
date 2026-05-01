package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_ClassifyBatch(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"choices": [{
				"message": {
					"content": "{\"results\":[{\"category\":\"Question\",\"topic\":\"test setup\",\"sentiment_score\":0.5,\"confidence\":0.9}]}"
				}
			}]
		}`))
	}))
	defer mockServer.Close()

	client := NewClient("test-key")
	client.baseURL = mockServer.URL

	comments := []string{"How do I test this?"}
	results, err := client.ClassifyBatch(context.Background(), comments)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Category != "Question" {
		t.Errorf("expected category Question, got %s", results[0].Category)
	}
	if results[0].Topic != "test setup" {
		t.Errorf("expected topic 'test setup', got %s", results[0].Topic)
	}
}

func TestClient_GenerateDigest(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
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
	// mock 500 error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	client := NewClient("test-key")
	client.baseURL = mockServer.URL
	client.httpClient.Timeout = 1 * time.Second

	// we want to speed up backoff for this test
	ctx, cancel := context.WithTimeout(context.Background(), 100 * time.Millisecond)
	defer cancel()

	_, err := client.GenerateDigest(ctx, "[]")
	if err == nil {
		t.Fatalf("expected error on 500 status")
	}
}
