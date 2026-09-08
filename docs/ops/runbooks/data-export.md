# Data Export

User-facing data export (CSV/JSON comments + GDPR bundle).

## Symptoms

- A user requests an export and the download link is missing, expired, or 404s.
- An export job is stuck in `pending` or `failed`.

## Verification

- `data_export_jobs` row for the user: `status` in
  `pending | processing | completed | failed`.
- Completed export has `export_url` (persisted file path) and a signed
  download URL valid for `EXPORT_LINK_TTL_DAYS` (default 7).

## Recovery

- **Sync vs async:** exports under `EXPORT_SYNC_THRESHOLD` (default 50k rows)
  complete synchronously; above that they run async via the export worker
  (`EXPORT_WORKER_ENABLED=true`).
- **Failed job:** inspect the worker logs, then re-enqueue:
  ```sql
  UPDATE data_export_jobs
     SET status='pending', attempt_count=0, started_at=NULL
   WHERE user_id=<uuid> AND status='failed';
  ```
- **Completed but user reports 404:** the cleanup cron has likely swept the
  file (TTL elapsed). Re-enqueue with the same procedure.
- **Signed URL mismatch:** verify the signature params match the persisted
  `export_url` and that `expires_at` has not passed.
- Export ZIP cap: 500MB. Partial exports include a README and count as
  success (not failure).

## Postmortem

- Confirm `EXPORT_STORAGE_DIR` has space (see
  [`export-disk-monitoring.md`](export-disk-monitoring.md)).
