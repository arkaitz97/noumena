## Context

Workers are goroutines with tick loops over SQLite-backed job queues. Claim semantics are atomic (`UPDATE…WHERE…RETURNING`), but crash recovery is absent: a process killed mid-job leaves `running`/`processing` rows that enqueue-dedupe treats as active → permanent starvation. Digest worker has no catch-up. Shutdown uses `context.Background()` per worker, so `defer db.CloseDB()` runs under live workers.

## Goals / Non-Goals

**Goals:**
- Crashed jobs recover automatically (reaper).
- Digests are eventually delivered even after downtime.
- Clean, ordered shutdown with no worker/DB race and no lost telemetry.
- No duplicate LLM spend or double email send.

**Non-Goals:**
- Not introducing a distributed queue (SQLite-backed single-writer stays).
- Not changing worker tick intervals beyond what recovery requires.
- Not implementing leader election (single-instance deploy model).

## Decisions

### 1. Reaper sweep

A `recoverStaleJobs` helper run at startup + on a tick (e.g. every 5m):
- ingestion_jobs: `status='running' AND started_at < now-15m` → `status='pending'`, clear started_at.
- llm_processing_jobs: same for `processing`.
- data_export_jobs: `exporting` stale → retryable (see export change).
Guard: active workers refresh `started_at` (heartbeat) or the sweep is scoped to rows whose `started_at` is older than the max plausible job duration. Add LLM job dedupe at enqueue (skip if a pending/running job already covers the comment set).

### 2. Digest recovery

- Startup catch-up: scan `weekly_digests` for `week_start` in the past where status is `none`/`pending` but `comments_count >= 10` → enqueue. 
- Re-dispatch: treat `deferred` as retryable with a backoff schedule (e.g. retry next day, then stop after N) so transient LLM outages don't permanently drop a week.
- Per-user dispatch deadline (e.g. 60s) so one stuck user can't stall the run loop; retries become async items rather than blocking.

### 3. Shutdown ordering

Introduce a root `ctx` canceled on SIGTERM/SIGINT. Wire every worker's `Start(ctx)`. Sequence on signal: cancel root ctx → workers observe cancellation and drain (bounded wait) → stop HTTP server → `otelShutdown` flush after workers confirm stop → `db.CloseDB()`. Add a `sync.WaitGroup` per worker or a coordinator that waits.

### 4. Processor panic safety + inflight coverage

Mirror ingestion worker's per-tick `recover`: log + metric, continue loop. Move `inflight.Add(1)` to *before* the LLM call (not just persistBatch) so graceful shutdown waits for in-flight LLM requests; `inflight.Done` deferred.

### 5. Token selection + persistence

`GetIngestionTokenRow` ordering: prefer `token_expires_at > now`, then `token_priority ASC`. After a forced refresh (`worker.go:443`), persist the refreshed access/expiry (same update path used at connect) so each poll doesn't re-refresh and revoked-token windows shrink.

### 6. Atomic email claim

`ClaimPendingEmails` becomes `UPDATE email_log SET status='sending', claimed_at=? WHERE id IN (SELECT id FROM email_log WHERE status='pending' LIMIT ?) RETURNING …` (single writer makes this safe). Two-worker double-send eliminated; `retry_worker`/`pipeline` share the claim.

### 7. Backoff jitter

`backoff = base * 2^retry` → `+ rand.Int63n(base)` or full-jitter; keeps bursty channels from retrying in lockstep.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Reaper races an active slow job | Heartbeat/`started_at` age threshold + only sweep older than max job duration; log every reset |
| Catch-up floods LLM/email after long downtime | Throttle catch-up (e.g. N per tick) and respect existing per-user deadlines |
| Shutdown waits too long | Bounded drain (e.g. 30s) then force-stop workers; telemetry flush last with short timeout |
| Token persistence writes on every poll | Only write when token actually refreshed/expiry near |
