## Context

The LLM processing layer currently supports three providers (OpenCode, OpenAI, Anthropic) with a failover mechanism that switches from primary to fallback on error threshold breach. Comment classification uses the `LLMClient` interface (`ClassifyBatch`), while weekly digest generation uses a separate `DigestLLMClient` interface implemented by a legacy HTTP client directly calling OpenAI's API.

The system uses external SDKs (`go-openai`, `anthropic-sdk-go`) for two providers and a hand-rolled HTTP client for OpenCode. Configuration includes 15+ environment variables for API keys, models, base URLs, failover thresholds, and window settings.

OpenCode is already the default primary provider and implements an OpenAI-compatible API, making it a natural consolidation target.

## Goals / Non-Goals

**Goals:**
- Single LLM provider (OpenCode) for all operations: classification and digest generation
- Remove failover complexity (sliding window error tracking, one-way switch logic)
- Remove external SDK dependencies (`go-openai`, `anthropic-sdk-go`)
- Unified `LLMClient` interface implementation for all LLM operations
- Simplified configuration: only `OPENCODE_*` environment variables needed

**Non-Goals:**
- No changes to LLM prompt structure, taxonomy, or output format
- No changes to batch processing logic, confidence thresholds, or quota tracking
- No changes to digest worker scheduling, retry chain, or content shape
- No performance optimization beyond removing unnecessary abstraction layers

## Decisions

### 1. Remove failover client entirely

**Rationale:** With a single provider, failover is meaningless. The sliding window error tracking, one-way switch logic, and `recordLLMFailoverSwitch` telemetry add complexity with no benefit.

**Alternatives considered:**
- Keep failover infrastructure for future multi-provider support: Adds dead code. Can be re-added when needed.
- Replace with circuit breaker: Over-engineering for single provider. Per-provider retry logic already handles transient errors.

### 2. Extend `opencodeClient` to implement `DigestLLMClient`

**Rationale:** The existing `opencodeClient` already has HTTP transport, retry logic, and response parsing. Adding `GenerateDigestRaw` method reuses this infrastructure instead of maintaining a separate legacy client.

**Implementation:** Add `GenerateDigestRaw(ctx, prompt string) (string, error)` to `*opencodeClient`. Use the same OpenAI-compatible chat completions endpoint with the digest-specific prompt.

### 3. Remove factory pattern, use direct constructor

**Rationale:** `BuildLLMClientByName` with `ProviderConfig` struct exists to support multiple providers. With only OpenCode, `cmd/api/main.go` can call `NewOpenCodeClient` directly.

**Files removed:** `factory.go`

### 4. Remove provider-specific environment variables

**Rationale:** `OPENAI_API_KEY`, `OPENAI_MODEL`, `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL`, `LLM_FALLBACK_PROVIDER`, `LLM_FAILOVER_ERROR_THRESHOLD`, `LLM_FAILOVER_WINDOW_SECONDS` become dead configuration.

**Retained variables:**
- `OPENCODE_API_KEY`
- `OPENCODE_BASE_URL`
- `OPENCODE_MODEL`
- `LLM_PROCESSOR_ENABLED`
- `LLM_PROCESSOR_TICK_INTERVAL`
- `LLM_BATCH_SIZE`
- `LLM_CONFIDENCE_THRESHOLD`
- `LLM_PROCESSOR_SHUTDOWN_GRACE`
- `DIGEST_WORKER_ENABLED`

### 5. Remove `provider_switched` telemetry metric

**Rationale:** `recordLLMFailoverSwitch` in `telemetry.go` is only called during failover. With no failover, this metric and its OpenTelemetry instrumentation are dead code.

### 6. Digest worker uses same client as classification

**Rationale:** Currently `digest_worker.go` uses a separate `DigestLLMClient` interface and legacy `*Client`. Both operations now use the same `*opencodeClient` instance, created once at startup and passed to both `Processor` and `Worker`.

### 7. Retain per-provider retry logic in `opencodeClient`

**Rationale:** The existing retry-with-backoff (3 retries, [1s, 2s, 4s], honoring `Retry-After` header) in `opencodeClient` handles transient errors adequately. This is sufficient without a failover layer.

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| **Single point of failure**: No fallback if OpenCode is down. | OpenCode is an internal gateway with its own redundancy. Per-provider retry handles transient errors. If needed, failover can be re-added later. |
| **Digest generation compatibility**: OpenCode must support the same prompt/response format as the legacy OpenAI digest client. | OpenCode uses OpenAI-compatible API. The digest prompt is a standard chat completion. Validate during testing. |
| **Configuration migration**: Existing deployments with `OPENAI_*` or `ANTHROPIC_*` vars will have unused config. | Document the change. Unused env vars are harmless (just ignored). |
| **Model capability differences**: OpenCode's model may produce different classification quality than GPT-4o or Claude. | Already in use as default primary provider. Quality is validated. |

## Migration Plan

1. Extend `opencodeClient` with `GenerateDigestRaw` method
2. Update `digest_worker.go` to accept `*opencodeClient` instead of legacy `*Client`
3. Remove `digest_client.go`, `digest_llm.go`
4. Simplify `cmd/api/main.go` — direct `NewOpenCodeClient` call, no factory or failover
5. Remove `openai.go`, `anthropic.go`, `failover.go`, `factory.go`
6. Remove `recordLLMFailoverSwitch` from `telemetry.go`
7. Update `config.go` — remove OpenAI, Anthropic, failover fields
8. Update `.env.example` — remove unused variables
9. Run `go mod tidy` to remove unused SDK dependencies
10. Run full test suite

**Rollback:** Revert the change commit. All removed files and configuration are preserved in git.

## Open Questions

1. **OpenCode model configuration**: Should `OPENCODE_MODEL` default to a specific model, or remain `opencode/default`? (Current default is fine.)
2. **Telemetry replacement**: Should we add a generic `llm_request_duration` or `llm_error_rate` metric to replace the removed `provider_switched` metric? (Out of scope for this change.)
