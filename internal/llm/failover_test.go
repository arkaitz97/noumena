package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type stubClient struct {
	name  string
	model string
	err   error
	calls int
}

func (s *stubClient) ProviderName() string { return s.name }
func (s *stubClient) ModelName() string    { return s.model }
func (s *stubClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([]LLMResult, len(comments))
	for i := range comments {
		out[i] = LLMResult{CommentIndex: i, Category: "praise", Confidence: 0.9, Topic: "ok", Sentiment: 0.5}
	}
	return out, nil
}

func newFailover(t *testing.T, primary, fallback LLMClient, threshold int) *failoverClient {
	t.Helper()
	c := NewFailoverClient(FailoverConfig{
		Primary:        primary,
		Fallback:       fallback,
		ErrorThreshold: threshold,
		WindowSeconds:  60,
	}).(*failoverClient)
	return c
}

func TestFailover_BelowThresholdNoSwitch(t *testing.T) {
	primary := &stubClient{name: "openai", model: "gpt-4o", err: errors.New("boom")}
	fallback := &stubClient{name: "anthropic", model: "claude"}
	f := newFailover(t, primary, fallback, 5)
	for i := 0; i < 4; i++ {
		_, _ = f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	if f.usingFallback() {
		t.Fatalf("should not have switched after 4 errors")
	}
}

func TestFailover_ThresholdBreachSwitches(t *testing.T) {
	primary := &stubClient{name: "openai", model: "gpt-4o", err: errors.New("boom")}
	fallback := &stubClient{name: "anthropic", model: "claude"}
	f := newFailover(t, primary, fallback, 5)
	for i := 0; i < 6; i++ {
		_, _ = f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	if !f.usingFallback() {
		t.Fatalf("expected switch after 6 errors with threshold 5")
	}
	// Subsequent calls go to the fallback (which succeeds).
	results, err := f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err != nil || len(results) != 1 {
		t.Fatalf("post-switch call should hit healthy fallback: err=%v results=%v", err, results)
	}
}

func TestFailover_ErrorsAgeOutOfWindow(t *testing.T) {
	primary := &stubClient{name: "openai", model: "gpt-4o", err: errors.New("boom")}
	fallback := &stubClient{name: "anthropic", model: "claude"}
	f := newFailover(t, primary, fallback, 5)
	// Inject a fake clock.
	base := time.Now()
	f.clock = func() time.Time { return base }
	for i := 0; i < 4; i++ {
		_, _ = f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	// Advance clock past the window so prior errors age out.
	f.clock = func() time.Time { return base.Add(2 * time.Minute) }
	for i := 0; i < 4; i++ {
		_, _ = f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	if f.usingFallback() {
		t.Fatalf("should not have switched: prior 4 errors aged out, only 4 in current window")
	}
}

func TestFailover_NoFallbackSurfacesErrors(t *testing.T) {
	primary := &stubClient{name: "openai", model: "gpt-4o", err: errors.New("boom")}
	f := newFailover(t, primary, nil, 5)
	for i := 0; i < 10; i++ {
		_, err := f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
		if err == nil {
			t.Fatalf("expected error to surface unchanged")
		}
	}
	if f.usingFallback() {
		t.Fatalf("must not switch when no fallback is configured")
	}
	if primary.calls != 10 {
		t.Errorf("expected all 10 calls to hit primary, got %d", primary.calls)
	}
}

// 8.7 — failover wrapper switches after 5 errors in 60s, no auto-recovery.
func TestFailover_NoAutoRecovery(t *testing.T) {
	primary := &stubClient{name: "openai", model: "gpt-4o", err: errors.New("boom")}
	fallback := &stubClient{name: "anthropic", model: "claude"}
	f := newFailover(t, primary, fallback, 5)
	// Trip the breaker.
	for i := 0; i < 6; i++ {
		_, _ = f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	if !f.usingFallback() {
		t.Fatalf("expected to switch")
	}
	// Even if primary "recovers" (we cannot test that directly), the wrapper
	// should not flip back. We assert the public flag stays true.
	primary.err = nil
	for i := 0; i < 5; i++ {
		_, _ = f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	if !f.usingFallback() {
		t.Fatal("auto-recovery to primary observed; spec requires one-way switch")
	}
}

// TestFailover_OpenCodePrimaryToAnthropicFallback validates the
// use-opencode-as-ai-provider change: the failover wrapper is
// provider-agnostic, so naming the primary "opencode" and the fallback
// "anthropic" must produce the same one-way switch behavior, and the
// `provider_switched` log line must include both names.
func TestFailover_OpenCodePrimaryToAnthropicFallback(t *testing.T) {
	primary := &stubClient{name: "opencode", model: "opencode/default", err: errors.New("boom")}
	fallback := &stubClient{name: "anthropic", model: "claude"}
	f := newFailover(t, primary, fallback, 5)

	var logged string
	f.logger = func(format string, args ...any) {
		logged = fmt.Sprintf(format, args...)
	}

	for i := 0; i < 6; i++ {
		_, _ = f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	if !f.usingFallback() {
		t.Fatalf("expected switch after 6 errors with threshold 5")
	}
	if !strings.Contains(logged, "from=opencode") || !strings.Contains(logged, "to=anthropic") {
		t.Errorf("provider_switched log should include from=opencode to=anthropic; got %q", logged)
	}

	// Post-switch calls go to the healthy Anthropic fallback.
	results, err := f.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if err != nil || len(results) != 1 {
		t.Fatalf("post-switch call should hit healthy fallback: err=%v results=%v", err, results)
	}
	if f.ProviderName() != "anthropic" {
		t.Errorf("ProviderName after switch: got %q, want anthropic", f.ProviderName())
	}
}
