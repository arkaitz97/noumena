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

// anthropicResponseBody returns a minimal Messages API success envelope: a
// content block list whose first text block carries `content`.
func anthropicResponseBody(content string) string {
	body, _ := json.Marshal(map[string]any{
		"content": []map[string]any{
			{"type": "thinking", "thinking": "not used"},
			{"type": "text", "text": content},
		},
		"stop_reason": "end_turn",
	})
	return string(body)
}

// anthropicRequestBody decodes the JSON body an anthropicClient request
// carries so tests can assert model/max_tokens/system/messages.
type anthropicRequestBody struct {
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	System      string  `json:"system"`
	Messages    []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func TestAnthropicClient_SuccessfulClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path: got %q, want /v1/messages", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "sk-ant-test" {
			t.Errorf("x-api-key: got %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version: got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type: got %q", got)
		}
		var body anthropicRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.Model != "claude-test" {
			t.Errorf("model: got %q, want claude-test", body.Model)
		}
		if body.MaxTokens != 4096 {
			t.Errorf("max_tokens: got %d, want 4096", body.MaxTokens)
		}
		if body.Temperature != 0 {
			t.Errorf("temperature: got %v, want 0", body.Temperature)
		}
		if body.System == "" {
			t.Error("system prompt is empty (prompt builder not used)")
		}
		if len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content == "" {
			t.Fatalf("messages: got %+v, want one non-empty user message", body.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, anthropicResponseBody(validClassificationJSON(2)))
	}))
	defer srv.Close()

	c := newAnthropicClientWithHTTP(srv.Client(), srv.URL, "sk-ant-test", "claude-test")
	results, _, err := c.ClassifyBatch(context.Background(), []PromptComment{
		{Body: "great"}, {Body: "ok"},
	})
	if err != nil {
		t.Fatalf("ClassifyBatch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len: got %d, want 2", len(results))
	}
	for i, r := range results {
		if r.Category != "praise" {
			t.Errorf("result[%d].Category: got %q", i, r.Category)
		}
	}
}

func TestAnthropicClient_ProviderAndModelName(t *testing.T) {
	c := newAnthropicClientWithHTTP(nil, "http://x", "k", "claude-x")
	if c.ProviderName() != "anthropic" {
		t.Errorf("ProviderName: got %q", c.ProviderName())
	}
	if c.ModelName() != "claude-x" {
		t.Errorf("ModelName: got %q", c.ModelName())
	}
	if c.baseURL != "http://x" {
		t.Errorf("baseURL: got %q", c.baseURL)
	}

	def := newAnthropicClientWithHTTP(nil, "", "", "")
	if def.baseURL != "https://api.anthropic.com" {
		t.Errorf("default baseURL: got %q", def.baseURL)
	}
	if def.model != "claude-sonnet-4-20250514" {
		t.Errorf("default model: got %q", def.model)
	}
	if def.maxTokens != 4096 {
		t.Errorf("default max_tokens: got %d", def.maxTokens)
	}
}

func TestAnthropicClient_429RetryAfterHonored(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "sk-ant-test" {
			t.Errorf("x-api-key on retry: got %q", got)
		}
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, anthropicResponseBody(validClassificationJSON(1)))
	}))
	defer srv.Close()

	c := newAnthropicClientWithHTTP(srv.Client(), srv.URL, "sk-ant-test", "")
	start := time.Now()
	results, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "hi"}})
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

func TestAnthropicClient_Persistent5xxReturnsError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if testing.Short() {
		t.Skip("skipping under -short: persistent retry test is wall-clock heavy")
	}

	c := newAnthropicClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err == nil {
		t.Fatalf("expected error after exhausted retries")
	}
	if atomic.LoadInt32(&calls) != 4 { // 1 initial + 3 retries
		t.Errorf("calls: got %d, want 4", calls)
	}
}

func TestAnthropicClient_400NonRetryable(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"message":"bad"}}`)
	}))
	defer srv.Close()

	c := newAnthropicClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
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
