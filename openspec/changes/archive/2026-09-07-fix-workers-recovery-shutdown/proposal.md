## Why

Background workers have recovery and shutdown defects that cause permanent starvation, lost digests, and dirty teardown:

1. **Stuck 'running' jobs starve channels permanently** — ingestion jobs are claimed atomically (`internal/db/ingestion.go:155-178`) but nothing reaps jobs left `running` by a crash/shutdown; the enqueue-dedupe gate (`status IN ('pending','running')`) then blocks that channel **forever** until manual DB fix. Same class of bug in `internal/db/llm_jobs.go:20-27` (`processing` stuck; plus no dedupe → duplicate LLM jobs on retry).
2. **Deferred digests never retried** — `digest_worker.go:302-324` marks LLM-failed digests `deferred` after 3 attempts, but nothing re-dispatches them (only a startup sweep for stale `pending`).
3. **Missed weekly digests lost** — `digest_worker.go:178-183` triggers on the exact minute; downtime across Mon 08:00 UTC loses the week's digest with no catch-up.
4. **Digest dispatch blocks the run loop** — `digest_worker.go:198-244` serializes N users × 25min retry chains through 8 workers, can take hours; no per-user dispatch deadline.
5. **Workers race DB teardown on shutdown** — `cmd/api/main.go:161-327`: every worker started on `context.Background()`; SIGTERM stops only HTTP + telemetry; workers keep running into `defer db.CloseDB()`.
6. **No panic recovery in LLM processor** — `processor.go:56-76` lacks the `recover` the ingestion worker has; a panic kills the whole process.
7. **Duplicate LLM spend on failure** — `processor.go:195-200`: LLM failure leaves comments unprocessed → re-claimed next tick → paid LLM calls repeat every 10s on same comments; `inflight.Add` (`processor.go:206-207`) wraps only persistBatch, not the LLM call, so shutdown grace misses it.
8. **Expired primary token preferred** — `channel_mgmt.go:382-397` picks by `token_priority ASC` without checking `token_expires_at`; an expired primary keeps being used until explicitly promoted.
9. **Email claim is SELECT-only** — `email.go:117-139` `ClaimPendingEmails` doesn't atomically claim → two workers double-send.
10. **Backoff has no jitter** — `ingestion.go:203-209` `2^retry` capped 16s with zero jitter → retry storms in lockstep.

## What Changes

- Add a reaper (startup + periodic) that resets stale `running`/`processing` jobs to `pending` for ingestion and LLM job queues; add LLM job dedupe.
- Re-dispatch `deferred` digests; add a startup catch-up scan for missed week-start windows.
- Bound per-user digest dispatch with a deadline; keep retries from blocking the loop.
- Single cancellable shutdown context wired to all workers; teardown order: workers → HTTP server → telemetry flush.
- `recover` per tick in the LLM processor; `inflight.Add` before the LLM call so graceful shutdown waits for it.
- Persist/refresh token after OAuth refresh so the worker doesn't re-refresh every poll; select tokens by expiry (prefer unexpired).
- Atomic email claim (UPDATE…RETURNING).
- Add jitter to retry backoff.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `ingestion`: Jobs recover from crashes; token selection respects expiry.
- `weekly-digest`: Missed or deferred digests are eventually dispatched.
- `observability-metrics`: Graceful shutdown drains workers before telemetry flush.

## Impact

- `internal/db/ingestion.go`, `internal/db/llm_jobs.go`, `internal/db/email.go`, `internal/db/channel_mgmt.go`
- `internal/ingestion/worker.go`, `internal/llm/processor.go`, `internal/llm/digest_worker.go`, `internal/email/retry_worker.go`
- `cmd/api/main.go` (shutdown wiring)
- Tests: `internal/ingestion/worker_test.go`, `internal/llm/*_test.go`, `internal/email/*_test.go`
