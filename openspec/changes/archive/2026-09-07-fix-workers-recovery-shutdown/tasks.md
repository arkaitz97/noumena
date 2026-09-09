## 1. Reaper for stuck jobs

- [x] 1.1 `recoverStaleJobs`: ingestion `running` stale → `pending`
- [x] 1.2 LLM `processing` stale → `pending`
- [x] 1.3 Run at startup + 5m tick; heartbeat/age guard; log resets
- [x] 1.4 LLM job dedupe at enqueue (pending/running covers comment set → skip)
- [x] 1.5 Tests: crash mid-job → channel recovers next boot; no duplicate LLM job

## 2. Digest recovery

- [x] 2.1 Startup catch-up for missed week-start digests (comments >= 10)
- [x] 2.2 Re-dispatch `deferred` digests with backoff schedule
- [x] 2.3 Per-user dispatch deadline; retries async
- [x] 2.4 Tests: downtime across Mon 08:00 → digest still generated; deferred eventually sent

## 3. Shutdown ordering

- [x] 3.1 Root cancellable ctx from signal handling
- [x] 3.2 Wire all workers to root ctx; WaitGroup coordination
- [x] 3.3 Sequence: workers drain → HTTP server stop → telemetry flush → db.CloseDB
- [x] 3.4 Test (integration): SIGTERM mid-job → job reset by reaper, no DB race

## 4. Processor safety

- [x] 4.1 `recover` per tick in LLM processor (log + metric + continue)
- [x] 4.2 `inflight.Add` before LLM call; Done deferred
- [x] 4.3 Test: panic in a tick doesn't kill process

## 5. Token selection + persistence

- [x] 5.1 Prefer unexpired token in `GetIngestionTokenRow`, then priority
- [x] 5.2 Persist refreshed token/expiry after forced refresh
- [x] 5.3 Test: expired primary skipped when fallback unexpired

## 6. Atomic email claim

- [x] 6.1 `ClaimPendingEmails` → UPDATE…RETURNING claim
- [x] 6.2 Share claim path between pipeline + retry worker
- [x] 6.3 Test: two concurrent claims get disjoint sets

## 7. Backoff jitter

- [x] 7.1 Add jitter to ingestion retry backoff
- [x] 7.2 Test: retry timing varies

## 8. Verify

- [x] 8.1 `go vet`, `go build`, `go test ./...` green
- [ ] 8.2 Manual: kill process mid-ingestion, restart, channel recovers and ingests
