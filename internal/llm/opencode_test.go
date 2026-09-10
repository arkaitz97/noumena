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

// validClassificationJSON returns a JSON array of `n` valid LLMResult entries
// suitable for placement inside an OpenAI-compatible chat completion message.
func validClassificationJSON(n int) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		// Use a benign category and zero sentiment to satisfy the parser.
		sb.WriteString(`{"comment_index":`)
		sb.WriteString(itoa(i))
		sb.WriteString(`,"category":"praise","confidence":0.95,"topic":"general","sentiment":0.5}`)
	}
	sb.WriteString("]")
	return sb.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// chatCompletionResponseBody returns a minimal OpenAI-compatible chat
// completion JSON envelope wrapping `content`.
func chatCompletionResponseBody(content string) string {
	body, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": content}},
		},
	})
	return string(body)
}

func TestOpenCodeClient_SuccessfulClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization: got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, chatCompletionResponseBody(validClassificationJSON(3)))
	}))
	defer srv.Close()

	c := newOpenCodeClientWithHTTP(srv.Client(), srv.URL, "test-key", "opencode/test")
	results, _, err := c.ClassifyBatch(context.Background(), []PromptComment{
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
	}
}

func TestOpenCodeClient_ProviderAndModelName(t *testing.T) {
	c := newOpenCodeClientWithHTTP(nil, "http://x", "k", "opencode/large")
	if c.ProviderName() != "opencode" {
		t.Errorf("ProviderName: got %q", c.ProviderName())
	}
	if c.ModelName() != "opencode/large" {
		t.Errorf("ModelName: got %q", c.ModelName())
	}
}

func TestOpenCodeClient_429RetryAfterHonored(t *testing.T) {
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

	c := newOpenCodeClientWithHTTP(srv.Client(), srv.URL, "k", "")
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

func TestOpenCodeClient_Persistent5xxReturnsError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// Use a client whose retries are not bounded by long backoff: rewire
	// using a tiny override is non-trivial without changing the package, so
	// we accept the wall-clock cost (≈7s) and run with -short skipping in
	// CI configurations that need it.
	if testing.Short() {
		t.Skip("skipping under -short: persistent retry test is wall-clock heavy")
	}

	c := newOpenCodeClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err == nil {
		t.Fatalf("expected error after exhausted retries")
	}
	if atomic.LoadInt32(&calls) != 4 { // 1 initial + 3 retries
		t.Errorf("calls: got %d, want 4", calls)
	}
}

func TestOpenCodeClient_400NonRetryable(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"bad model"}`)
	}))
	defer srv.Close()

	c := newOpenCodeClientWithHTTP(srv.Client(), srv.URL, "k", "")
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

func TestOpenCodeClient_BearerHeaderSent(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, chatCompletionResponseBody(validClassificationJSON(1)))
	}))
	defer srv.Close()

	c := newOpenCodeClientWithHTTP(srv.Client(), srv.URL, "sk-secret-123", "")
	if _, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "hi"}}); err != nil {
		t.Fatalf("ClassifyBatch: %v", err)
	}
	if seenAuth != "Bearer sk-secret-123" {
		t.Errorf("Authorization: got %q", seenAuth)
	}
}
