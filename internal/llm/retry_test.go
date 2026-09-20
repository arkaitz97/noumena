package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsTerminal_TerminalErrors(t *testing.T) {
	// HTTP 4xx (non-429) should be terminal
	err400 := terminal(&httpError{status: 400})
	if !IsTerminal(err400) {
		t.Errorf("HTTP 400 should be terminal")
	}

	err401 := terminal(&httpError{status: 401})
	if !IsTerminal(err401) {
		t.Errorf("HTTP 401 should be terminal")
	}

	err403 := terminal(&httpError{status: 403})
	if !IsTerminal(err403) {
		t.Errorf("HTTP 403 should be terminal")
	}

	// Parse/decode errors should be terminal
	parseErr := terminal(errors.New("parse: invalid category"))
	if !IsTerminal(parseErr) {
		t.Errorf("parse error should be terminal")
	}

	decodeErr := terminal(errors.New("decode response: invalid JSON"))
	if !IsTerminal(decodeErr) {
		t.Errorf("decode error should be terminal")
	}
}

func TestIsTerminal_TransientErrors(t *testing.T) {
	// HTTP 429 should NOT be terminal
	err429 := &httpError{status: 429}
	if IsTerminal(err429) {
		t.Errorf("HTTP 429 should NOT be terminal")
	}

	// HTTP 5xx should NOT be terminal
	err500 := &httpError{status: 500}
	if IsTerminal(err500) {
		t.Errorf("HTTP 500 should NOT be terminal")
	}

	err503 := &httpError{status: 503}
	if IsTerminal(err503) {
		t.Errorf("HTTP 503 should NOT be terminal")
	}

	// Transport errors should NOT be terminal
	transportErr := errors.New("transport: connection refused")
	if IsTerminal(transportErr) {
		t.Errorf("transport error should NOT be terminal")
	}

	// Context deadline should NOT be terminal
	deadlineErr := contextDeadlineExceeded{}
	if IsTerminal(deadlineErr) {
		t.Errorf("context deadline exceeded should NOT be terminal")
	}
}

// httpError simulates an HTTP error with status code for testing IsTerminal
type httpError struct{ status int }

func (e *httpError) Error() string { return "HTTP error" }

// contextDeadlineExceeded simulates context.DeadlineExceeded
type contextDeadlineExceeded struct{}

func (contextDeadlineExceeded) Error() string { return "context deadline exceeded" }

func TestClient_HTTP4xxIsTerminal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	c := newOpenCodeClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, _, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsTerminal(err) {
		t.Errorf("401 error should be terminal, got: %v", err)
	}
}

func TestClient_HTTP429IsTransient(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping under -short")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newOpenCodeClientWithHTTP(srv.Client(), srv.URL, "k", "")
	_, _, _, err := c.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err == nil {
		t.Fatal("expected error after retries")
	}
	if IsTerminal(err) {
		t.Errorf("429 error should NOT be terminal after retries exhausted, got: %v", err)
	}
}
