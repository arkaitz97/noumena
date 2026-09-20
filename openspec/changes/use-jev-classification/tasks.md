## 1. Spike and configuration

- [ ] 1.1 Obtain a TypeSafe early-access key and verify `POST https://api.typesafe.ai/v1/systemone` with a representative comment, confirming the response shape, latency, and rate-limit headers
- [ ] 1.2 Add config fields to `internal/config/config.go`: `JEVClassificationEnabled`, `TypeSafeAPIKey`, `TypeSafeBaseURL`, `TypeSafeModel`, `TypeSafeMaxConcurrency`, `LLMExtractorProvider`, `LLMShadowProvider`, `DigestLLMProvider`, `DigestLLMFallback`
- [ ] 1.3 Add the corresponding variables to `.env.example` with `# REQUIRED | optional` annotations and defaults, and keep `TestEnvExampleParity` green
- [ ] 1.4 Add `typesafe` to `providerEnvKeys` in `cmd/api/main.go` mapping to `TYPESAFE_API_KEY`

## 2. Jev provider

- [ ] 2.1 Add `internal/llm/jev.go` with `typeSafeClient` implementing `LLMClient`: request/response structs for `systemone`, `ProviderName()="typesafe"`, `ModelName()` from config
- [ ] 2.2 Implement `ClassifyBatch` as a per-comment fan-out with a bounded semaphore (`TYPESAFE_MAX_CONCURRENCY`), one `category` Choice (8 labels), one `sentiment` Choice (3 labels), and summed `TokenUsage`
- [ ] 2.3 Map answers to `LLMResult`: validate the selected category against `ValidCategories`, use Jev `confidence`, mark failed/out-of-taxonomy indexes missing, and reuse `retryTransient`/`terminal` for 429/5xx vs 4xx
- [ ] 2.4 Register `typesafe`/`jev` in `providerFactories` (`internal/llm/failover.go`) and add `NewTypeSafeClient`
- [ ] 2.5 Add `internal/llm/jev_test.go` using `httptest`: valid mapping, invalid category to missing, partial batch, 429 retry, non-429 4xx terminal, timeout, usage summation
- [ ] 2.6 Add the `jev-latest` price card (`$0.042/M` input, $0 output) and `typesafe` provider default in `internal/llm/prices.go`, and bump `PriceVersion`
- [ ] 2.7 Add price-card tests covering `jev-latest`, an unknown `typesafe` model falling back to the provider default, and version snapshot behavior

## 3. Extraction split

- [ ] 3.1 Add `ExtractionSystemPrompt`, `BuildExtractionPrompt`, `ParseExtractionResponse`, and an `ExtractionResult{Topic, Insights}` type in `internal/llm/prompts.go` (topic and actionable_insights only)
- [ ] 3.2 Add `ExtractBatch(ctx, comments)` to `opencodeClient`, `openaiClient`, and `anthropicClient`, reusing `httpClassifyOnce` and the shared transient retry schedule
- [ ] 3.3 Add extraction parser/builder unit tests, including malformed and partial responses

## 4. Hybrid classifier

- [ ] 4.1 Add `internal/llm/hybrid.go` with `hybridClient{judge, extractor}` implementing `LLMClient`; run both legs concurrently under the parent context
- [ ] 4.2 Merge per comment index: judge category/sentiment/confidence plus extractor topic/insights; compute missing as the union of both legs; sum usage; report `ProviderName()` as `judge+extractor`
- [ ] 4.3 Add `internal/llm/hybrid_test.go` covering merge, concurrent invocation, union-missing requeue semantics, and one-leg failure

## 5. Flag, wiring, and failover

- [ ] 5.1 Add `JEV_CLASSIFICATION_ENABLED` parsing in `internal/config/config.go` and document it in `.env.example`
- [ ] 5.2 Branch `newProcessorClient` (`cmd/api/main.go`) on the flag: off → unchanged `LLM_PRIMARY`/`LLM_FALLBACK`; on → require `TYPESAFE_API_KEY`, build `hybridClient(Typesafe, LLMExtractorProvider)`, wrap with `NewFailoverClient(hybrid, LLM_FALLBACK, threshold)`
- [ ] 5.3 Fail startup with a clear error when the flag is on and `TYPESAFE_API_KEY` is empty
- [ ] 5.4 Add tests asserting flag-off constructs the legacy client and does not read `TYPESAFE_API_KEY`, and flag-on builds the hybrid wrapped in failover

## 6. Configurable digest provider

- [ ] 6.1 Add `GenerateDigestRaw` to `openaiClient` and `anthropicClient` mirroring the OpenCode implementation
- [ ] 6.2 Define a narrow `digestGenerator` interface (`GenerateDigestRaw`, `ProviderName`, `ModelName`) and change `llm.Worker.LLMClient` to it; update `recordDigestCost` to use `ProviderName()`
- [ ] 6.3 Resolve the digest client in `startDigestWorker` from `DIGEST_LLM_PROVIDER`/`DIGEST_LLM_FALLBACK`, reject `typesafe`/`jev`, refuse to start when the provider key is missing, and default to the existing OpenCode configuration
- [ ] 6.4 Add tests for the default (OpenCode) path, Jev rejection, missing-key refusal, and cost attribution under a non-OpenCode provider

## 7. Shadow mode

- [ ] 7.1 Add `internal/llm/shadow.go` with `shadowClient{active, shadow, sampleRate}`, deterministic sampling, and async shadow invocation that never blocks the active result
- [ ] 7.2 Emit `noumena_llm_shadow_agreement_total{field,active_provider,shadow_provider}` (category exact-match, sentiment exact-match, confidence delta, topic/insight overlap) and structured logs
- [ ] 7.3 Wire `LLM_SHADOW_PROVIDER` in `newProcessorClient` only when the Jev flag is on; add tests for sampled comparison, non-fatal shadow errors, and no-op when unset

## 8. Verification and docs

- [ ] 8.1 Run `gofmt -w`, `go vet ./...`, and `go test ./...` green; confirm flag-off behavior is unchanged
- [ ] 8.2 Add a runbook (for example `docs/ops/runbooks/jev-classification.md`) covering enable, shadow evaluation, threshold tuning, digest-provider selection, and rollback
- [ ] 8.3 Update `internal/config` env parity and any affected JSON/output tests; verify `openspec validate --change use-jev-classification`
