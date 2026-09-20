## 1. Config and wiring

- [x] 1.1 Add `LLMMaxConcurrency int` to `config.Config` with default 4 and parse `LLM_MAX_CONCURRENCY` in `internal/config/config.go` (clamp to `>= 1`)
- [x] 1.2 Document `LLM_MAX_CONCURRENCY` (default 4; `1` = serial) in `.env.example`
- [x] 1.3 In `startLLMProcessor` (`cmd/api/main.go`), set `proc.MaxConcurrency = config.Default.LLMMaxConcurrency`

## 2. Processor concurrency

- [x] 2.1 Add `MaxConcurrency int` to `Processor` and default it in `NewProcessor` (fallback 4 when `<= 0`)
- [x] 2.2 Rewrite `tickOnce` to use a buffered-channel semaphore: one goroutine per claimed channel batch, bounded by `MaxConcurrency`, with a `WaitGroup` joined before returning
- [x] 2.3 Check `ctx.Err()` before acquiring each slot so cancellation stops new batches while in-flight ones finish
- [x] 2.4 Move `inflight.Add(1)`/`Done()` to span the whole `processChannelBatch` call so `shutdown()` waits for all concurrent batches
- [x] 2.5 Ensure concurrent goroutines never mutate shared processor state (`now`, `RetryBackoff`, client) — read-only after construction

## 3. Observability

- [x] 3.1 Add gauge `noumena_llm_batches_in_flight` to `internal/llm/metrics.go` and helpers to inc/dec it
- [x] 3.2 Instrument slot acquire/release around `processChannelBatch`; keep existing per-batch counters/labels unchanged

## 4. Tests

- [x] 4.1 Add a goroutine-safe blocking fake provider tracking concurrent call count
- [x] 4.2 Test: with `MaxConcurrency=3` and 6 channel batches, observed max concurrency is exactly 3 (`processor_test.go`)
- [x] 4.3 Test: `MaxConcurrency=1` keeps strict serial execution (no overlap)
- [x] 4.4 Test: disjoint concurrent batches classify each comment exactly once (no duplicates)
- [x] 4.5 Test: `shutdown()` waits for all in-flight concurrent batches and no goroutine leaks after `tickOnce` returns
- [x] 4.6 Test: `ctx` cancellation prevents new batches from starting
- [x] 4.7 `go test ./internal/llm/... ./internal/config/...` passes; `go build ./...` clean

## 5. Validation

- [x] 5.1 Verify default activation does not change job/dedupe logs (no `batch_deduped` spike) on a multi-channel backlog
- [x] 5.2 Confirm `LLM_MAX_CONCURRENCY=1` reproduces current behavior and is the documented rollback
- [x] 5.3 Note failover interaction: confirm the error threshold does not spuriously flip under concurrent load (or file a follow-up)

## 6. Continuous pool + durable claim (follow-up to observed barrier)

- [x] 6.1 Continuous worker pool in `Run`: `MaxConcurrency` workers plus a claimer, so a freed worker starts the next batch immediately (no per-tick barrier)
- [x] 6.2 Durable claim: `GetUnprocessedCommentsByChannel` excludes comments referenced by a `pending`/`processing` `llm_processing_jobs` row (JSON1 `json_each`), making rapid reclaiming safe
- [x] 6.3 Reserve at claim time (`reserveBatch`) so the exclusion sees in-flight work and skipped channels never leave job rows
- [x] 6.4 Bound reservations with a `permits` channel: at most `MaxConcurrency` batches reserved/started at once, regardless of claimable channel count
- [x] 6.5 Keep `tickOnce` as a deterministic test seam (`claimBatches` + barrier)
- [x] 6.6 Tests: worker refills without barrier; reservations bounded by concurrency; DB claim excludes in-flight jobs and releases on completion
- [x] 6.7 `go build ./...` clean; `go test ./internal/llm/... ./internal/db/... ./internal/config/...` passes; new tests pass under `-race`

## 7. Bug fixes found during validation

- [x] 7.1 NULL-safe claim exclusion: `NOT IN (subquery)` with a JSON-null `comment_ids` row suppressed every claim; replaced with `NOT EXISTS (... je.value = c.id)` guarded by `json_valid`
- [x] 7.2 Resume restores the channel primary: `ResumeTracking` now assigns `token_priority = 0` when no active priority-0 tracker remains, instead of always `MAX+1`
- [x] 7.3 Tests: `TestClaim_NullJobCommentIdsDoesNotSuppressClaims`, `TestResume_RestoresPrimaryAndClaimVisibility`, `TestProcessor_RunDrainsBacklogE2E`
- [x] 7.4 `go test ./internal/db/... ./internal/llm/...` passes; new tests pass under `-race`; `gofmt` clean


