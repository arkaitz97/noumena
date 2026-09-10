## Context

The LLM processor claims comments grouped by channel and originally walked the channels **serially**, so a tick with K channels cost K × provider-latency. A first refinement ran the tick's batches concurrently but kept a per-tick barrier (`wg.Wait()` for all claimed batches before the next claim). That made throughput ≈ the *slowest* batch per wave: a fast channel idles until a slow sibling finishes, and with only two channels the second `MaxConcurrency` slots are unused.

The barrier was not incidental — it protected against duplicate work. The claim `GetUnprocessedCommentsByChannel` (`internal/db/comments.go:107`) is a plain `SELECT` with no durable reservation, and `EnqueueLLMJobIfNotExists` (`internal/db/llm_jobs.go:26`) dedupes on the **exact** `comment_ids` JSON string. Re-claiming while a batch is in flight returns the same still-unprocessed rows; `ORDER BY posted_at LIMIT` can yield overlapping-but-different sets, which bypass the exact-match dedupe and duplicate LLM calls.

Relevant constraints already in the codebase:

- The LLM call is already made **outside** any DB transaction (`classify`), by design.
- `db.SetMaxOpenConns(1)` (`internal/db/db.go:61`) serializes all DB access through one connection, with `busy_timeout(5000)`.
- `failoverClient` is explicitly mutex-guarded and documented as safe to share across workers (`internal/llm/failover.go:24-35`).
- `Processor.inflight sync.WaitGroup` exists to track in-flight batches for graceful shutdown.
- SQLite ships JSON1 (`json_each` verified against `modernc.org/sqlite`), so `llm_processing_jobs.comment_ids` (a JSON array) can drive a durable in-flight exclusion in the claim query.

## Goals / Non-Goals

**Goals:**
- Keep up to `LLM_MAX_CONCURRENCY` per-channel batches in flight continuously (default 4); a worker that finishes a batch immediately takes the next, never waiting for a slower sibling.
- Make the claim **durable** so continuous/rapid reclaiming is safe (no duplicate classification).
- Preserve all existing per-batch semantics: billing guard, retry/park, partial acceptance, overage, telemetry, and graceful shutdown.
- Keep serial behavior available (`LLM_MAX_CONCURRENCY=1`) and make rollback a config-only change.
- Add a gauge so operators can see effective in-flight batch count.

**Non-Goals:**
- Parallelizing **within** a batch (a batch is already one LLM request) or raising `LLM_BATCH_SIZE`.
- Parallelizing the weekly digest or opening a second DB connection.
- Changing quota semantics to strongly consistent admission control.

## Decisions

### D1 — Continuous worker pool, not a per-tick barrier

`Run` starts `MaxConcurrency` long-lived workers and one claimer. A `permits` channel pre-filled with `MaxConcurrency` tokens bounds reservations: the claimer consumes one permit per reserved batch and the worker returns it when that batch finishes. This means at most `MaxConcurrency` batches are ever reserved (`llm_processing_jobs` rows) or logged as `event=batch_started`, regardless of how many channels the claim query returns. A freed worker takes the next reserved batch immediately. `tickOnce` (claim-once + barrier) is retained only as a deterministic test seam — production does not use it.

- **Why:** removes the "slowest batch gates the wave" behaviour, and prevents the claimer from reserving a whole multi-channel result set up front (which produced more `batch_started` logs than the concurrency limit).
- **Alternative rejected:** deleting `wg.Wait()` without making the claim durable — unsafe (duplicate LLM work, see Context).
- **Alternative rejected:** fixed number of independent claim-and-process workers — concurrent plain `SELECT`s race on the same unprocessed rows.
- **Alternative rejected:** leaving reservations unbounded and only limiting execution — workers would still create `MaxConcurrency`×N job rows and misleading start logs.

### D2 — Durable claim via `llm_processing_jobs` exclusion

The claim query excludes any comment reserved by a `pending`/`processing` job:

```sql
AND NOT EXISTS (
  SELECT 1 FROM llm_processing_jobs j,
       json_each(CASE WHEN json_valid(j.comment_ids) THEN j.comment_ids ELSE '[]' END) je
   WHERE j.status IN ('pending','processing') AND je.value = c.id)
```

`NOT EXISTS` with equality (rather than `c.id NOT IN (subquery)`) is deliberate: a single reservation row whose JSON yields a NULL (e.g. `comment_ids = 'null'`) would make `NOT IN` evaluate to NULL for every row and suppress the entire claim — a global processing stall. Invalid JSON is likewise neutralised via `json_valid`. Reservation happens in the claimer (not the worker): `claimBatches`/`claimLoop` resolve the primary, call `enqueueLLMJobDeduped` and mark the job `processing`, then dispatch. Because a single claimer performs `SELECT` → `enqueue` → next `SELECT`, there is no intra-process race. The exact-set dedupe remains a backstop for the multi-instance case.

- **Why:** makes reclaiming safe without a barrier, and matches the project's Job Queue pattern.
- **Migration:** no schema change — `comment_ids` already exists as a JSON array; JSON1 is built in.

### D3 — DB access stays safe via the single-connection pool

`SetMaxOpenConns(1)` serializes reads and writes at statement granularity, so concurrent workers cannot corrupt state; they only queue briefly. The long-running provider call holds no connection. `IncrementCommentsUsed` remains atomic per call.

- **Trade-off accepted:** the pre-batch quota guard is advisory (`requireActivePrimary`). With concurrency, multiple batches can pass the guard before any increments, so a batch may cross the quota boundary. `logOverage` already handles boundary crossing; not a regression.

### D4 — `inflight` spans the whole batch lifecycle

`runBatchGuarded` wraps `processClaimedBatch` in `inflight.Add(1)`/`Done()`; `shutdown()` waits on the same WaitGroup bounded by `ShutdownGrace`. `Run` closes the jobs channel, then calls `shutdown()` and `wg.Wait()` so no worker outlives Run. On `ctx` cancellation, the claimer stops reserving and the jobs channel closes; in-flight calls are bounded by `BatchTimeout`.

### D5 — Config surface

`LLMMaxConcurrency int` on `config.Config` (default 4), parsed from `LLM_MAX_CONCURRENCY`, clamped to `>= 1`, assigned to `proc.MaxConcurrency` in `startLLMProcessor` (`cmd/api/main.go`). Documented in `.env.example`. `1` reproduces serial behaviour.

### D6 — Observability

Gauge `noumena_llm_batches_in_flight` (increment when a worker starts a batch, decrement when it ends) makes effective concurrency visible without new high-cardinality labels.

## Risks / Trade-offs

- **Provider rate limits / 429 bursts** → conservative default (4), bounded workers, existing transient retry-with-backoff and failover. Operators can lower `LLM_MAX_CONCURRENCY`.
- **Failover trips sooner** — `failoverClient` counts *consecutive* errors; simultaneous failures can reach the threshold quickly and begin the permanent fallback flip. Mitigation: mutex serializes state; document; if noisy, raise `LLM_FAILOVER_ERROR_THRESHOLD` or reduce concurrency (follow-up).
- **Claim/enqueue window** → the JSON1 exclusion only reflects jobs that exist; the single claimer closes the intra-process gap, and exact-set dedupe backstops multi-instance races. Residual window is negligible and never duplicates a *persisted* result.
- **JSON1 dependency** → verified available in `modernc.org/sqlite`; the claim query fails loudly if a build ever drops it.
- **SQLite write contention** → all DB work queues on one connection; LLM latency dominates and statements are short; `busy_timeout(5000)` absorbs bursts.
- **Quota over-crossing under concurrency** → advisory guard, overage logging unchanged; billing charge is per persisted comment.
- **Test determinism** — add goroutine-safe fakes: a blocking provider asserting the concurrency bound, a scripted provider asserting immediate refill, and a DB test asserting the in-flight exclusion.
- **Graceful shutdown latency** → bounded by `ShutdownGrace`; in-flight provider calls bounded by `BatchTimeout`.

## Migration Plan

1. Ship code with `LLM_MAX_CONCURRENCY` defaulting to 4; deploy is additive (no schema/API change).
2. Rollback = set `LLM_MAX_CONCURRENCY=1` (no code change) or revert the change set.
3. Validate on a staged backlog: ticks drain faster, a channel's next batch starts as soon as its previous one finishes, no duplicate/`batch_deduped` spike, failover counter does not spuriously flip.

## Open Questions

- Should the failover threshold become concurrency-aware (count error *rates* rather than raw consecutive failures)? Deferred; follow-up if burst-flips appear.
- Should the digest worker get the same concurrency treatment? Out of scope; digests are one call per user per week.
