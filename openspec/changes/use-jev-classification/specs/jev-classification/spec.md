# Jev Classification Specification

## Purpose
Adds TypeSafe AI's Jev System One model as a decision provider for comment classification, a hybrid Jev+generative classifier for the free-text fields Jev cannot produce, a restart-scoped kill-switch flag, generative failover, and shadow-mode agreement measurement.

## ADDED Requirements

### Requirement: TypeSafe Jev provider
The system SHALL provide a `typesafe` LLM provider (`typeSafeClient`) that implements the `LLMClient` interface, calls `POST {TYPESAFE_BASE_URL}/v1/systemone` with `Authorization: Bearer {TYPESAFE_API_KEY}` and model `TYPESAFE_MODEL` (default `jev-latest`), and reports `ProviderName()="typesafe"` and `ModelName()` equal to the configured model.

#### Scenario: Provider constructed from configuration
- **GIVEN** `TYPESAFE_API_KEY` is set and `TYPESAFE_BASE_URL`/`TYPESAFE_MODEL` are optionally set
- **WHEN** the client is constructed
- **THEN** the system SHALL send `Bearer` auth to `{baseURL}/v1/systemone` using the configured model, defaulting the base URL to `https://api.typesafe.ai` and the model to `jev-latest`

#### Scenario: Per-comment parallel fan-out
- **GIVEN** a batch of N comments grouped by channel
- **WHEN** `ClassifyBatch` runs
- **THEN** the system SHALL issue one `systemone` request per comment (Jev accepts a single `state` per request), concurrently up to `TYPESAFE_MAX_CONCURRENCY` (default 8), and SHALL return results in the same index order as the input

#### Scenario: Typed answer mapping
- **GIVEN** a Jev response containing a `category` Choice, a `sentiment` Choice, and their `confidence` values
- **WHEN** the provider maps the answer to an `LLMResult`
- **THEN** the system SHALL set `Category` to the selected choice when it is in `ValidCategories` (otherwise treat the index as missing), set `Sentiment` to the selected label, and set `Confidence` from Jev's calibrated Choice `confidence`

#### Scenario: Transient versus terminal transport errors
- **GIVEN** a `systemone` request that returns HTTP 429 or 5xx, or a transport error
- **WHEN** the provider handles the response
- **THEN** the error SHALL be retryable via the shared `retryTransient` schedule, while any other 4xx SHALL be terminal

#### Scenario: Partial batch failure preserves valid siblings
- **GIVEN** a batch where some per-comment `systemone` calls fail or return an out-of-taxonomy choice
- **WHEN** the provider aggregates results
- **THEN** the system SHALL return the valid results and the indexes of the failed/invalid comments as missing, without discarding valid siblings

#### Scenario: Token usage aggregated for cost accounting
- **GIVEN** a fan-out of N `systemone` requests each reporting `usage.input_tokens` and `usage.output_tokens`
- **WHEN** the provider returns
- **THEN** the system SHALL sum the reported usage across the fan-out into a single `TokenUsage` for the batch

### Requirement: Hybrid Jev plus extraction classifier
The system SHALL provide a hybrid `LLMClient` composed of a Jev judge and a generative extractor. The judge SHALL supply `category`, `sentiment`, and `confidence`; the extractor SHALL supply `topic` and `actionable_insights` via an extraction-only prompt and parser. The hybrid SHALL merge the two results per comment index and SHALL preserve the `LLMClient` contract so the processor requires no changes.

#### Scenario: Merge judgments and extractions
- **GIVEN** a batch where the Jev judge returns category/sentiment/confidence per comment and the extractor returns topic/actionable_insights per comment
- **WHEN** the hybrid merges the results
- **THEN** each persisted `LLMResult` SHALL carry Jev's category, sentiment, and confidence together with the extractor's topic and actionable insights

#### Scenario: Concurrent legs
- **GIVEN** the hybrid is invoked for a batch
- **WHEN** it calls both providers
- **THEN** the judge and extractor calls SHALL run concurrently under the same parent context and batch timeout

#### Scenario: Missing indexes are the union of both legs
- **GIVEN** a comment index that is valid in the extractor result but missing or invalid in the judge result (or vice versa)
- **WHEN** the hybrid computes missing indexes
- **THEN** the index SHALL be reported missing so the existing bounded-retry path requeues it

#### Scenario: Extraction-only prompt omits judgment fields
- **WHEN** the extractor prompt is built
- **THEN** it SHALL request only `topic` and `actionable_insights` and SHALL NOT request category, sentiment, or confidence

### Requirement: Feature-flagged classification flow
The system SHALL gate Jev-based classification behind the process-level environment flag `JEV_CLASSIFICATION_ENABLED` (default `false`), read once at startup. When the flag is `false`, provider selection SHALL be unchanged (`LLM_PRIMARY`/`LLM_FALLBACK`) and no Jev code path SHALL execute. When the flag is `true`, the processor SHALL use the hybrid client.

#### Scenario: Flag disabled preserves the existing flow
- **GIVEN** `JEV_CLASSIFICATION_ENABLED` is unset or `false`
- **WHEN** the LLM processor is started
- **THEN** the system SHALL construct the same client it constructs today from `LLM_PRIMARY` and `LLM_FALLBACK`, and SHALL NOT read `TYPESAFE_API_KEY` or require it

#### Scenario: Flag enabled selects the hybrid
- **GIVEN** `JEV_CLASSIFICATION_ENABLED=true` and `TYPESAFE_API_KEY` is set
- **WHEN** the LLM processor is started
- **THEN** the system SHALL construct the hybrid client using the Jev judge and the `LLM_EXTRACTOR_PROVIDER` generative extractor

#### Scenario: Flag enabled without an API key fails fast
- **GIVEN** `JEV_CLASSIFICATION_ENABLED=true` and `TYPESAFE_API_KEY` is empty
- **WHEN** the LLM processor is started
- **THEN** the system SHALL fail startup with a clear configuration error naming `TYPESAFE_API_KEY`

#### Scenario: Rollback without downtime migration
- **GIVEN** the flag was enabled and proved unhealthy
- **WHEN** an operator sets `JEV_CLASSIFICATION_ENABLED=false` and restarts
- **THEN** classification SHALL revert to the legacy provider path with no schema or data change

### Requirement: Generative failover for the hybrid
When the flag is enabled, the system SHALL wrap the hybrid client in the existing failover wrapper using `LLM_FALLBACK` as the fallback, so a permanent Jev outage degrades to the full generative classification path.

#### Scenario: Jev consecutive failures flip to generative
- **GIVEN** the flag is enabled and a generative `LLM_FALLBACK` is configured
- **WHEN** the hybrid produces `LLM_FAILOVER_ERROR_THRESHOLD` consecutive failed classification calls
- **THEN** the system SHALL switch to the full generative provider for the process lifetime

#### Scenario: Provider identity separates flagged from legacy traffic
- **WHEN** the hybrid reports its provider identity
- **THEN** `ProviderName()` SHALL identify both legs (for example `typesafe+opencode`) so provider-call metrics distinguish flagged from legacy classification

### Requirement: Shadow-mode agreement measurement
The system SHALL support an optional shadow provider (`LLM_SHADOW_PROVIDER`) that runs alongside the active classifier in sampled batches to measure agreement, without affecting persisted results or the critical path.

#### Scenario: Shadow comparison recorded
- **GIVEN** a shadow provider is configured and a batch is sampled
- **WHEN** the active classifier returns
- **THEN** the system SHALL invoke the shadow classifier asynchronously and record agreement metrics (category exact-match, sentiment exact-match, confidence delta, topic/insight overlap) labeled by both provider names, while the active classifier's results SHALL be the ones returned

#### Scenario: Shadow failure is non-fatal
- **GIVEN** the shadow provider errors or times out
- **WHEN** the active classifier has already returned
- **THEN** the shadow error SHALL be logged and SHALL NOT change the returned results or fail the job

#### Scenario: Shadow disabled is a no-op
- **GIVEN** `LLM_SHADOW_PROVIDER` is unset
- **WHEN** classification runs
- **THEN** no shadow call SHALL be made and no shadow metric SHALL be emitted
