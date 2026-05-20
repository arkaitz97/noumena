package llm

import (
	"context"
)

// LLMClient is the narrow interface every classification provider satisfies.
// Implementations concern themselves only with request building, transport
// (including 429/Retry-After backoff), response parsing, and returning either
// a complete []LLMResult of the same length as `comments` or an error.
//
// The failover wrapper composes two LLMClients (primary + optional fallback)
// behind the same interface and is the only place where provider-switching
// policy lives.
type LLMClient interface {
	ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error)
	// ProviderName returns a stable identifier (e.g. "openai", "anthropic")
	// used for attribution in comments.llm_provider.
	ProviderName() string
	// ModelName returns the configured model identifier persisted alongside
	// the classification (e.g. "gpt-4o", "claude-3-5-sonnet").
	ModelName() string
}
