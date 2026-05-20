package llm

import (
	"fmt"
)

// ProviderConfig is the minimal per-provider configuration consumed by the
// factory. The caller (cmd/api/main.go) populates this from internal/config.
//
// Each field is optional from the factory's perspective; the factory returns
// (nil, nil) when the named provider has no API key configured, allowing the
// fallback wiring to gracefully disable failover (see
// llm-provider-failover/spec.md "Fallback unavailable when not configured").
type ProviderConfig struct {
	OpenAIAPIKey  string
	OpenAIModel   string

	AnthropicAPIKey string
	AnthropicModel  string

	OpenCodeAPIKey  string
	OpenCodeBaseURL string
	OpenCodeModel   string
}

// BuildLLMClientByName returns an LLMClient implementation selected by `name`.
// Recognized names are "opencode", "openai", and "anthropic" (case-sensitive,
// matching the values configured via LLM_PRIMARY_PROVIDER /
// LLM_FALLBACK_PROVIDER).
//
// Behavior:
//   - Returns (nil, nil) when `name` is empty: caller treats this as "no
//     provider configured" (used for the optional fallback slot).
//   - Returns (nil, nil) when the provider is recognized but its API key is
//     empty. The caller decides whether this is fatal (primary) or a warning
//     (fallback).
//   - Returns (nil, error) for an unknown name; the caller MUST treat this
//     as a startup misconfiguration.
func BuildLLMClientByName(name string, cfg ProviderConfig) (LLMClient, error) {
	switch name {
	case "":
		return nil, nil
	case "opencode":
		if cfg.OpenCodeAPIKey == "" {
			return nil, nil
		}
		return NewOpenCodeClient(cfg.OpenCodeAPIKey, cfg.OpenCodeBaseURL, cfg.OpenCodeModel), nil
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, nil
		}
		return NewOpenAIClient(cfg.OpenAIAPIKey, cfg.OpenAIModel), nil
	case "anthropic":
		if cfg.AnthropicAPIKey == "" {
			return nil, nil
		}
		return NewAnthropicClient(cfg.AnthropicAPIKey, cfg.AnthropicModel), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider name: %q (valid: opencode, openai, anthropic)", name)
	}
}

// RequiredEnvVarForProvider returns the env-var name whose absence is fatal
// when `name` is the configured primary provider. Used by main.go to produce
// a clear log.Fatalf message.
func RequiredEnvVarForProvider(name string) string {
	switch name {
	case "opencode":
		return "OPENCODE_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	default:
		return ""
	}
}
