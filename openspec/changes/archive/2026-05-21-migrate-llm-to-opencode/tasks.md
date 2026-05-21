## 1. Extend OpenCode Client for Digest Support

- [x] 1.1 Add `GenerateDigestRaw(ctx context.Context, prompt string) (string, error)` method to `*opencodeClient` in `opencode.go`
- [x] 1.2 Update `opencodeClient` to satisfy both `LLMClient` and `DigestLLMClient` interfaces
- [x] 1.3 Verify OpenCode client retry logic applies to digest generation calls

## 2. Update Digest Worker to Use OpenCode Client

- [x] 2.1 Update `digest_worker.go` to accept `*opencodeClient` instead of legacy `*Client`
- [x] 2.2 Remove `DigestLLMClient` interface usage from digest worker
- [x] 2.3 Delete `digest_client.go` (legacy HTTP client)
- [x] 2.4 Delete `digest_llm.go` (digest interface and prompt helpers if not used elsewhere)

## 3. Simplify Application Startup

- [x] 3.1 Update `cmd/api/main.go` to call `NewOpenCodeClient` directly instead of factory + failover chain
- [x] 3.2 Pass single `*opencodeClient` instance to both `llm.NewProcessor` and `llm.Worker`
- [x] 3.3 Remove failover client initialization and `FailoverConfig` from main.go

## 4. Remove Unused Provider Files

- [x] 4.1 Delete `internal/llm/openai.go`
- [x] 4.2 Delete `internal/llm/anthropic.go`
- [x] 4.3 Delete `internal/llm/failover.go`
- [x] 4.4 Delete `internal/llm/factory.go`

## 5. Remove Failover Telemetry

- [x] 5.1 Remove `recordLLMFailoverSwitch` function from `internal/llm/telemetry.go`
- [x] 5.2 Remove related OpenTelemetry metric definition if isolated to failover

## 6. Update Configuration

- [x] 6.1 Remove `OpenAIAPIKey`, `OpenAIModel`, `AnthropicAPIKey`, `AnthropicModel`, `OpenCodeAPIKey`, `OpenCodeBaseURL`, `OpenCodeModel` fields from `ProviderConfig` — replace with single `OpenCodeConfig` struct
- [x] 6.2 Remove `LLMPrimaryProvider`, `LLMFallbackProvider`, `LLMFailoverErrorThreshold`, `LLMFailoverWindowSeconds` from `config.go`
- [x] 6.3 Remove `RequiredEnvVarForProvider` function from config
- [x] 6.4 Update `.env.example` — remove `OPENAI_API_KEY`, `OPENAI_MODEL`, `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL`, `LLM_FALLBACK_PROVIDER`, `LLM_FAILOVER_ERROR_THRESHOLD`, `LLM_FAILOVER_WINDOW_SECONDS`
- [x] 6.5 Retain `OPENCODE_API_KEY`, `OPENCODE_BASE_URL`, `OPENCODE_MODEL` in `.env.example`

## 7. Clean Up Dependencies

- [x] 7.1 Remove `github.com/sashabaranov/go-openai` from `go.mod`
- [x] 7.2 Remove `github.com/anthropics/anthropic-sdk-go` from `go.mod`
- [x] 7.3 Run `go mod tidy`

## 8. Update Tests

- [x] 8.1 Update any LLM-related tests to use `*opencodeClient` or mock the interface
- [x] 8.2 Remove tests specific to OpenAI, Anthropic, and failover behavior
- [x] 8.3 Run `go test ./internal/llm/...` and fix any failures

## 9. Documentation

- [x] 9.1 Update `openspec/specs/llm-processing/spec.md` with delta spec changes
- [x] 9.2 Update `AGENTS.md` LLM guidance to reflect single-provider architecture
- [x] 9.3 Run full test suite `go test ./...` and verify all tests pass
