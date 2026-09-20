## REMOVED Requirements

### Requirement: OpenCode as Sole LLM Provider
**Reason**: Provider selection is no longer fixed to OpenCode. When `JEV_CLASSIFICATION_ENABLED=false` the configurable `LLM_PRIMARY`/`LLM_FALLBACK` providers are used (as already implemented by the failover requirement); when `true` the hybrid Jev+extraction classifier is used. A single hardwired `*opencodeClient` for both processor and digest no longer describes the system.
**Migration**: Replace OpenCode-only wiring with flag-gated selection: flag off → `LLM_PRIMARY`/`LLM_FALLBACK`; flag on → hybrid Jev judge + `LLM_EXTRACTOR_PROVIDER` extractor. Digest provider configuration is owned by the `weekly-digest` capability (`DIGEST_LLM_PROVIDER`).

### Requirement: Digest generation via OpenCode
**Reason**: Digest generation is no longer bound to OpenCode. The digest worker uses a configurable generative provider so it can run on OpenCode, OpenAI, or Anthropic.
**Migration**: Use `DIGEST_LLM_PROVIDER` (default `opencode`) and `DIGEST_LLM_FALLBACK`; see the `weekly-digest` capability's "Configurable generative digest provider" requirement.

## MODIFIED Requirements

### Requirement: LLM provider failover
The system SHALL support a configurable primary and fallback LLM provider, switching to the fallback after a configurable number of consecutive classification failures. When `JEV_CLASSIFICATION_ENABLED=true`, the hybrid Jev+extraction client SHALL be the primary and `LLM_FALLBACK` SHALL be the fallback, so a permanent Jev outage degrades to full generative classification.

#### Scenario: Primary provider fails consecutively
- **GIVEN** a configured primary LLM provider and a fallback
- **WHEN** the primary produces `LLM_FAILOVER_ERROR_THRESHOLD` consecutive failed classification calls (default 3; successes reset the count; timing irrelevant)
- **THEN** the system switches to the fallback provider for the process lifetime

#### Scenario: Slow failure cadence still triggers failover
- **GIVEN** primary failures that arrive minutes apart (one per retry cycle)
- **WHEN** the consecutive-failure count reaches the threshold
- **THEN** the failover SHALL trigger — the previous sliding-window rule must not be reintroduced

#### Scenario: No fallback configured
- **GIVEN** only a primary provider is configured
- **WHEN** the primary fails
- **THEN** the system continues to retry the primary and does not crash

#### Scenario: Hybrid Jev primary fails consecutively
- **GIVEN** `JEV_CLASSIFICATION_ENABLED=true`, a Jev judge, a generative extractor, and a generative `LLM_FALLBACK`
- **WHEN** the hybrid client produces `LLM_FAILOVER_ERROR_THRESHOLD` consecutive failed classification calls
- **THEN** the system SHALL permanently switch to the full generative fallback for the process lifetime

### Requirement: Bounded retry attempts with backoff
The system SHALL cap how many times any comment may be sent to the LLM for classification at `LLM_MAX_ATTEMPTS` (default 3) and SHALL delay each re-attempt by an escalating backoff instead of reclaiming on the next tick.

#### Scenario: Per-attempt retry state recorded
- **GIVEN** a classification job that fails with a transient error
- **WHEN** the processor records the failure
- **THEN** every comment in the batch SHALL have `llm_attempts` incremented by 1 and `llm_next_retry_at` set to now plus the backoff entry for the new attempt count (default schedule 1m, 5m, 15m)

#### Scenario: Backoff gates claiming
- **GIVEN** a comment with `llm_attempts < LLM_MAX_ATTEMPTS` and `llm_next_retry_at` in the future
- **WHEN** the processor claims unprocessed comments
- **THEN** the comment SHALL NOT be included in any batch until `llm_next_retry_at` has passed

#### Scenario: Attempt cap dead-letters a comment
- **GIVEN** a comment whose `llm_attempts` has reached `LLM_MAX_ATTEMPTS` and whose last failure was transient
- **WHEN** the processor claims unprocessed comments
- **THEN** the comment SHALL be permanently excluded from claiming (dead-lettered) without any further provider calls

#### Scenario: Parked batch is logged
- **WHEN** the last attempt of a batch fails and its comments cross the attempt cap
- **THEN** the system SHALL emit a structured `event=batch_parked` log line naming the channel, comment IDs, and failure reason

#### Scenario: Hybrid partial failure requeues only missing comments
- **GIVEN** `JEV_CLASSIFICATION_ENABLED=true` and a batch where the judge leg succeeds for all indexes but the extractor leg misses one index
- **WHEN** the hybrid returns the merged result with that index missing
- **THEN** the processor SHALL persist the valid merged results and requeue only the missing index through the bounded-retry path
