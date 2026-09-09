## Context

Deletion pipeline: `deletion_requested_at` set → async `deletion_worker` runs staged job (export best-effort → per-channel detach with chunked comment delete → final wipe → hard delete). FK is OFF in production (see `fix-sqlite-driver-migrations`), so every cascade assumption in the current code is silently inert. Column drift (`last_error`/`error_message`) is fixed by the SQLite/migrations change; this change makes the pipeline actually run and actually erase everything.

## Goals / Non-Goals

**Goals:**
- Account deletion endpoint returns success and enqueues the job (no 500).
- Refresh token revoked provider-side.
- Hard delete removes every user-owned row across all tables, regardless of FK setting.
- Channel purge removes stars/jobs from other users.
- Workspace delete cleans up member references.
- Free-tier users get a quota reset schedule.

**Non-Goals:**
- Not changing the staged/deferred deletion model (reversible window stays).
- Not running auto-retry for failed deletion jobs (stays manual-triage by design).
- Not implementing export here (see `fix-export-pipeline`).

## Decisions

### 1. SQLite-correct deletion SQL

`deletion_requested_at = strftime('%Y-%m-%dT%H:%M:%SZ','now')` with `?` placeholders. Define a shared `nowUTC()` helper in `internal/db` to avoid repeating the format string. Use the same helper for the worker's status writes.

### 2. Refresh-token revocation

`revoke.go` currently calls Google `revoke` with the access token. Add a second revocation using the decrypted `RefreshTokenEnc` (same endpoint accepts both token kinds). Order: revoke refresh first (it is the durable credential), then access. Treat a 400 from Google as already-revoked (success) — matches Google semantics for already-invalid tokens.

### 3. GDPR-complete hard delete with FK off

Run final wipe in one tx; delete child tables in dependency order **before** the user row. Table list (from schema + audit): refresh_tokens, oauth_states, user_comment_stars, comment_usage_logs, comment_exports, workspace_members, rate_limit_buckets, user_channel_tracking (and per-channel purge of platform_channels + ingestion_jobs + comments), weekly_digests, llm_processing_jobs, deletion/data-export jobs, audit log entries (optional retention), then `users`. Because FK may be off, do explicit DELETEs and don't rely on cascade. Where the shared-channel model means other users share `platform_channels`/comments, only detach (delete tracking) — never delete shared rows owned by another user (existing per-channel detach logic preserved).

### 4. Stale-user and channel purge orphan cleanup

- `DeleteStaleUnverifiedUsers`: explicit deletes for `refresh_tokens`, `oauth_states`, `user_channel_tracking`, `comment_exports` for that user before deleting the user row.
- `PurgeChannelData`: add `DELETE FROM user_comment_stars WHERE comment_id IN (SELECT id FROM comments WHERE channel_id=?)` and `DELETE FROM ingestion_jobs WHERE platform_channel_id=?` before deleting the channel row.

### 5. Workspace delete cleanup

`DeleteWorkspace` first `UPDATE users SET active_workspace_id=NULL WHERE active_workspace_id=?`, then `UPDATE user_channel_tracking SET workspace_id=NULL WHERE workspace_id=?` (or delete trackings if that is the workspace policy), then delete the workspace row. Reorder so it works with FK ON or OFF.

### 6. Non-blocking provider revoke

Move the retry loop out of the deletion job: attempt revoke with a short bounded retry (e.g. 3 tries, 30s total); on failure, record `revoke_attempted`/`revoke_error` on the job and let a periodic out-of-band sweeper retry revocations for jobs in a terminal-deleting state. Cap per-job time so the worker drains other users.

### 7. Free-tier reset schedule

Set `quota_reset_at = now + 30d` in the user-creation path (and for existing NULL users via a one-time migration/backfill) so `ResetFreeTierQuotas` always has a target.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Deleting shared-channel data of other users | Preserve detach-only semantics; only delete platform_channels/comments when no other tracker remains |
| Large comment tables make chunked delete slow | Keep existing chunked delete; run inside job with its own timeout |
| Out-of-band revoke sweeper adds a new periodic path | Reuse existing cron/tick infrastructure; idempotent (400 = done) |
| quota_reset_at backfill touches billing | Small one-time UPDATE guarded by migration |
