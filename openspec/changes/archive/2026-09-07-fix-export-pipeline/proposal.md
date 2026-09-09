## Why

The data-export pipeline has runtime and robustness defects (audit + reproduced against live DB):

1. **Export worker errors on real SQLite** — `internal/db/lifecycle.go:496-498` writes `storage_path`, `download_url`, `partial_export`, `last_error`, but the schema declares `export_url`/`error_message` → `MarkDataExportCompleted`/`MarkDataExportFailed`/`ListExpiredDataExports` fail (reproduced).
2. **Stuck 'exporting' jobs never recover** — `ResetStuckProcessingExports` (`internal/db/exports.go:156`) only covers `comment_exports`; `data_export_jobs` stuck in `exporting` after a crash stay stuck forever.
3. **Orphaned ZIPs on mark failure** — `internal/lifecycle/export_worker.go:160-162`: ZIP written to disk, then `MarkDataExportCompleted` fails → file orphaned, job stuck `exporting`, no `os.Remove` cleanup path.
4. **Export blocks all writers** — `internal/export/worker.go:102` streams the export while holding the single pooled connection (`MaxOpenConns(1)`); with no WAL and busy_timeout 0, concurrent writes (ingestion marking synced_at, quota increments) block until the export ends.

## What Changes

- Fix column references in the export lifecycle to `export_url`/`error_message` (with the schema migration in `fix-sqlite-driver-migrations`).
- Add a reaper for `data_export_jobs` stuck in `exporting` (mirror of the comment-exports reset).
- On `MarkDataExportCompleted` failure, remove the written ZIP and reset the job to a retryable state (or mark failed) so it isn't orphaned.
- Stream exports over a dedicated read-only database handle (or rely on WAL + short busy_timeout from the SQLite fix) so export no longer starves writers.
- On completion, persist the final export URL before/atomically with state change so a crash cannot leave URL-less "completed" jobs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `data-lifecycle`: Data-export jobs recover from crashes and never orphan files.

## Impact

- `internal/db/lifecycle.go`, `internal/db/exports.go`, `internal/lifecycle/export_worker.go`, `internal/export/worker.go`
- `internal/db/db.go` (dedicated export handle, if adopted)
- Tests: `internal/lifecycle/*`, `internal/export/encoder_test.go`
