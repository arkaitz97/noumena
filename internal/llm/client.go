package llm

import (
	"context"
)

// TokenUsage carries the provider-reported token counts for one batch
// call (llm-cost-analytics 1.3). Zero values mean the provider omitted a
// usage object — cost derivation treats those as unbilled, never guessed.
type TokenUsage struct {
	Input  int64
	Output int64
}

// LLMClient is the narrow interface every classification provider satisfies.
// Implementations concern themselves only with request building, transport
// (including 429/Retry-After backoff), response parsing, and returning either
// a complete []LLMResult of the same length as `comments` or an error.
//
// The failover wrapper composes two LLMClients (primary + optional fallback)
// behind the same interface and is the only place where provider-switching
// policy lives.
type LLMClient interface {
	ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, []int, TokenUsage, error)
	// ProviderName returns a stable identifier (e.g. "openai", "anthropic")
	// used for attribution in comments.llm_provider.
	ProviderName() string
	// ModelName returns the configured model identifier persisted alongside
	// the classification (e.g. "gpt-4o", "claude-3-5-sonnet").
	ModelName() string
}

// classifyOutcome carries a batch's partial classification: valid results
// plus the indexes the model omitted or returned invalid. It is the generic
// payload retryTransient and httpClassifyOnce shuttle through their loops so
// partial success survives in-job retries.
type classifyOutcome struct {
	results        []LLMResult
	missingIndexes []int
	usage          TokenUsage
}
