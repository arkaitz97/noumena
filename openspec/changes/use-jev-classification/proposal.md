## Why

Comment classification currently rents a generative LLM (`opencode`/`openai`/`anthropic`) to emit JSON that the parser then validates, so every batch pays output-token cost for prose we discard, and the persisted `confidence` is the model's self-report with no calibration guarantee. TypeSafe AI's Jev is a System One decision model that returns typed `Choice`/`Score`/`Noul` answers with calibrated probabilities in ~70–500ms at $0.042/M input tokens (output free) and structurally cannot emit a value outside the declared schema `(source: https://docs.typesafe.ai/introduction/quickstart, https://docs.typesafe.ai/confidence)`. We want to route the judgment half of classification to Jev while keeping a generative provider for the free-text half (`topic`, insight `description`), all behind a kill-switch flag so the current flow stays the default and the change is reversible at restart.

## What Changes

- Add a `typesafe`/`jev` LLM provider implementing `LLMClient` (per-comment parallel `systemone` fan-out).
- Add a hybrid classifier: Jev supplies `category`, `sentiment`, and calibrated `confidence`; the existing generative provider supplies `topic` and `actionable_insights` via an extraction-only prompt.
- Gate the entire flow behind `JEV_CLASSIFICATION_ENABLED` (default `false`): off = today's `LLM_PRIMARY`/`LLM_FALLBACK` path byte-for-byte; on = hybrid wrapped in generative failover.
- Add a shadow-comparison mode (`LLM_SHADOW_PROVIDER`) to measure Jev-vs-generative agreement before cutover.
- Add a Jev price card ($0.042/M input, $0 output) to cost analytics.
- Make the weekly digest provider configurable (`DIGEST_LLM_PROVIDER`, default `opencode`) instead of hardwiring `*opencodeClient`; Jev is rejected as a digest provider because it cannot generate prose.

## Capabilities

### New Capabilities
- `jev-classification`: The TypeSafe Jev provider, per-comment fan-out, the hybrid Jev+extraction classifier, the `JEV_CLASSIFICATION_ENABLED` kill-switch, and shadow-mode agreement measurement.

### Modified Capabilities
- `llm-processing`: Provider selection becomes flag-gated; the "OpenCode as Sole LLM Provider" requirement is replaced by configurable generative-vs-hybrid selection.
- `llm-cost-analytics`: Add the Jev price card and price the hybrid's two legs per batch.
- `weekly-digest`: Digest generation uses a configurable generative provider rather than a hardwired OpenCode client.

## Impact

- Code: `internal/llm/jev.go` (new), `internal/llm/hybrid.go` (new), `internal/llm/shadow.go` (new), `internal/llm/prompts.go`, `internal/llm/opencode.go`/`openai.go`/`anthropic.go` (extraction + digest methods), `internal/llm/failover.go`, `internal/llm/prices.go`, `internal/llm/digest_worker.go`, `internal/config/config.go`, `cmd/api/main.go`, `.env.example`.
- Config: `JEV_CLASSIFICATION_ENABLED`, `TYPESAFE_API_KEY`, `TYPESAFE_BASE_URL`, `TYPESAFE_MODEL`, `TYPESAFE_MAX_CONCURRENCY`, `LLM_EXTRACTOR_PROVIDER`, `LLM_SHADOW_PROVIDER`, `DIGEST_LLM_PROVIDER`, `DIGEST_LLM_FALLBACK`.
- Dependencies: external TypeSafe AI API (early access), API key required only when the flag is on.
- Schema: none — `comments` columns and `weekly_digests.content` are unchanged, so feed, insights, and digest aggregates keep working.
- **Non-goals:** replacing the generative digest with Jev; removing the generative extraction leg; changing `topic` to a bounded taxonomy; per-user (as opposed to process-level) flag granularity.
