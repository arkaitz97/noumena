## MODIFIED Requirements

### Requirement: LLM Provider Failover

**Reason**: Removed because OpenCode is now the sole provider; failover is unnecessary.
**Migration**: Failover logic has been removed entirely. Per-provider retry logic in `opencodeClient` handles transient errors. If multi-provider support is needed in the future, the failover client can be re-added from git history.

### Requirement: Multi-Provider Support

**Reason**: Consolidated to single OpenCode provider.
**Migration**: OpenCode is now the only supported provider. Configuration via `OPENCODE_API_KEY`, `OPENCODE_BASE_URL`, and `OPENCODE_MODEL`.

### Requirement: Batch Comment Classification

The system SHALL process unprocessed comments in batches through the OpenCode LLM provider.

#### Scenario: Successful Batch Processing
- **GIVEN** unprocessed comments in the database
- **WHEN** the processor ticks (default every 10 seconds)
- **THEN** the system SHALL claim up to 50 comments per channel, send them to the OpenCode provider, apply confidence threshold (default 0.7), persist classification results (category, topic, sentiment, confidence), increment the primary token holder's quota usage, and mark the job completed

#### Scenario: Confidence Threshold Downgrade
- **GIVEN** an LLM result with confidence below 0.7
- **WHEN** the processor applies the confidence threshold
- **THEN** the system SHALL downgrade the category to "other"

#### Scenario: No Primary Tracker
- **GIVEN** a batch of comments for a channel with no active primary tracker
- **WHEN** the processor attempts to resolve the primary token holder
- **THEN** the system SHALL skip the batch without writing any results

#### Scenario: Paused User Batch Skip
- **GIVEN** a batch where the primary token holder's subscription_status is 'paused'
- **WHEN** the processor checks billing guard
- **THEN** the system SHALL skip the entire batch without processing

#### Scenario: Quota Exceeded Batch Skip
- **GIVEN** a batch where the primary token holder has used their full comment quota
- **WHEN** the processor checks billing guard
- **THEN** the system SHALL skip the entire batch without processing

### Requirement: Graceful Shutdown

The system SHALL complete in-flight batch persistence before shutting down.

#### Scenario: Graceful Shutdown with In-Flight Batches
- **GIVEN** the processor is shutting down (context cancelled)
- **WHEN** there are in-flight Tx2 persistence operations
- **THEN** the system SHALL wait up to ShutdownGrace (default 30 seconds) for them to complete before exiting

## ADDED Requirements

### Requirement: OpenCode as Sole LLM Provider

The system SHALL use OpenCode as the exclusive LLM provider for all operations including comment classification and weekly digest generation.

#### Scenario: OpenCode initialized at startup
- **GIVEN** `OPENCODE_API_KEY` is configured and `LLM_PROCESSOR_ENABLED=true`
- **WHEN** the application starts
- **THEN** the system SHALL create a single `*opencodeClient` instance and pass it to both the batch processor and digest worker

#### Scenario: OpenCode configuration validation
- **GIVEN** `OPENCODE_API_KEY` is empty when `LLM_PROCESSOR_ENABLED=true`
- **WHEN** the application starts
- **THEN** the system SHALL fail startup with a clear configuration error message

### Requirement: Digest generation via OpenCode

The system SHALL generate weekly digests using the OpenCode provider, replacing the legacy direct-HTTP OpenAI client.

#### Scenario: Digest worker uses OpenCode
- **GIVEN** `DIGEST_WORKER_ENABLED=true` and OpenCode is configured
- **WHEN** the weekly digest cron triggers (Monday 08:00 UTC)
- **THEN** the system SHALL call OpenCode's `GenerateDigestRaw` method to produce digest content

#### Scenario: Digest retry chain uses OpenCode
- **GIVEN** an OpenCode transient error during digest generation
- **WHEN** the digest worker's retry chain executes (3 attempts at 0m, +5m, +20m)
- **THEN** each retry SHALL use the same OpenCode client with its built-in retry-with-backoff logic
