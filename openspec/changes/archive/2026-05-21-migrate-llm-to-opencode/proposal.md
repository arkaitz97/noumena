## Why

Consolidate all LLM operations to use OpenCode as the sole provider, eliminating external API dependencies (OpenAI, Anthropic), reducing costs, simplifying the failover mechanism, and centralizing LLM routing through our internal gateway.

## What Changes

- Remove OpenAI and Anthropic as LLM providers for comment classification
- Remove the failover client (no longer needed with single provider)
- Migrate the weekly digest worker from legacy OpenAI HTTP client to OpenCode
- Simplify `LLMClient` interface to single OpenCode implementation
- Remove OpenAI and Anthropic SDK dependencies (`go-openai`, `anthropic-sdk-go`)
- Update environment variables: remove `OPENAI_*`, `ANTHROPIC_*`, `LLM_FALLBACK_PROVIDER`, `LLM_FAILOVER_*`
- Retain `OPENCODE_*` variables as the sole LLM configuration

## Capabilities

### New Capabilities

(none — this is an implementation consolidation, no new user-facing capabilities)

### Modified Capabilities

- `llm-processing`: Remove multi-provider support and failover mechanism; OpenCode becomes the sole provider for both classification and digest generation

## Impact

- **`internal/llm/`**: Remove `openai.go`, `anthropic.go`, `failover.go`, `factory.go`, `digest_client.go`, `digest_llm.go`. Rewrite `opencode.go` to support both `LLMClient` and `DigestLLMClient` interfaces. Update `processor.go` and `digest_worker.go` to use unified OpenCode client.
- **`internal/config/config.go`**: Remove OpenAI, Anthropic, and failover configuration fields
- **`cmd/api/main.go`**: Simplify LLM initialization — direct `NewOpenCodeClient()` instead of factory + failover chain
- **`go.mod`**: Remove `github.com/sashabaranov/go-openai` and `github.com/anthropics/anthropic-sdk-go`
- **`.env.example`**: Remove `OPENAI_*`, `ANTHROPIC_*`, `LLM_FALLBACK_PROVIDER`, `LLM_FAILOVER_*` variables
- **`openspec/specs/llm-processing/spec.md`**: Update to reflect single-provider architecture
- **`internal/llm/telemetry.go`**: Remove `recordLLMFailoverSwitch` metric
