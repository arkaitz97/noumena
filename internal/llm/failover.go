package llm

import (
	"context"
	"log"
	"sync"
	"time"
)

// failoverClient wraps a primary and an optional fallback LLMClient. It owns
// an in-memory sliding window of error timestamps from the active provider
// and performs a one-way switch from primary to fallback once the threshold
// is breached. The switch is permanent for the lifetime of the process — no
// auto-recovery.
type failoverClient struct {
	primary  LLMClient
	fallback LLMClient // may be nil when ANTHROPIC_API_KEY is unset

	threshold int
	window    time.Duration

	mu        sync.Mutex
	errors    []time.Time
	usingFB   bool
	clock     func() time.Time
	logger    func(format string, args ...any)
}

// FailoverConfig captures the runtime knobs for the wrapper.
type FailoverConfig struct {
	Primary           LLMClient
	Fallback          LLMClient // nil disables failover
	ErrorThreshold    int       // default 5
	WindowSeconds     int       // default 60
}

// NewFailoverClient constructs the wrapper. Defaults are filled in when the
// caller passes zero values.
func NewFailoverClient(cfg FailoverConfig) LLMClient {
	if cfg.ErrorThreshold <= 0 {
		cfg.ErrorThreshold = 5
	}
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = 60
	}
	return &failoverClient{
		primary:   cfg.Primary,
		fallback:  cfg.Fallback,
		threshold: cfg.ErrorThreshold,
		window:    time.Duration(cfg.WindowSeconds) * time.Second,
		clock:     time.Now,
		logger:    log.Printf,
	}
}

func (f *failoverClient) ProviderName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.usingFB && f.fallback != nil {
		return f.fallback.ProviderName()
	}
	return f.primary.ProviderName()
}

func (f *failoverClient) ModelName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.usingFB && f.fallback != nil {
		return f.fallback.ModelName()
	}
	return f.primary.ModelName()
}

// active returns the LLMClient currently servicing requests under f.mu.
func (f *failoverClient) activeLocked() LLMClient {
	if f.usingFB && f.fallback != nil {
		return f.fallback
	}
	return f.primary
}

// ClassifyBatch routes the request to the active provider and on error
// records the failure timestamp. When the count of errors within the sliding
// window exceeds the threshold AND a fallback is configured AND we haven't
// already switched, the wrapper flips to the fallback and emits a structured
// `provider_switched` log line.
func (f *failoverClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error) {
	f.mu.Lock()
	active := f.activeLocked()
	f.mu.Unlock()

	results, err := active.ClassifyBatch(ctx, comments)
	if err == nil {
		return results, nil
	}

	// Record the error and possibly trigger a switch.
	f.recordError(err)
	return nil, err
}

// recordError appends to the sliding window and triggers the one-way switch
// when threshold is breached. No-ops on the fallback or when no fallback is
// configured.
func (f *failoverClient) recordError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Errors on the fallback do not trigger further switches.
	if f.usingFB {
		return
	}
	// No fallback configured: surface errors unchanged, never switch.
	if f.fallback == nil {
		return
	}

	now := f.clock()
	cutoff := now.Add(-f.window)
	// Drop aged-out timestamps.
	pruned := f.errors[:0]
	for _, t := range f.errors {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	pruned = append(pruned, now)
	f.errors = pruned

	if len(f.errors) > f.threshold {
		f.usingFB = true
		f.errors = nil
		// otel-instrumentation: record the switch BEFORE we drop
		// the lock so the metric ordering matches the log line.
		// Use a background ctx because the original request's ctx
		// might be cancelled (the call that pushed us over the
		// threshold may be the one that returns DeadlineExceeded).
		recordLLMFailoverSwitch(context.Background(),
			f.primary.ProviderName(), f.fallback.ProviderName())
		f.logger("provider_switched event=provider_switched from=%s to=%s threshold=%d window_seconds=%d last_error=%v",
			f.primary.ProviderName(), f.fallback.ProviderName(),
			f.threshold, int(f.window.Seconds()), err)
	}
}

// usingFallback exposes the current state for tests.
func (f *failoverClient) usingFallback() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.usingFB
}
