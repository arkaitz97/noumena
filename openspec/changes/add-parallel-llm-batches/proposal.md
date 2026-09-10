# Proposal: Parallel LLM Batch Processing

## Why

The LLM processor claims every channel's pending batch in one tick but then walks the channels **serially** (`internal/llm/processor.go:164-169`), awaiting each LLM round-trip before starting the next. A backfill across many channels therefore serializes N network-bound generations back to back, so a large ingestion backlog drains at ~1 batch per provider latency. The LLM call is I/O-bound and spends most of its wall-clock waiting — concurrency across channels is the cheapest throughput win available, and horizontal scaling (more process instances) is unavailable in the single-process self-hosted deployment.

## What Changes

- **Continuous worker pool:** `Run` keeps `LLM_MAX_CONCURRENCY` channel batches in flight (default 4). A worker that finishes a batch immediately takes the next; a fast channel is never blocked behind a slow one.
- **Bounded reservations:** a permit is consumed per reserved batch and returned on completion, so at most `LLM_MAX_CONCURRENCY` batches are ever reserved (`llm_processing_jobs` rows) or started (`event=batch_started`) at once — regardless of how many channels the claim query returns.
- **Durable claim:** `GetUnprocessedCommentsByChannel` excludes comments referenced by a `pending`/`processing` job (JSON1 `json_each` over `comment_ids`), so a continuously-claiming processor cannot re-select in-flight work. Reservations happen at claim time via `reserveBatch`.
- **Config:** new `LLM_MAX_CONCURRENCY` env var (default 4; `1` restores serial behavior). Plumbed through `config.Config` and the processor construction path.
- **Graceful shutdown:** the `inflight` WaitGroup spans the whole batch lifecycle so shutdown waits for all in-flight batches (bounded by `ShutdownGrace`).
- **Observability:** in-flight/active-batch gauge; per-batch job-attempt and token metrics keep their existing labels.
- **Provider safety:** concurrency is capped and documented; the failover error window now observes simultaneous errors and may trip sooner under load (trade-off, called out in design).

## Capabilities

### New Capabilities
<!-- None: this modifies the existing processing behavior, no new capability surface. -->

### Modified Capabilities
- `llm-processing`: batch classification runs through a bounded continuous worker pool with a durable in-flight claim, instead of a barrier-gated serial/concurrent tick.
- `channel-management`: resuming a paused channel restores a priority-0 primary so the channel becomes claimable again.

## Impact

- `internal/llm/processor.go` — continuous pool, reservation permits, `reserveBatch`, shutdown accounting, concurrency gauge.
- `internal/db/comments.go` — NULL-safe in-flight exclusion in the claim query (JSON1, no schema change).
- `internal/db/channel_mgmt.go` — `ResumeTracking` restores the priority-0 primary when none remains.
- `internal/config/config.go` + `.env.example` — `LLM_MAX_CONCURRENCY` (default 4).
- Processor wiring (`cmd/api/main.go`) — pass the concurrency limit.
- Tests — `internal/llm/processor_test.go` (bounded reservations, immediate refill, end-to-end drain), `internal/db/comments_retry_test.go` (in-flight exclusion, NULL-safe claim, resume visibility).
- No schema migration, no new dependencies, no API/frontend changes. DB writes stay serialized by SQLite as today.
