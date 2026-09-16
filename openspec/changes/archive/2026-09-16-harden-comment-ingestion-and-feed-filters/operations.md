# Recovery Operations

Inspect `channel_sync_checkpoints` and `video_sync_state` before retrying interrupted ingestion. Rows with `paused` or `error` status retain last committed page progress and are safe to retry. Do not advance `platform_channels.last_synced_at` manually while a poll/backfill checkpoint is incomplete.

After migration, verify schema version 20 and run ingestion tests against a copy of production data. Requeue affected channel jobs after correcting credentials or quota limits. Reconciliation retries are idempotent because comments use platform IDs for upsert/deduplication.
