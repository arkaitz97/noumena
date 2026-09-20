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
	results, _, _, err := c.ClassifyBatch(context.Background(), []PromptComment{
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
	results, _, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "hi"}})
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
	_, _, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
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
	_, _, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
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

func TestOpenAIClient_InvalidCategoryIsPartialNotRetryable(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, chatCompletionResponseBody(`[{"comment_index":0,"category":"not_a_real_category"}]`))
	}))
	defer srv.Close()

	c := newOpenAIClientWithHTTP(srv.Client(), srv.URL, "k", "")
	results, missing, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err != nil {
		t.Fatalf("partial parse must not abort: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("invalid category must be excluded, got %d valid", len(results))
	}
	if len(missing) != 1 || missing[0] != 0 {
		t.Errorf("missing indexes = %v, want [0]", missing)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls: got %d, want 1 (no retry on partial parse)", got)
	}
}

// TestOpenAIClient_Regression_SlowResponseUnderBudget verifies that a slow
// provider response completes when the caller ctx budget exceeds the response
// time, and that no hidden client timeout aborts it. Scaled to 500ms/5s for CI.
func TestOpenAIClient_Regression_SlowResponseUnderBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping under -short: wall-clock heavy")
	}
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, chatCompletionResponseBody(`[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"test","actionable_insights":[],"sentiment":"positive"}]`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := newOpenAIClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, _, _, err := c.ClassifyBatch(ctx, []PromptComment{{Body: "x"}})
	if err != nil {
		t.Fatalf("expected success with 5s budget for 500ms response: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls: got %d, want 1 (single attempt succeeds)", got)
	}
}

// TestOpenAIClient_Regression_BudgetAbortsAtOnce verifies that with a ctx
// budget shorter than the response time, the call is aborted after exactly
// one provider call (proving no hidden client timeout that would allow retries).
// Scaled to 500ms budget vs 2s response.
func TestOpenAIClient_Regression_BudgetAbortsAtOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping under -short: wall-clock heavy")
	}
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, chatCompletionResponseBody(`[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"test","actionable_insights":[],"sentiment":"positive"}]`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	c := newOpenAIClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, _, _, err := c.ClassifyBatch(ctx, []PromptComment{{Body: "x"}})
	if err == nil {
		t.Fatalf("expected deadline exceeded error with 500ms budget for 2s response")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("error should be deadline exceeded: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls: got %d, want 1 (aborted on first attempt, no hidden client timeout allowing retries)", got)
	}
}
