## Context

Comment classification runs as an always-on batch worker (`internal/llm/processor.go`) that claims per-channel batches, calls an `LLMClient` (`internal/llm/client.go:23`), and persists category/topic/sentiment/confidence/insights into the shared `comments` table. Providers are registered in `internal/llm/failover.go:87` and selected by `LLM_PRIMARY`/`LLM_FALLBACK`; the generative providers all coerce a chat model into emitting a JSON array that `ParseClassificationResponse` (`internal/llm/prompts.go:250`) validates against a closed taxonomy. Jev is a different interface: state in, typed `Choice`/`Score`/`Noul` answers out, one `state` per request, no free-text generation (`https://docs.typesafe.ai/introduction/quickstart`).

The digest worker (`internal/llm/digest_worker.go`) is separate and currently hardwires `*opencodeClient` (`cmd/api/main.go:670`), computing numeric aggregates in Go and using the LLM only for qualitative synthesis.

Two facts constrain the design: Jev cannot emit `topic` or insight `description` (free text), and Jev cannot write the digest. Both must remain on a generative provider.

## Goals / Non-Goals

**Goals:**
- Route the judgment half of classification (category, sentiment, confidence) to Jev, gaining calibrated confidence and lower cost/latency.
- Keep `topic` and `actionable_insights` working via an extraction-only generative call, with no schema or API change.
- Make the whole thing opt-in and reversible with a single restart-scoped flag.
- Degrade safely: a Jev outage falls back to today's full generative classification.
- Measure Jev-vs-generative agreement before trusting it.
- Make the digest provider configurable while preserving its default behavior.

**Non-Goals:**
- Replacing generative digest synthesis with Jev (impossible by design).
- Removing the generative extraction leg (Jev cannot produce the free-text fields).
- Migrating `topic` to a bounded taxonomy or changing the `comments`/`weekly_digests` schema.
- Per-user or per-request flag granularity in this iteration (process-level only).
- Independent re-verification of TypeSafe's accuracy claims; we measure agreement on our own corpus.

## Decisions

### 1. Kill-switch flag `JEV_CLASSIFICATION_ENABLED` (default false)
- **Decision:** A process-level boolean read once at startup, matching existing flags (`LLM_PROCESSOR_ENABLED`, `DIGEST_WORKER_ENABLED`, `environment`/`envOn`, `cmd/api/main.go:679`). Branch happens in `newProcessorClient` (`main.go:716`).
- **Rationale:** The flag-off path is byte-for-byte the current behavior, so the change is provably inert until deliberately enabled, and rollback is a restart — no migration, no data rewrite.
- **Alternatives Considered:** Per-user or per-channel rollout (rejected: adds a config read to the hot claim path and branching inside `Processor`; process-level matches how every other feature flag here works). Runtime-mutable flag (rejected: repo convention is read-once env).

### 2. Hybrid split: Jev judge + generative extractor
- **Decision:** A `hybridClient` composes two `LLMClient`s. The judge (Jev) returns category/sentiment/confidence; the extractor (any generative provider) returns topic/insights via a new extraction-only prompt. Results merge per comment index into one `LLMResult`; missing indexes are the union of both legs.
- **Rationale:** Preserves the `LLMClient` contract, so `Processor.persistBatch` and the whole retry/park/quota machinery are untouched. Keeps `topic` and `actionable_insights` populated, so feed, insights filters, digest aggregates, and CSV export do not regress.
- **Alternatives Considered:** Full Jev (rejected: requires abandoning topic/insights or redefining the product surface). Jev-only gate with generative enrichment (deferred: more branching, less predictable latency). Calling the extractor's full classification and discarding its judgments (rejected: wastes output tokens for no benefit).

### 3. Per-comment fan-out for Jev, bounded concurrency
- **Decision:** `typeSafeClient.ClassifyBatch` issues one `POST /v1/systemone` per comment (state = comment body + parent excerpt + custom-prompt context), up to `TYPESAFE_MAX_CONCURRENCY` (default 8) in flight, and sums usage. Missing indexes come from failed/invalid per-comment calls.
- **Rationale:** Jev evaluates all questions against one `state` per request; per-comment is the only faithful mapping of the batch. Jev's 70–500ms latency and 32k-token state budget make per-comment calls viable where a chat model would be too slow/costly.
- **Alternatives Considered:** One call per channel batch with a JSON array state (rejected: questions apply to the whole state and cannot be asked reproducibly per element). Serial per-comment (rejected: loses the latency advantage).

### 4. Calibrated confidence replaces self-reported confidence
- **Decision:** Treat Jev's Choice `confidence` as the persisted `confidence` and as the input to `ApplyConfidenceThreshold` (`prompts.go:331`).
- **Rationale:** Jev is trained for calibration (RLCD), so confidence is meaningful in aggregate, enabling a real "act / review / human" split. The current self-reported value is not calibrated.
- **Consequences:** The 0.7 default threshold was tuned for self-reported values and MUST be re-tuned from shadow data rather than carried over blindly.

### 5. Generative failover wraps the hybrid
- **Decision:** When the flag is on, build `NewFailoverClient(hybrid, LLM_FALLBACK, threshold)`. `ProviderName()` reports both legs (e.g. `typesafe+opencode`) for metric separation.
- **Rationale:** Reuses the existing consecutive-failure failover semantics (`failover.go`), so a permanent Jev outage degrades to the proven full-generative path instead of parking batches.
- **Alternatives Considered:** No failover (rejected: a Jev outage would dead-letter comments).

### 6. Extraction-only prompt
- **Decision:** Add `BuildExtractionPrompt`/`ParseExtractionResponse` returning `{Topic, Insights}` and an `ExtractBatch` method on the generative providers.
- **Rationale:** The extractor no longer needs to emit category/sentiment/confidence, cutting generative output tokens (the dominant cost) while keeping the fields.
- **Alternatives Considered:** Reusing the full classification prompt (rejected: no token savings).

### 7. Configurable digest provider, Jev rejected
- **Decision:** Introduce `DIGEST_LLM_PROVIDER` (default `opencode`) and `DIGEST_LLM_FALLBACK`. The worker depends on a narrow `digestGenerator` interface (`GenerateDigestRaw` + `ProviderName` + `ModelName`) instead of `*opencodeClient`; add `GenerateDigestRaw` to the OpenAI and Anthropic clients. Reject `typesafe`/`jev`.
- **Rationale:** Removes the last hardwired `*opencodeClient`, lets the digest run on the same generative provider family as extraction, and prevents an operator from configuring a model that structurally cannot write prose.
- **Alternatives Considered:** Leaving the digest hardwired (rejected by the user decision to generalize). Letting the digest reuse the hybrid (impossible). Adding `GenerateDigestRaw` to the shared `LLMClient` (rejected: forces Jev to implement a method it cannot honor; an optional narrow interface is cleaner).

### 8. Shadow mode
- **Decision:** `LLM_SHADOW_PROVIDER` runs a comparison classifier asynchronously on sampled batches, emits agreement metrics (`noumena_llm_shadow_agreement_total{field,active_provider,shadow_provider}`) and logs, and never affects persisted results.
- **Rationale:** Provides the labelled evidence needed to tune the confidence threshold and decide cutover, without a separate offline pipeline.
- **Alternatives Considered:** Offline replay job (heavier; deferred). Blocking comparison (rejected: doubles latency on the critical path).

## Risks / Trade-offs

- **[Threshold drift]** Jev's calibrated confidence distribution differs from self-reported confidence, so 0.7 may over- or under-downgrade categories. → Mitigation: ship with the flag off, run shadow mode, and pick the threshold from observed agreement/error clusters before enabling.
- **[Fan-out rate/latency]** One request per comment multiplies call volume by batch size. → Mitigation: bounded `TYPESAFE_MAX_CONCURRENCY`, per-comment retry via `retryTransient`, and the existing batch timeout budget; concurrency is configurable.
- **[Literal reading]** Jev answers the question as written; negations and implied conditions are taken at face value, so negatively-phrased channel custom prompts underperform. → Mitigation: audit custom-prompt phrasing; surface guidance in the runbook.
- **[Two-leg cost accounting]** Cost now has two components and two providers. → Mitigation: sum both legs in one derived cost per batch; snapshot `PriceVersion`; separate metrics by provider identity.
- **[Extraction leg still generative]** Savings come from reduced output tokens, not from eliminating the generative call. → Mitigation: measured via cost analytics before/after; document the actual delta.
- **[Vendor early access]** API availability, key distribution, and terms are immature. → Mitigation: flag-off default; failover to generative; key required only when enabled.
- **[Correlated failure with generic failover]** `LLM_FALLBACK` may be the same vendor family as the extractor. → Mitigation: document that extraction and fallback should be chosen deliberately; failover is on the classification result, not per leg.

## Migration Plan

1. **Ship inert:** merge the Jev provider, hybrid, config, and flag with `JEV_CLASSIFICATION_ENABLED=false`; no behavior change. Verify `go test ./...` and that flag-off selects the legacy client.
2. **Staging enable:** set `JEV_CLASSIFICATION_ENABLED=true`, `TYPESAFE_API_KEY`, `LLM_EXTRACTOR_PROVIDER=opencode`, `LLM_FALLBACK=openai`; run batch classification and confirm results populate all fields.
3. **Shadow evaluation:** enable `LLM_SHADOW_PROVIDER=opencode` for 1–2 weeks; collect agreement metrics; choose the confidence threshold.
4. **Production cutover:** enable the flag with the tuned `LLM_CONFIDENCE_THRESHOLD`.
5. **Rollback:** set `JEV_CLASSIFICATION_ENABLED=false` and restart; classification reverts to `LLM_PRIMARY`/`LLM_FALLBACK`. The digest provider change is independent and defaults to today's OpenCode behavior.
6. **Digest provider:** can ship separately; leave `DIGEST_LLM_PROVIDER` unset to preserve current behavior.

## Open Questions

- Which generative provider should be the default extractor — reuse `LLM_FALLBACK` or a dedicated `LLM_EXTRACTOR_PROVIDER` defaulting to `opencode`? (Design assumes a dedicated variable defaulting to `opencode`.)
- Should shadow mode sample per batch deterministically (hash of channel ID) or randomly, to make agreement metrics reproducible? (Lean deterministic hash for stable comparisons.)
- Is a per-channel opt-out needed once the global flag is on, given custom prompts interact with Jev's literalism? (Deferred.)
