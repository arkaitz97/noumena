# Proposal: Fix LLM duplicate processing (bounded retry cost)

## Why

Production DB evidence: a single 50-comment batch was sent to the LLM provider 89 times (up to 4 HTTP attempts each) before completing — 192 failed vs 62 completed jobs in two days, an estimated ≤38k duplicate comment classifications billed. Root cause: hardcoded 60s `http.Client` timeouts abort slow non-streaming generations that the provider still completes and bills, and every failure re-enqueues the identical comment set on the next 10s tick with no attempt cap, no backoff, and no terminal class for deterministic failures (parse errors, 401, 400).

## What Changes

- **Per-attempt timeout aligned with batch budget:** remove the hardcoded 60s `http.Client.Timeout` in the opencode/openai/anthropic clients; the processor's `BatchTimeout` context becomes the single generation budget, split across attempts (each retry gets only its remaining share).
- **Bounded attempts per comment set:** failed jobs no longer re-enqueue indefinitely. Per-comment-set attempt counter persisted across job rows; after N attempts (default 3) the set is parked as dead-letter and never re-claimed.
- **Exponential backoff between re-attempts:** re-enqueue of a failed comment set is gated by a `next_retry_at` timestamp honored by the claim query (1m, 5m, 15m schedule) instead of the next 10s tick.
- **Deterministic failures are terminal:** non-retryable errors (HTTP 4xx other than 429, JSON/parse errors, auth errors) park the batch immediately without consuming the full attempt budget.
- **Partial result acceptance:** when a classification response is valid but omits or corrupts individual comment indexes, persist the good results and requeue only the missing/invalid comments (bug today: one bad index discards all 49 good results and re-bills the whole batch).
- **Failover trigger fixed:** sliding-window error (5/60s) can never accumulate under the new timing; switch to a consecutive-failure counter with configurable threshold so the fallback provider actually engages.
- **Config defaults revised:** `LLM_BATCH_SIZE` default lowered 50 → 15 so a batch's generation fits typical provider latency; `.env.example` `LLM_PROCESSOR_BATCH_TIMEOUT` guidance updated accordingly.
- **Duplicate-work telemetry:** new metric for LLM provider calls per unique comment set (`noumena_llm_job_attempts_total{outcome}` + stale-loop counter) and a structured `event=batch_parked` log for dead-lettered sets.

No breaking API changes. Existing completed/failed job rows are left as-is; a startup migration seeds the attempt counter from current `failed` rows.

## Capabilities

### New Capabilities

_(none — all behavior fits under existing capabilities as modified requirements)_

### Modified Capabilities

- `llm-processing`: new requirements for bounded retries, backoff, dead-letter parking, terminal deterministic failures, partial-result acceptance, and per-attempt timeout budget; failover requirement changes from sliding window to consecutive-failure counter; batch-size default changes.
- `observability-metrics`: LLM Processing Metrics requirement gains per-attempt counters and dead-letter telemetry so duplicate-call loops are detectable and alertable.

## Impact

- `internal/llm/opencode.go`, `openai.go`, `anthropic.go` — HTTP client construction (drop 60s `Timeout`), per-attempt ctx budget.
- `internal/llm/retry.go` — attempt-budget split across retries; error-class plumbing (retryable vs terminal).
- `internal/llm/processor.go` — partial persist path, park/dead-letter flow, backoff-aware skip.
- `internal/llm/failover.go` — consecutive-failure trigger, config knobs.
- `internal/db/llm_jobs.go`, `internal/db/comments.go`, schema + migration — `retry_count`/`next_retry_at` enforcement, parked state, claim query changes.
- `internal/config/config.go`, `.env.example`, `docs/setup-guide.md` — new knobs (`LLM_MAX_ATTEMPTS`, `LLM_RETRY_BACKOFF`, batch timeout guidance) and defaults.
- `internal/llm/metrics.go` — new counters.
- Tests: `internal/llm/*_test.go`, `internal/db/*_test.go` — retry-loop regression coverage (the 89× repro must end at ≤3 attempts).
