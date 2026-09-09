package llm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"noumena/internal/config"
)

// Defaults for the failover decision (openspec design.md decision 6, spec
// scenario "Primary provider fails"): N = 5 errors within a 60s sliding
// window triggers a permanent switch to the fallback for the process
// lifetime. With no fallback configured the wrapper never flips and the
// primary's errors keep propagating to the caller's own retry handling.
const (
	failoverDefaultWindow    = 60 * time.Second
	failoverDefaultThreshold = 5
)

// failoverClient is the provider-switching LLMClient. It routes every
// ClassifyBatch call to the primary until the primary has produced
// `threshold` errors within the trailing `window` (a sliding window: each
// recorded error ages out `window` after it happened; successes do not
// reset it). The flip is PERMANENT: once the fallback is active the wrapper
// never routes back to the primary, and the fallback's own errors propagate
// without triggering further state changes.
//
// Thread-safe: Processor drives it from a single goroutine today, but
// failover state (failure timestamps, flip) is guarded by a mutex so the
// same client could be shared across workers safely.
type failoverClient struct {
	primary   LLMClient
	fallback  LLMClient // nil ⇒ never flip; primary errors propagate
	window    time.Duration
	threshold int

	now func() time.Time // clock, injectable for tests

	mu       sync.Mutex
	failures []time.Time // timestamps of primary errors still inside window
	flipped  bool
}

// NewFailoverClient wraps a primary and an optional fallback LLMClient.
// window <= 0 and threshold <= 0 select the defaults (60s, 5). A nil
// fallback yields a wrapper that never flips: per spec, the system then
// keeps retrying the primary and does not crash.
func NewFailoverClient(primary, fallback LLMClient, window time.Duration, threshold int) *failoverClient {
	if window <= 0 {
		window = failoverDefaultWindow
	}
	if threshold <= 0 {
		threshold = failoverDefaultThreshold
	}
	return &failoverClient{
		primary:   primary,
		fallback:  fallback,
		window:    window,
		threshold: threshold,
		now:       time.Now,
	}
}

// NewConfiguredClient is the main.go convenience constructor. Provider names
// are exactly "opencode", "openai", "anthropic" (empty means "opencode").
// primaryKey/fallbackKey are the provider API keys already resolved by the
// caller: OPENCODE_API_KEY for opencode, OPENAI_API_KEY for openai,
// ANTHROPIC_API_KEY for anthropic — read via os.Getenv in main, never here.
// Env that selects the names: LLM_PRIMARY (→ config.Default.LLMPrimaryProvider)
// and LLM_FALLBACK (→ config.Default.LLMFallbackProvider); defaults and
// models apply per constructor.
//
// With fallbackName empty it returns the primary client unwrapped (no
// failover). Otherwise it returns a failoverClient with the default 5-in-60s
// window. An unknown provider name returns an error; main decides whether
// that is fatal (processor) or a warning (digest worker keeps its dedicated
// opencode client either way).
func NewConfiguredClient(primaryName, fallbackName, primaryKey, fallbackKey string) (LLMClient, error) {
	primary, err := newClientByProvider(primaryName, primaryKey)
	if err != nil {
		return nil, err
	}
	if fallbackName == "" {
		return primary, nil
	}
	fallback, err := newClientByProvider(fallbackName, fallbackKey)
	if err != nil {
		return nil, err
	}
	if fallbackKey == "" {
		log.Println("WARN: LLM fallback provider configured without an API key; failover will switch to an unauthenticated client")
	}
	return NewFailoverClient(primary, fallback, 0, 0), nil
}

// newClientByProvider maps a provider name to a freshly built LLMClient.
// Keys are optional at construction: transport omits auth headers when the
// key is empty and the API responds 401 (non-retryable, surfaces as an
// error — never a crash).
func newClientByProvider(name, apiKey string) (LLMClient, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "opencode":
		// Honors OPENCODE_BASE_URL/OPENCODE_MODEL from config (same values the
		// legacy single-provider path passes) so failover wiring does not
		// silently fall back to constructor defaults.
		return NewOpenCodeClient(apiKey, config.Default.OpenCodeBaseURL, config.Default.OpenCodeModel), nil
	case "openai":
		return NewOpenAIClient(apiKey, "", ""), nil
	case "anthropic":
		return NewAnthropicClient(apiKey, "", ""), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q (want opencode|openai|anthropic)", name)
	}
}

// active returns the client current calls should hit, without holding the
// lock at call time so a slow upstream never stalls concurrent readers.
func (f *failoverClient) activeLocked() LLMClient {
	if f.flipped {
		return f.fallback
	}
	return f.primary
}

// pruneLocked drops failure timestamps that have fallen out of the window.
func (f *failoverClient) pruneLocked(now time.Time) {
	cutoff := now.Add(-f.window)
	kept := f.failures[:0]
	for _, ts := range f.failures {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	f.failures = kept
}

// ClassifyBatch routes to the active provider. Primary errors are recorded
// against the sliding window; once `threshold` errors sit inside the window
// (and a fallback exists) the wrapper flips permanently. Errors from the
// fallback after a flip, and errors with no fallback configured, propagate
// unchanged.
func (f *failoverClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error) {
	f.mu.Lock()
	f.pruneLocked(f.now())
	active := f.activeLocked()
	f.mu.Unlock()
	if active == nil {
		return nil, fmt.Errorf("failover: no active client (nil primary, no fallback)")
	}

	results, err := active.ClassifyBatch(ctx, comments)
	if err == nil {
		return results, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	// Only the primary's failures feed the flip decision; post-flip fallback
	// errors and the nil-fallback case propagate without state change.
	if f.flipped || f.fallback == nil {
		return nil, err
	}
	now := f.now()
	f.failures = append(f.failures, now)
	f.pruneLocked(now)
	if len(f.failures) < f.threshold {
		return nil, err
	}
	f.flipped = true
	f.failures = nil
	log.Printf("event=provider_failover from=%s to=%s errors=%d window=%s — permanent for process lifetime",
		f.primary.ProviderName(), f.fallback.ProviderName(), f.threshold, f.window)
	recordLLMFailover(ctx, f.primary.ProviderName(), f.fallback.ProviderName())
	return nil, err
}

// ProviderName reports the active provider (pre-flip primary, post-flip
// fallback) for attribution in comments.llm_provider.
func (f *failoverClient) ProviderName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activeLocked().ProviderName()
}

// ModelName reports the active provider's model for persistence.
func (f *failoverClient) ModelName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activeLocked().ModelName()
}
