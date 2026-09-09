## Context

Export flow: user requests export → `data_export_jobs` row → export worker claims, queries comments/channels/digests, writes a ZIP to `EXPORT_DIR`, calls `MarkDataExportCompleted` (sets `export_url`, signed URL served later) → user downloads via signed URL. Failures: column drift makes the mark step error; crash leaves `exporting` rows; a held single connection blocks writers during long exports.

## Goals / Non-Goals

**Goals:**
- Export completes on real SQLite (column names agree with schema).
- Stuck `exporting` jobs recover after crash.
- No orphaned ZIP files.
- Export does not block ingestion/quota writes.

**Non-Goals:**
- Not adding new export formats (CSV/JSON/ZIP stay).
- Not moving export storage to S3 (local disk stays).
- Not changing signed-URL download design.

## Decisions

### 1. Column alignment

Use `export_url` and `error_message` in all export queries. The schema rename migration lives in `fix-sqlite-driver-migrations`; this change updates the Go code so the two land together (apply order: migrations first, then this).

### 2. Dedicated export read handle

Open a second `*sql.DB` at startup (`exportDB`) with `_pragma=query_only(1)` and `SetMaxOpenConns(1)` (or N) used only by export reads. This keeps the export cursor off the main write pool. Combined with WAL from the SQLite fix, writers proceed while export streams. If a second handle is deemed over-engineering, fall back to WAL + `_pragma=busy_timeout(5000)` only and document the constraint.

### 3. Reaper for data_export_jobs

Add `ResetStuckDataExports` run at startup + periodic tick: rows in `exporting` with `started_at` older than a threshold (e.g. 30m) reset to `pending` (or `failed` with error if retry count exceeded). Reuse the existing comment-exports reset pattern.

### 4. Orphan-file cleanup

Order the completion path as: write ZIP → write final URL into job (UPDATE, small tx) → mark completed. If the mark/completion tx fails, `defer os.Remove(zipPath)` removes the partial file and the job is reset to retryable. Log the removal for triage. Additionally, a startup sweep removes ZIPs whose job is not in `completed`/`pending` state (crash between write and mark).

### 5. Atomic state change

Persist `export_url` in the same tx as the status flip to `completed` (single UPDATE) so no URL-less completed row exists. Consumers of signed URLs read from the same column.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Second DB handle adds memory/connection overhead | query_only + low conns; SQLite handle is cheap; can gate behind env |
| Reaper resets a genuinely slow export | Threshold 30m + retry count; logs every reset |
| Deleting ZIP while download in flight | Only remove ZIPs for non-completed jobs; completed downloads keep file until expiry cleanup |
| Race: reaper vs active export | Reaper checks `started_at` age and status; active export periodically refreshes `started_at` or is excluded by in-memory marker |
