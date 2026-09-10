# Design: Fix LLM duplicate processing (bounded retry cost)

## Context

The processor (`internal/llm/processor.go`) claims up to `BatchSize` unprocessed comments per tick (10s), calls one provider client per batch, and persists. Failure evidence from `llm_processing_jobs` (2026-09-08→10): 192 failed / 62 completed jobs; the worst comment set retried 89× (~247s per cycle = 4 HTTP attempts × the hardcoded 60s `http.Client.Timeout` + 1s/2s/4s backoff). Three independent defects compound:

1. Client-level 60s timeout aborts non-streaming generations the provider still completes and bills; `BatchTimeout=300s` never applies because every attempt dies at 60s first.
2. Transport/timeout errors are retryable (`retry.go`) → 4 billed generations per job, and job-level failure re-enqueues the identical set every tick — dedupe (`llm_jobs.go`) only blocks `pending/processing`, and `retry_count` is written but never read.
3. A single invalid result index discards an otherwise good batch (`persistBatch`), converting a 1/50 problem into a 50/50 re-bill.

Constraints: SQLite single-writer, MVP per-statement persistence (no cross-statement Tx accessor on `DBQuery`), existing env-flag config style, spec-driven repo (`openspec/specs/llm-processing`).

## Goals / Non-Goals

**Goals:**
- Bound worst-case provider calls per comment set to `LLM_MAX_ATTEMPTS` (default 3) × in-job HTTP attempts.
- Stop aborting generations that would have succeeded: one timeout budget, sized to fit real latency.
- Never re-pay for valid partial results.
- Make dead-letter state visible in logs/metrics and operable via SQL.
- Failover to fallback provider that actually triggers.

**Non-Goals:**
- Streaming (SSE) provider responses — right long-term fix, deferred to a follow-up change.
- Per-comment cost accounting / price cards — owned by `llm-cost-analytics` change.
- UI surfacing of parked comments; admin API for resetting them.
- Multiple processor instances / horizontal scaling semantics.

## Decisions

### D1. One timeout budget: ctx owns it, clients obey
Remove `Timeout` from `http.Client` in `NewOpenCodeClient`/`NewOpenAIClient`/`NewAnthropicClient` (rely on `Transport` defaults + the request ctx). `retryTransient` derives each attempt's deadline as `remaining_ctx / remaining_attempts` (adaptive split of the classify-level `BatchTimeout`) and passes it via `context.WithTimeout`; the server's `Retry-After` still wins when shorter than the share.

*Alternative considered:* raise the client timeout to 300s — rejected; two overlapping budgets is exactly this incident, and it starves backoff math. Streaming — rejected for scope (SSE parsing differs per provider), tracked as non-goal.

### D2. Retry state lives on comments, not job sets
Add to `comments`: `llm_attempts INTEGER NOT NULL DEFAULT 0`, `llm_next_retry_at TEXT` (NULL = eligible). The claim query (`GetUnprocessedCommentsByChannel`) gains `AND llm_attempts < ? AND (llm_next_retry_at IS NULL OR llm_next_retry_at <= now)`. On job failure every comment in the batch gets `llm_attempts = llm_attempts + 1` and `llm_next_retry_at = now + backoff[new_attempts]` (schedule `1m,5m,15m` → effectively parked after `LLM_MAX_ATTEMPTS=3`). Comments that reach the cap are dead-lettered by definition (claim-excluded) — no separate parked table.

*Alternative considered:* `set_key` hash + attempt chain on `llm_processing_jobs` — rejected: batches mutate whenever a new comment ingests (set payloads stop matching, proven by the 89-retry set only matching while the claim set was stable), so per-set accounting is fragile; per-comment state is stable under regrouping and reuses the existing claim path. Backfill migration: set `llm_attempts = LLM_MAX_ATTEMPTS` for comments that appear in ≥ `LLM_MAX_ATTEMPTS` historical failed payloads in-flight at deploy? No — default 0 (fresh budget); the boot reaper already fails their orphaned jobs; the new caps stop any loop within 3 attempts post-deploy.

### D3. Explicit error taxonomy: terminal vs transient
Export `type TerminalError` + `IsTerminal(err)` in `internal/llm`. Classification: HTTP 4xx except 429, JSON decode/parse/invalid-category errors, auth errors → terminal (abort in-job retries immediately — already true — and additionally skip the remaining attempt budget: processor parks straight to the cap: `llm_attempts = LLM_MAX_ATTEMPTS`). Transport, 429, 5xx, ctx-deadline → transient (backoff schedule applies). `failBatch` consults `IsTerminal`.

*Alternative:* re-derive class at the processor by string-matching error messages — rejected, brittle and already the de-facto state.

### D4. Partial-result acceptance
`ParseClassificationResponse` stops aborting on a bad entry: valid-index results are kept, offending indexes collected into a `MissingIndexes []int` returned alongside. `persistBatch` persists valid entries (each is one `UpdateCommentLLMData` + sentiment write, as today), then for missing/invalid indexes: those comments get transient-failure retry state (D2/D3), the job row is marked `failed` with the per-index error, and `IncrementCommentsUsed` charges only the persisted count. If zero valid entries, behavior equals full failure. A comment that later succeeds on requeue is billed once — quota increments happen only in the persist path.

### D5. Failover = consecutive-failure counter
Replace the 5-in-60s sliding window with: `consecutive >= LLM_FAILOVER_ERROR_THRESHOLD` (default 3, env `LLM_FAILOVER_WINDOW_SECONDS` deprecated/ignored with boot warning) flips permanently (unchanged post-flip semantics). Success resets the counter. The old window could never fill because errors arrive ≥247s apart — a counter is timing-independent.

### D6. Config defaults
`LLM_BATCH_SIZE` default 50 → 15 (smaller batches fit latency and shrink blast radius of any requeue); `.env.example` batch timeout note updated to state BatchTimeout is the total budget across attempts. New: `LLM_MAX_ATTEMPTS`, `LLM_RETRY_BACKOFF` (CSV durations). All wired through `internal/config` in existing env style.

### D7. Telemetry
New counters in `internal/llm/metrics.go`: `noumena_llm_job_attempts_total{provider, outcome=completed|failed_transient|failed_terminal|parked}` and `noumena_llm_provider_calls_total{provider, retryable}` (incremented per HTTP attempt inside `retryTransient` — this is the duplicate-work canary: ratio to distinct jobs ≫ 1.2 pages a human). Structured log `event=batch_parked channel_id=%s comment_ids=%s reason=%s`. Dead-letter depth gauge `noumena_llm_parked_comments` = count of eligible-excluded unprocessed comments (cheap count, sampled per tick).

## Risks / Trade-offs

- Dead-lettered comments silently stall until ops acts → mitigated by `event=batch_parked`, parked gauge, and a runbook section with the un-park SQL (`UPDATE comments SET llm_attempts=0, llm_next_retry_at=NULL WHERE ...`).
- Removing the client timeout risks a hung connection pinning the tick loop → mitigated: every attempt carries a ctx deadline (D1), plus `Transport` dial/TLS timeouts remain on the shared default transport.
- Per-comment retry state charges transient provider outages against the comment's attempt budget → accepted: 3 attempts with growing backoff is generous for blips; failover (D5) mitigates provider-wide outages.
- Partial persistence breaks batch-quota atomicity further (comments persisted while job marked failed) → accepted: it already is per-statement MVP behavior; quota increments exactly match persisted comments, so billing stays correct.
- Smaller default batch increases provider call count for the same comment volume (~3×) while cutting tokens-per-call risk; net cost direction is strongly negative for the observed failure mode — monitored via D7 ratio.

## Migration Plan

1. Additive columns via versioned migration (`llm_attempts`, `llm_next_retry_at`, DEFAULTs keep old semantics on existing rows). No data rewrite.
2. Deploy binary with new defaults; keep `LLM_BATCH_SIZE` env override available for instant revert to 50.
3. Observe `noumena_llm_provider_calls_total` ratio and `event=batch_parked` for 24h.
4. Rollback = redeploy previous binary; new columns are ignored by it (nullable/defaulted), so no schema revert needed.

## Open Questions

- Backoff schedule defaults (`1m,5m,15m`) — confirm against opencode.ai p99 generation latency after first week of metrics.
- Whether parked comments should eventually surface in the dashboard (separate future change).
- SSE streaming change to eliminate client-abort billing entirely — file follow-up proposal after this lands.
