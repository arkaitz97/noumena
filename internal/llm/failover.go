package llm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"noumena/internal/config"
)

// failoverDefaultThreshold is the number of consecutive primary failures that
// flips to the fallback (design D5). Successes reset the counter, so the
// trigger is timing-independent — minutes-apart failures still accumulate.
const failoverDefaultThreshold = 3

// failoverClient is the provider-switching LLMClient. It routes every
// ClassifyBatch call to the primary until the primary has produced
// `threshold` CONSECUTIVE errors. A success resets the counter. The flip is
// PERMANENT: once the fallback is active the wrapper never routes back to the
// primary, and the fallback's own errors propagate without further state
// changes.
//
// Thread-safe: Processor drives it from a single goroutine today, but
// failover state is guarded by a mutex so the same client could be shared
// across workers safely.
type failoverClient struct {
	primary   LLMClient
	fallback  LLMClient // nil ⇒ never flip; primary errors propagate
	threshold int

	mu          sync.Mutex
	consecutive int
	flipped     bool
}

// NewFailoverClient wraps a primary and an optional fallback LLMClient.
// threshold <= 0 selects the default (3). A nil fallback yields a wrapper that
// never flips: per spec, the system then keeps retrying the primary and does
// not crash.
func NewFailoverClient(primary, fallback LLMClient, threshold int) *failoverClient {
	if threshold <= 0 {
		threshold = failoverDefaultThreshold
	}
	return &failoverClient{
		primary:   primary,
		fallback:  fallback,
		threshold: threshold,
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
// failover). Otherwise it returns a failoverClient using
// config.Default.LLMFailoverErrorThreshold (default 3). An unknown provider
// name returns an error; main decides whether that is fatal (processor) or a
// warning (digest worker keeps its dedicated opencode client either way).
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
	return NewFailoverClient(primary, fallback, config.Default.LLMFailoverErrorThreshold), nil
}

// providerFactories maps a normalized provider name to its client builder.
// New providers register here instead of extending a switch statement.
var providerFactories = map[string]func(apiKey string) LLMClient{
	"":          newConfigProviderClient,
	"opencode":  newConfigProviderClient,
	"openai":    func(k string) LLMClient { return NewOpenAIClient(k, "", "") },
	"anthropic": func(k string) LLMClient { return NewAnthropicClient(k, "", "") },
}

// newConfigProviderClient honors OPENCODE_BASE_URL/OPENCODE_MODEL from config
// (same values the legacy single-provider path passes) so failover wiring
// does not silently fall back to constructor defaults.
func newConfigProviderClient(apiKey string) LLMClient {
	return NewOpenCodeClient(apiKey, config.Default.OpenCodeBaseURL, config.Default.OpenCodeModel)
}

// newClientByProvider maps a provider name to a freshly built LLMClient.
// Keys are optional at construction: transport omits auth headers when the
// key is empty and the API responds 401 (non-retryable, surfaces as an
// error — never a crash).
func newClientByProvider(name, apiKey string) (LLMClient, error) {
	factory, ok := providerFactories[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, fmt.Errorf("unknown LLM provider %q (want opencode|openai|anthropic)", name)
	}
	return factory(apiKey), nil
}

// activeLocked returns the client current calls should hit, without holding
// the lock at call time so a slow upstream never stalls concurrent readers.
func (f *failoverClient) activeLocked() LLMClient {
	if f.flipped {
		return f.fallback
	}
	return f.primary
}

// ClassifyBatch routes to the active provider. Primary errors increment the
// consecutive-failure counter (successes reset it); once it reaches
// `threshold` (and a fallback exists) the wrapper flips permanently. Errors
// from the fallback after a flip, and errors with no fallback configured,
// propagate unchanged.
func (f *failoverClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, []int, TokenUsage, error) {
	f.mu.Lock()
	active := f.activeLocked()
	f.mu.Unlock()
	if active == nil {
		return nil, nil, TokenUsage{}, fmt.Errorf("failover: no active client (nil primary, no fallback)")
	}

	results, missing, usage, err := active.ClassifyBatch(ctx, comments)

	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		if !f.flipped {
			f.consecutive = 0
		}
		return results, missing, usage, nil
	}
	// Only the primary's failures feed the flip decision; post-flip fallback
	// errors and the nil-fallback case propagate without state change. Usage
	// is dropped: failed HTTP attempts report no billable token counts.
	if f.flipped || f.fallback == nil {
		return nil, nil, usage, err
	}
	f.consecutive++
	if f.consecutive < f.threshold {
		return nil, nil, usage, err
	}
	f.flipped = true
	f.consecutive = 0
	log.Printf("event=provider_failover from=%s to=%s consecutive_errors=%d — permanent for process lifetime",
		f.primary.ProviderName(), f.fallback.ProviderName(), f.threshold)
	recordLLMFailover(ctx, f.primary.ProviderName(), f.fallback.ProviderName())
	return nil, nil, usage, err
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
