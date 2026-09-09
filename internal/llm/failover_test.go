package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// recordingClient is a deterministic fake LLMClient for failover tests.
// When err is set every ClassifyBatch call fails with it; otherwise calls
// succeed with an empty result set.
type recordingClient struct {
	provider string
	model    string
	calls    int
	err      error
}

func (r *recordingClient) ProviderName() string { return r.provider }
func (r *recordingClient) ModelName() string    { return r.model }

func (r *recordingClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return nil, nil
}

func TestFailover_FlipsToFallbackAfterThreshold(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "opencode/default", err: errors.New("boom")}
	fallback := &recordingClient{provider: "openai", model: "gpt-4o-mini"}

	fc := NewFailoverClient(primary, fallback, 60*time.Second, 5)

	for i := 0; i < 5; i++ {
		if _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err == nil {
			t.Fatalf("call %d: expected error from failing primary", i+1)
		}
	}
	if primary.calls != 5 {
		t.Fatalf("primary calls: got %d, want 5", primary.calls)
	}
	if fc.ProviderName() != "openai" || fc.ModelName() != "gpt-4o-mini" {
		t.Errorf("post-flip attribution: got %s/%s, want openai/gpt-4o-mini", fc.ProviderName(), fc.ModelName())
	}

	if _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err != nil {
		t.Fatalf("post-flip call: expected fallback success, got %v", err)
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls: got %d, want 1", fallback.calls)
	}
	if primary.calls != 5 {
		t.Fatalf("primary calls after flip: got %d, want 5 (no more primary traffic)", primary.calls)
	}
}

func TestFailover_FlipIsPermanent(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "m", err: errors.New("down")}
	fallback := &recordingClient{provider: "anthropic", model: "c", err: errors.New("also down")}

	fc := NewFailoverClient(primary, fallback, 60*time.Second, 5)
	for i := 0; i < 5; i++ {
		_, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}

	for i := 0; i < 20; i++ {
		if _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err == nil {
			t.Fatalf("call %d: expected fallback error to propagate", i+1)
		}
	}
	if primary.calls != 5 {
		t.Errorf("primary calls: got %d, want 5 (never returns to primary)", primary.calls)
	}
	if fallback.calls != 20 {
		t.Errorf("fallback calls: got %d, want 20", fallback.calls)
	}
}

func TestFailover_ErrorsSpreadBeyondWindowDoNotFlip(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "m", err: errors.New("flaky")}
	fallback := &recordingClient{provider: "openai", model: "m2"}

	fc := NewFailoverClient(primary, fallback, 60*time.Second, 5)
	now := time.Unix(1_000_000, 0)
	fc.now = func() time.Time { return now }

	// One error every 61s: each ages out before the next arrives, so the
	// window never holds >= 5 errors and the wrapper must never flip.
	for i := 0; i < 10; i++ {
		if _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err == nil {
			t.Fatalf("call %d: expected error", i+1)
		}
		now = now.Add(61 * time.Second)
	}
	if primary.calls != 10 {
		t.Errorf("primary calls: got %d, want 10", primary.calls)
	}
	if fallback.calls != 0 {
		t.Errorf("fallback calls: got %d, want 0 (no flip)", fallback.calls)
	}
	if fc.ProviderName() != "opencode" {
		t.Errorf("provider after spread errors: got %q, want opencode", fc.ProviderName())
	}
}

func TestFailover_NilFallbackNeverFlipsAndPropagates(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "m", err: errors.New("down")}

	fc := NewFailoverClient(primary, nil, 60*time.Second, 5)

	for i := 0; i < 10; i++ {
		if _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err == nil {
			t.Fatalf("call %d: expected error to propagate (no fallback configured)", i+1)
		}
	}
	if primary.calls != 10 {
		t.Errorf("primary calls: got %d, want 10 (keeps retrying primary)", primary.calls)
	}
	if fc.ProviderName() != "opencode" {
		t.Errorf("provider: got %q, want opencode (never flipped)", fc.ProviderName())
	}
}

func TestFailover_ConstructorDefaults(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "m", err: errors.New("x")}
	fallback := &recordingClient{provider: "openai", model: "m2"}

	fc := NewFailoverClient(primary, fallback, 0, 0)
	if fc.window != 60*time.Second {
		t.Errorf("window default: got %s, want 60s", fc.window)
	}
	if fc.threshold != 5 {
		t.Errorf("threshold default: got %d, want 5", fc.threshold)
	}

	// 4 errors then a >60s quiet gap then 1 more error: the 4th error has
	// aged out, so the default window must still prevent a flip.
	now := time.Unix(1_000_000, 0)
	fc.now = func() time.Time { return now }
	for i := 0; i < 4; i++ {
		_, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	now = now.Add(61 * time.Second)
	_, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	if fc.flipped {
		t.Errorf("flipped with only 1 error in the default window; want no flip")
	}

	// Five errors back-to-back inside the window still flip.
	now = now.Add(61 * time.Second)
	for i := 0; i < 5; i++ {
		_, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	if !fc.flipped {
		t.Errorf("5 errors inside the default window: want flip")
	}
	if fc.ProviderName() != "openai" {
		t.Errorf("provider after flip: got %q, want openai", fc.ProviderName())
	}
}

func TestFailover_SuccessesDoNotClearWindow(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "m"}
	fallback := &recordingClient{provider: "openai", model: "m2"}

	fc := NewFailoverClient(primary, fallback, 60*time.Second, 3)

	// Two failures, four successes in between (successes must NOT reset the
	// window), then a third failure inside the same 60s → flip.
	primary.err = errors.New("flaky")
	_, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	_, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	primary.err = nil
	for i := 0; i < 4; i++ {
		if _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err != nil {
			t.Fatalf("success call %d: %v", i+1, err)
		}
	}
	primary.err = errors.New("flaky")
	_, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})

	if !fc.flipped {
		t.Errorf("3 errors within window despite interleaved successes: want flip")
	}
	if primary.calls != 7 || fallback.calls != 0 {
		t.Errorf("calls: primary=%d fallback=%d, want 7/0 pre-flip", primary.calls, fallback.calls)
	}
}

func TestNewConfiguredClient(t *testing.T) {
	if _, err := NewConfiguredClient("klingon", "", "k", ""); err == nil {
		t.Fatal("expected error for unknown provider name")
	} else if !strings.Contains(err.Error(), "klingon") {
		t.Fatalf("error should name the provider: %v", err)
	}

	// No fallback → returns the primary client unwrapped, not a wrapper.
	cli, err := NewConfiguredClient("openai", "", "sk-test", "")
	if err != nil {
		t.Fatalf("NewConfiguredClient: %v", err)
	}
	if cli.ProviderName() != "openai" {
		t.Fatalf("provider: got %q, want openai", cli.ProviderName())
	}
	if _, ok := cli.(*failoverClient); ok {
		t.Fatal("no fallback configured: expected unwrapped primary client")
	}

	// Empty primary name defaults to opencode; fallback wraps.
	fc, err := NewConfiguredClient("", "openai", "oc-key", "sk-test")
	if err != nil {
		t.Fatalf("NewConfiguredClient: %v", err)
	}
	w, ok := fc.(*failoverClient)
	if !ok {
		t.Fatalf("fallback configured: expected *failoverClient, got %T", fc)
	}
	if w.primary.ProviderName() != "opencode" || w.fallback.ProviderName() != "openai" {
		t.Errorf("wrapped providers: primary=%s fallback=%s", w.primary.ProviderName(), w.fallback.ProviderName())
	}
	if w.threshold != 5 || w.window != 60*time.Second {
		t.Errorf("wrapper defaults: threshold=%d window=%s", w.threshold, w.window)
	}

	// anthropic is a legal explicit name.
	ac, err := NewConfiguredClient("anthropic", "", "sk-ant", "")
	if err != nil {
		t.Fatalf("anthropic primary: %v", err)
	}
	if ac.ProviderName() != "anthropic" {
		t.Errorf("provider: got %q, want anthropic", ac.ProviderName())
	}
}
