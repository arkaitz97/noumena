# LLM Processing Specification

## Purpose
Classifies unprocessed comments using the OpenCode LLM provider and generates weekly digests. OpenCode is the sole provider for all LLM operations.

## Requirements

### Requirement: Batch Comment Classification
The system SHALL process unprocessed comments in batches through the configured LLM provider, extracting category, topic, confidence, and actionable insights.

#### Scenario: Successful Batch Processing
- **GIVEN** unprocessed comments in the database
- **WHEN** the processor ticks (default every 10 seconds)
- **THEN** the system SHALL claim up to `LLM_BATCH_SIZE` comments per channel (default 15; atomic `UPDATE … RETURNING` on the SQLite-backed queue), send them to the provider, apply confidence threshold (default 0.7), persist classification results (category, topic, confidence, actionable_insights), increment the primary token holder's quota usage, and mark the job completed

#### Scenario: Only eligible comments are claimed
- **GIVEN** unprocessed comments with `llm_attempts >= LLM_MAX_ATTEMPTS`, or with a future `llm_next_retry_at`
- **WHEN** the processor builds a batch
- **THEN** those comments SHALL be excluded from the claim

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

### Requirement: Single generation timeout budget
The system SHALL bound each LLM classification call by one budget — the processor's `BatchTimeout` — shared across in-call HTTP attempts, and SHALL NOT impose any shorter per-attempt timeout at the HTTP client level.

#### Scenario: Slow but healthy generation completes
- **GIVEN** a batch whose provider generation takes 90 seconds
- **WHEN** the processor calls `ClassifyBatch` with `BatchTimeout=300s`
- **THEN** no HTTP client timeout aborts the request at 60s and the classification is persisted

#### Scenario: Attempts split the remaining budget
- **GIVEN** a `BatchTimeout` of 300 seconds and the transient retry schedule (1 initial + 3 retries)
- **WHEN** `retryTransient` starts attempt *k*
- **THEN** the attempt receives a deadline of the remaining budget divided by the number of attempts still available (server `Retry-After` honored when it is shorter than that share)

#### Scenario: Whole budget exhausted
- **GIVEN** every attempt consuming its share without success
- **WHEN** the `BatchTimeout` deadline passes
- **THEN** the call returns a context-deadline error and no further attempts are made

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

#### Scenario: Operators requeue parked comments
- **GIVEN** parked comments (attempts at cap)
- **WHEN** an operator resets `llm_attempts` to 0 and clears `llm_next_retry_at` for those rows
- **THEN** the next processor tick claims and classifies them normally

### Requirement: Deterministic failures park immediately
The system SHALL classify provider errors as terminal or transient and SHALL NOT consume retry attempts or re-enqueue batches whose failure is terminal.

#### Scenario: Terminal error parks the batch
- **GIVEN** a batch whose call fails with a terminal error (HTTP 4xx other than 429, auth failure, or response parse error)
- **WHEN** the processor records the failure
- **THEN** every comment in the batch SHALL be dead-lettered immediately (`llm_attempts` set to the cap) with no backoff and no re-attempt

#### Scenario: Transient error keeps the retry ladder
- **GIVEN** a batch whose call fails with a transport, 429, 5xx, or context-deadline error
- **WHEN** the processor records the failure
- **THEN** the comments SHALL follow the bounded-retry attempt-and-backoff path

### Requirement: Partial classification results accepted
The system SHALL persist valid per-comment results from a partially valid batch response and requeue only the missing or invalid comments.

#### Scenario: Invalid index does not discard valid siblings
- **GIVEN** a batch of 50 comments whose response contains 49 valid results and 1 result with an invalid category value or omitted index
- **WHEN** the processor persists the batch
- **THEN** the 49 valid comments SHALL be written (category, topic, confidence, insights, sentiment), quota SHALL be incremented by 49 only, and the job marked failed naming the bad indexes

#### Scenario: Only the invalid comments consume an attempt
- **GIVEN** the partially failed batch above
- **WHEN** retry state is recorded
- **THEN** only the 1 missing/invalid comment SHALL have `llm_attempts` incremented and be eligible for requeuing

#### Scenario: All-invalid response is a full failure
- **GIVEN** a response with zero valid results
- **WHEN** the processor handles it
- **THEN** no comments SHALL be persisted and the batch SHALL follow the failure path in full

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

### Requirement: LLM provider failover
The system SHALL support a configurable primary and fallback LLM provider, switching to the fallback after a configurable number of consecutive classification failures.

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

### Requirement: Bounded concurrent batch processing
The processor SHALL keep multiple per-channel batches in flight continuously, up to a configurable limit `LLM_MAX_CONCURRENCY` (default 4), instead of awaiting batches serially or gating progress on the slowest batch of a wave. It SHALL reserve (start) no more than `LLM_MAX_CONCURRENCY` batches at once, regardless of how many channels are claimable. When a worker finishes a batch it SHALL immediately start the next reserved batch without waiting for other in-flight batches. When the limit is 1, processing SHALL be serial. Concurrent batches SHALL NOT classify the same comment twice.

#### Scenario: Multiple channels processed concurrently
- **GIVEN** more claimable channels than `LLM_MAX_CONCURRENCY`
- **WHEN** the processor has free workers
- **THEN** up to `LLM_MAX_CONCURRENCY` channel batches SHALL be sent to the provider concurrently rather than one after another

#### Scenario: Concurrency is bounded by configuration
- **GIVEN** ten claimable channels and `LLM_MAX_CONCURRENCY=3`
- **WHEN** the processor runs
- **THEN** at most three channel batches SHALL have an in-flight LLM call at any instant, with the remainder waiting for a free worker

#### Scenario: Reservations are bounded by configuration
- **GIVEN** nine claimable channels and `LLM_MAX_CONCURRENCY=4`, with every batch still running
- **WHEN** the processor starts work
- **THEN** only four batches SHALL be reserved (`llm_processing_jobs` rows) and started (`event=batch_started`), and the next reservation SHALL wait until a running batch finishes

#### Scenario: Free worker starts the next batch immediately
- **GIVEN** `LLM_MAX_CONCURRENCY=2`, one fast channel batch already in flight, and one slow channel batch in flight
- **WHEN** the fast batch completes
- **THEN** the next reserved batch SHALL start immediately, without waiting for the slow batch to finish

#### Scenario: Serial behavior preserved at limit 1
- **GIVEN** `LLM_MAX_CONCURRENCY=1`
- **WHEN** the processor processes multiple channel batches
- **THEN** each batch SHALL complete before the next begins (no overlap)

#### Scenario: No duplicate classification across concurrent batches
- **GIVEN** batches running concurrently while more comments remain unprocessed
- **WHEN** the processor reclaims work
- **THEN** every comment SHALL be classified at most once and persisted exactly once

### Requirement: Durable in-flight batch claim
The claim SHALL treat a comment as reserved while an `llm_processing_jobs` row referencing it is `pending` or `processing`, so a continuously-claiming processor can never re-select work that is still in flight. A job that completes or fails SHALL release its comments for normal reprocessing rules (processed comments are excluded; failed ones follow retry/backoff).

#### Scenario: In-flight comments are not reclaimed
- **GIVEN** a comment reserved by a `processing` job
- **WHEN** the processor claims the next batches before that job finishes
- **THEN** the reserved comment SHALL be excluded from the claim, while other eligible comments remain claimable

#### Scenario: Terminal job releases the comment
- **GIVEN** a comment whose reserving job has completed or failed
- **WHEN** the processor next claims
- **THEN** the comment SHALL no longer be excluded by the reservation (it is either already processed or eligible under retry/backoff rules)

#### Scenario: Crash recovery re-enables claiming
- **GIVEN** a job left `processing` by a crashed process, older than the orphan age
- **WHEN** the processor starts or sweeps orphans
- **THEN** the stale job SHALL be failed and its comments SHALL become claimable again

#### Scenario: Malformed reservation rows do not suppress claims
- **GIVEN** a `pending`/`processing` job whose `comment_ids` is JSON null or invalid JSON
- **WHEN** the processor claims
- **THEN** that row SHALL match no comment and SHALL NOT suppress the rest of the claim (a NULL element must never make the whole claim empty)

### Requirement: Concurrency-aware graceful shutdown
The processor SHALL wait for every in-flight channel batch — including concurrent LLM calls and their persistence — to finish before exiting, bounded by `ShutdownGrace`; context cancellation SHALL stop new batches from being reserved while in-flight batches are awaited.

#### Scenario: Shutdown waits for all concurrent batches
- **GIVEN** the processor is shutting down while up to `LLM_MAX_CONCURRENCY` batches are in flight
- **WHEN** the context is cancelled
- **THEN** no new channel batch SHALL start and the processor SHALL wait up to `ShutdownGrace` for all in-flight batches to complete

#### Scenario: Worker pool drains without leaks
- **GIVEN** a running worker pool with concurrency greater than 1
- **WHEN** the context is cancelled and the run returns
- **THEN** every worker goroutine SHALL have exited and no batch SHALL remain running

### Requirement: Batch cost persistence

The LLM processing subsystem SHALL persist token usage and derived cost for every processed batch and digest, following the cost-attribution rules in the `llm-cost-analytics` capability.

#### Scenario: Classification batch with tokens
- **WHEN** a batch is classified successfully
- **THEN** the system records input/output tokens and cost for the batch in the same transaction that persists classification results

#### Scenario: Digest generation
- **WHEN** a weekly digest is generated for a user
- **THEN** the system records the digest's token usage and cost against the user's billing period

