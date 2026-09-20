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

func (r *recordingClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, []int, TokenUsage, error) {
	r.calls++
	if r.err != nil {
		return nil, nil, TokenUsage{}, r.err
	}
	return nil, nil, TokenUsage{}, nil
}

func TestFailover_FlipsAfterConsecutiveThreshold(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "opencode/default", err: errors.New("boom")}
	fallback := &recordingClient{provider: "openai", model: "gpt-4o-mini"}

	fc := NewFailoverClient(primary, fallback, 3)

	for i := 0; i < 3; i++ {
		if _, _, _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err == nil {
			t.Fatalf("call %d: expected error from failing primary", i+1)
		}
	}
	if primary.calls != 3 {
		t.Fatalf("primary calls: got %d, want 3", primary.calls)
	}
	if fc.ProviderName() != "openai" || fc.ModelName() != "gpt-4o-mini" {
		t.Errorf("post-flip attribution: got %s/%s, want openai/gpt-4o-mini", fc.ProviderName(), fc.ModelName())
	}

	if _, _, _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err != nil {
		t.Fatalf("post-flip call: expected fallback success, got %v", err)
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls: got %d, want 1", fallback.calls)
	}
	if primary.calls != 3 {
		t.Fatalf("primary calls after flip: got %d, want 3 (no more primary traffic)", primary.calls)
	}
}

func TestFailover_FlipIsPermanent(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "m", err: errors.New("down")}
	fallback := &recordingClient{provider: "anthropic", model: "c", err: errors.New("also down")}

	fc := NewFailoverClient(primary, fallback, 3)
	for i := 0; i < 3; i++ {
		_, _, _, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}

	for i := 0; i < 20; i++ {
		if _, _, _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err == nil {
			t.Fatalf("call %d: expected fallback error to propagate", i+1)
		}
	}
	if primary.calls != 3 {
		t.Errorf("primary calls: got %d, want 3 (never returns to primary)", primary.calls)
	}
	if fallback.calls != 20 {
		t.Errorf("fallback calls: got %d, want 20", fallback.calls)
	}
}

// The old sliding window could never fill because errors arrive minutes apart.
// The consecutive counter makes timing irrelevant: failures separated by a long
// gap still accumulate and flip.
func TestFailover_SlowFailureCadenceStillFlips(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "m", err: errors.New("flaky")}
	fallback := &recordingClient{provider: "openai", model: "m2"}

	fc := NewFailoverClient(primary, fallback, 3)

	for i := 0; i < 3; i++ {
		if _, _, _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err == nil {
			t.Fatalf("call %d: expected error", i+1)
		}
		time.Sleep(2 * time.Millisecond) // errors minutes apart in production; timing irrelevant
	}
	if primary.calls != 3 {
		t.Errorf("primary calls: got %d, want 3", primary.calls)
	}
	if fc.ProviderName() != "openai" {
		t.Errorf("provider after slow-cadence failures: got %q, want openai (flipped)", fc.ProviderName())
	}
}

// A success between failures resets the consecutive counter, so interleaved
// successes prevent the flip until the threshold is reached consecutively.
func TestFailover_SuccessResetsCounter(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "m", err: errors.New("flaky")}
	fallback := &recordingClient{provider: "openai", model: "m2"}

	fc := NewFailoverClient(primary, fallback, 3)

	// 2 failures, a success, then 2 failures → counter never reaches 3.
	for i := 0; i < 2; i++ {
		_, _, _, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	primary.err = nil
	if _, _, _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err != nil {
		t.Fatalf("success call: %v", err)
	}
	primary.err = errors.New("flaky")
	for i := 0; i < 2; i++ {
		_, _, _, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	if fc.flipped {
		t.Errorf("flipped after non-consecutive failures; success must reset the counter")
	}

	// Three consecutive failures now flip.
	for i := 0; i < 3; i++ {
		_, _, _, _ = fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}})
	}
	if !fc.flipped {
		t.Errorf("3 consecutive errors must flip")
	}
}

func TestFailover_NilFallbackNeverFlipsAndPropagates(t *testing.T) {
	primary := &recordingClient{provider: "opencode", model: "m", err: errors.New("down")}

	fc := NewFailoverClient(primary, nil, 3)

	for i := 0; i < 10; i++ {
		if _, _, _, err := fc.ClassifyBatch(context.Background(), []PromptComment{{Body: "x"}}); err == nil {
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

	fc := NewFailoverClient(primary, fallback, 0)
	if fc.threshold != 3 {
		t.Errorf("threshold default: got %d, want 3", fc.threshold)
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
	if w.threshold != 3 {
		t.Errorf("wrapper default threshold: got %d, want 3", w.threshold)
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
