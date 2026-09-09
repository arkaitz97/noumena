## 1. Fix export column references

- [x] 1.1 Replace `storage_path`/`download_url`/`partial_export`/`last_error` with `export_url`/`error_message` in `internal/db/lifecycle.go`
- [x] 1.2 Update `internal/lifecycle/export_worker.go` queries accordingly
- [x] 1.3 Test: export completes end-to-end on real SQLite (claim → ZIP → mark → read back)

## 2. Dedicated export read handle

- [x] 2.1 Add second `*sql.DB` (`query_only`) for export reads
- [x] 2.2 Route export queries through it
- [x] 2.3 Test: long export does not block a concurrent write (WAL or second handle)

## 3. Stuck-export reaper

- [x] 3.1 Add `ResetStuckDataExports` (exporting older than 30m → retryable)
- [x] 3.2 Wire into startup + periodic tick
- [x] 3.3 Test: crash mid-export recovers next boot

## 4. Orphan-file cleanup

- [x] 4.1 `defer os.Remove(zipPath)` on mark-failure
- [x] 4.2 Startup sweep removes ZIPs for non-completed/non-pending jobs
- [x] 4.3 Test: failed mark leaves no orphan file

## 5. Atomic URL + status change

- [x] 5.1 Persist `export_url` in the same tx as `completed` status flip
- [x] 5.2 Test: no completed row without URL; crash after write before mark recovers

## 6. Verify

- [x] 6.1 `go vet`, `go build`, `go test ./...` green
- [ ] 6.2 Manual: request export, confirm ZIP + URL + download; kill mid-export, restart, job recovers
