# LLM Processing Specification

## Purpose
Classifies unprocessed comments using the OpenCode LLM provider and generates weekly digests. OpenCode is the sole provider for all LLM operations.

## Requirements

### Requirement: Batch Comment Classification
The system SHALL process unprocessed comments in batches through the OpenCode LLM provider, extracting category, topic, confidence, and actionable insights.

#### Scenario: Successful Batch Processing
- **GIVEN** unprocessed comments in the database
- **WHEN** the processor ticks (default every 10 seconds)
- **THEN** the system SHALL claim up to 50 comments per channel (FOR UPDATE SKIP LOCKED), send them to the OpenCode provider, apply confidence threshold (default 0.7), persist classification results (category, topic, confidence, actionable_insights), increment the primary token holder's quota usage, and mark the job completed

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
The system SHALL generate weekly digests using the OpenCode provider.

#### Scenario: Digest worker uses OpenCode
- **GIVEN** `DIGEST_WORKER_ENABLED=true` and OpenCode is configured
- **WHEN** the weekly digest cron triggers (Monday 08:00 UTC)
- **THEN** the system SHALL call OpenCode's `GenerateDigestRaw` method to produce digest content

#### Scenario: Digest retry chain uses OpenCode
- **GIVEN** an OpenCode transient error during digest generation
- **WHEN** the digest worker's retry chain executes (3 attempts at 0m, +5m, +20m)
- **THEN** each retry SHALL use the same OpenCode client with its built-in retry-with-backoff logic

### Requirement: Overage Tracking
The system SHALL log overage usage when a batch crosses the quota boundary for overage-tracked plans.

#### Scenario: Overage Row Insertion
- **GIVEN** a user on Creator or Studio plan whose comments_used_this_period + batch_size exceeds their quota
- **WHEN** the processor persists the batch
- **THEN** the system SHALL insert a comment_usage_logs row with the delta beyond the quota, the period start/end boundaries, and log the event

#### Scenario: No Overage for Free/Agency
- **GIVEN** a user on Free or Agency plan
- **WHEN** the processor processes a batch
- **THEN** the system SHALL NOT insert overage rows (Free is below-quota by definition; Agency uses custom pricing)

### Requirement: Graceful Shutdown
The system SHALL complete in-flight batch persistence before shutting down.

#### Scenario: Graceful Shutdown with In-Flight Batches
- **GIVEN** the processor is shutting down (context cancelled)
- **WHEN** there are in-flight Tx2 persistence operations
- **THEN** the system SHALL wait up to ShutdownGrace (default 30 seconds) for them to complete before exiting

### Requirement: Custom Prompt Per Channel
The system SHALL allow users to customize the LLM classification prompt for specific channels.

#### Scenario: Set Custom Prompt
- **GIVEN** an authenticated user who tracks a channel
- **WHEN** the user calls PATCH /api/channels/{id}/custom-prompt with `{prompt: "Focus on product feedback and feature requests."}`
- **THEN** the system SHALL validate the prompt (max 500 chars, no HTML/script tags), store it in user_channel_tracking.custom_prompt, and return 200

#### Scenario: Custom Prompt Validation - Too Long
- **GIVEN** an authenticated user
- **WHEN** the user submits a custom prompt longer than 500 characters
- **THEN** the system SHALL return 400 with "custom prompt must be 500 characters or less"

#### Scenario: Custom Prompt Validation - HTML Injection
- **GIVEN** an authenticated user
- **WHEN** the user submits a custom prompt containing HTML tags or script elements
- **THEN** the system SHALL return 400 with "custom prompt must not contain HTML or script tags"

#### Scenario: Clear Custom Prompt
- **GIVEN** a channel with a custom prompt set
- **WHEN** the user calls PATCH /api/channels/{id}/custom-prompt with `{prompt: ""}`
- **THEN** the system SHALL clear the custom_prompt and return 200

### Requirement: Thread-Aware Classification
The system SHALL include parent comment context when classifying reply comments.

#### Scenario: Classify Reply with Parent Context
- **WHEN** the processor classifies a comment with parent_comment_id != NULL
- **THEN** the system SHALL include the parent comment text (truncated to 200 chars) in the classification prompt as context

#### Scenario: Classify Top-Level Comment
- **WHEN** the processor classifies a comment with parent_comment_id = NULL
- **THEN** the system SHALL classify without parent context (existing behavior)
