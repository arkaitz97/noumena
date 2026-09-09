package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// openAIRequestBody decodes the JSON body an openaiClient request carries so
// tests can assert model/messages/temperature without string matching.
type openAIRequestBody struct {
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	Messages    []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func TestOpenAIClient_SuccessfulClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path: got %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test-123" {
			t.Errorf("Authorization: got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type: got %q", got)
		}
		var body openAIRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.Model != "gpt-test" {
			t.Errorf("model: got %q, want gpt-test", body.Model)
		}
		if body.Temperature != 0 {
			t.Errorf("temperature: got %v, want 0", body.Temperature)
		}
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[1].Role != "user" {
			t.Fatalf("messages: got %+v, want [system, user]", body.Messages)
		}
		if body.Messages[1].Content == "" {
			t.Error("user message content is empty (prompt builder not used)")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, chatCompletionResponseBody(validClassificationJSON(3)))
	}))
	defer srv.Close()

	c := newOpenAIClientWithHTTP(srv.Client(), srv.URL, "sk-test-123", "gpt-test")
	results, err := c.ClassifyBatch(context.Background(), []PromptComment{
		{Body: "great"}, {Body: "ok"}, {Body: "meh"},
	})
	if err != nil {
		t.Fatalf("ClassifyBatch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results len: got %d, want 3", len(results))
	}
	for i, r := range results {
		if r.Category != "praise" {
			t.Errorf("result[%d].Category: got %q", i, r.Category)
		}
		if r.CommentIndex != i {
			t.Errorf("result[%d].CommentIndex: got %d", i, r.CommentIndex)
		}
	}
}

func TestOpenAIClient_ProviderAndModelName(t *testing.T) {
	c := newOpenAIClientWithHTTP(nil, "http://x", "k", "gpt-custom")
	if c.ProviderName() != "openai" {
		t.Errorf("ProviderName: got %q", c.ProviderName())
	}
	if c.ModelName() != "gpt-custom" {
		t.Errorf("ModelName: got %q", c.ModelName())
	}
	if c.baseURL != "http://x" {
		t.Errorf("baseURL: got %q", c.baseURL)
	}

	def := newOpenAIClientWithHTTP(nil, "", "", "")
	if def.model != "gpt-4o-mini" {
		t.Errorf("default model: got %q, want gpt-4o-mini", def.model)
	}
	if def.baseURL != "https://api.openai.com/v1" {
		t.Errorf("default baseURL: got %q", def.baseURL)
	}
}

func TestOpenAIClient_429RetryAfterHonored(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, chatCompletionResponseBody(validClassificationJSON(1)))
	}))
	defer srv.Close()

	c := newOpenAIClientWithHTTP(srv.Client(), srv.URL, "k", "")
	start := time.Now()
	results, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "hi"}})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ClassifyBatch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len: got %d", len(results))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls: got %d, want 2", calls)
	}
	if elapsed < 800*time.Millisecond {
		t.Errorf("expected >=1s wait from Retry-After, got %v", elapsed)
	}
}

func TestOpenAIClient_Persistent5xxReturnsError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if testing.Short() {
		t.Skip("skipping under -short: persistent retry test is wall-clock heavy")
	}

	c := newOpenAIClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err == nil {
		t.Fatalf("expected error after exhausted retries")
	}
	if atomic.LoadInt32(&calls) != 4 { // 1 initial + 3 retries
		t.Errorf("calls: got %d, want 4", calls)
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("error should mention retry exhaustion: %v", err)
	}
}

func TestOpenAIClient_400NonRetryable(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"bad model"}`)
	}))
	defer srv.Close()

	c := newOpenAIClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err == nil {
		t.Fatalf("expected error on 400")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls: got %d, want 1 (no retry on 4xx)", got)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status 400: %v", err)
	}
}

func TestOpenAIClient_ParseErrorIsNonRetryable(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, chatCompletionResponseBody(`[{"comment_index":0,"category":"not_a_real_category"}]`))
	}))
	defer srv.Close()

	c := newOpenAIClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls: got %d, want 1 (no retry on parse failure)", got)
	}
	if !strings.Contains(err.Error(), "invalid category") {
		t.Errorf("error should mention the bad category: %v", err)
	}
}
