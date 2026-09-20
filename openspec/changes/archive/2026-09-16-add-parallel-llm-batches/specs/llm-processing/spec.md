## ADDED Requirements

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
