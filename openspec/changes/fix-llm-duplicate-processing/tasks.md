# Tasks: Fix LLM duplicate processing

## 1. Schema & config foundation

- [ ] 1.1 Versioned migration adding `comments.llm_attempts INTEGER NOT NULL DEFAULT 0` and `comments.llm_next_retry_at TEXT` (`internal/db/schema.sql` + `migrations.go`); test via `internal/dbtest` harness
- [ ] 1.2 Add config fields `LLMMaxAttempts` (default 3), `LLMRetryBackoff` (default `1m,5m,15m`, CSV parser next to `envDuration`); change `LLMBatchSize` default 50 → 15 (`internal/config/config.go`)
- [ ] 1.3 Update `.env.example` and `docs/setup-guide.md`: new vars, batch-timeout semantics note (total budget across attempts), deprecate `LLM_FAILOVER_WINDOW_SECONDS` (boot warning when set)

## 2. Timeout budget (D1)

- [ ] 2.1 Remove hardcoded `Timeout` from `http.Client` construction in `internal/llm/opencode.go:45`, `openai.go`, `anthropic.go` (keep injectable-client test constructors; add explicit no-timeout comment)
- [ ] 2.2 Rework `retryTransient` (`internal/llm/retry.go`): per-attempt deadline = remaining caller ctx / remaining attempts via `context.WithTimeout`; honor `Retry-After` only when shorter than the share; update `retry_test`-style coverage in `openai_test.go`/`failover_test.go`
- [ ] 2.3 Regression test: fake server responding after 90s completes under a 300s ctx and fails with exactly one provider call when the ctx budget is 60s (proves no hidden 60s client timeout)

## 3. Error taxonomy (D3)

- [ ] 3.1 Export `TerminalError` + `IsTerminal(err)` in `internal/llm`; wrap decode/parse errors and HTTP 4xx (non-429) as terminal at the three clients' decode sites; leave transport/429/5xx/deadline transient
- [ ] 3.2 Unit tests: 401/400/parse → `IsTerminal` true; 429/503/transport → false

## 4. Partial results (D4)

- [ ] 4.1 Change classification parsing (`prompts.go`/`client.go` `ParseClassificationResponse`) to return valid results + `MissingIndexes` instead of aborting on one bad entry
- [ ] 4.2 `persistBatch` (`internal/llm/processor.go`): persist valid indexes only; `IncrementCommentsUsed` by persisted count; job marked failed naming bad indexes; zero-valid keeps current full-failure path
- [ ] 4.3 Tests: 49-valid/1-invalid batch persists 49, charges 49, one attempt burned only on the invalid comment; all-invalid burns all

## 5. Retry state, backoff, dead-letter (D2)

- [ ] 5.1 DB methods: `IncrementLLMAttempts(ctx, ids []string, nextRetryAt *time.Time, capAttempts bool)` and claim-side helpers on `SQLiteQuery` (`internal/db/llm_jobs.go` or new `comments_retry.go`)
- [ ] 5.2 `GetUnprocessedCommentsByChannel` (`internal/db/comments.go:107`): add `llm_attempts < ?` and `llm_next_retry_at` eligibility predicates; thread max attempts from processor config
- [ ] 5.3 `processor.go` failure paths: on transient fail → attempts+1 + backoff timestamp per comment; at cap → emit `event=batch_parked` log; on `IsTerminal` → park immediately (set attempts to cap); missing/invalid indexes (4.2) get single-comment attempt bump
- [ ] 5.4 Loop regression test reproducing the incident shape: permanently-failing set must stop after exactly `LLM_MAX_ATTEMPTS` jobs across many ticks (was 89), with backoff gaps honored via fake clock
- [ ] 5.5 DB tests: parked comment invisible to claim; requeued (reset columns) visible again

## 6. Failover counter (D5)

- [ ] 6.1 `internal/llm/failover.go`: replace sliding window with consecutive-failure counter (success resets); default threshold 3; drop window fields; warn-once if env sets a window
- [ ] 6.2 Rework `failover_test.go`: minutes-apart failures trigger flip (old-window-would-never-fire case); success interleaving prevents flip

## 7. Telemetry (D7)

- [ ] 7.1 Add `noumena_llm_provider_calls_total{provider,retryable}` (increment per HTTP attempt in `retryTransient`), `noumena_llm_job_attempts_total{provider,outcome}`, `noumena_llm_parked_comments` gauge sampled in `tickOnce` (`internal/llm/metrics.go`)
- [ ] 7.2 Metric tests per `observability-metrics` delta scenarios; update `internal/telemetry` attrs if new label constants needed

## 8. Ops & verification

- [ ] 8.1 Runbook section (`docs/ops/runbooks/`): reading `event=batch_parked`, un-park SQL, expected attempt-ratio alert threshold (>1.2 calls/job)
- [ ] 8.2 Full `go test ./internal/llm/... ./internal/db/...` green; boot API against a copy of `noumena.db`, observe one tick cycle in logs: no requeue of parked sets, `batch_parked` emitted for historical poison batch
- [ ] 8.3 `openspec validate fix-llm-duplicate-processing --strict` passes; sync notes for `llm-cost-analytics` (attempts counters feed its cost denominators)
