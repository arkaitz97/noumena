## Why

The GDPR account-deletion pipeline is broken end-to-end and leaks data when it does run:

1. **Deletion always 500s** — `internal/api/account_handlers.go:96` runs `UPDATE users SET deletion_requested_at = NOW() WHERE id = $1`; `NOW()` and `$1` are Postgres-isms invalid on modernc SQLite (reproduced) → every delete request errors. Same `$1` issue in `internal/lifecycle/deletion_worker.go:193`.
2. **Refresh token stays valid** — `internal/lifecycle/revoke.go:62-95` revokes the OAuth **access** token only; the long-lived refresh token remains valid provider-side.
3. **FK cascades are inert (FK off)** — final wipe (`deletion_worker.go:316-377`) covers digests/stars/tokens but misses `comment_exports`, `oauth_states`, `comment_usage_logs`, `workspace_members`, `rate_limit_buckets` → PII/tokens survive hard delete. `DeleteStaleUnverifiedUsers` (`db.go:434`) and `PurgeChannelData` (`channel_mgmt.go:110`) orphan rows the same way.
4. **Workspace deletion dangles refs** — `DeleteWorkspace` (`workspace.go:37`) doesn't clear `users.active_workspace_id`/`user_channel_tracking.workspace_id`; with FK on it would fail, with FK off it leaves dangling pointers.
5. **Free-tier quota never resets** — `ResetFreeTierQuotas` (`billing.go:202`) skips users with NULL `quota_reset_at`; new free users never get one → permanently blocked once they hit quota.
6. **Revoke retries stall the worker** — `revoke.go:105-127` runs 1m/1h/6h backoff inside the deletion job, blocking all subsequent deletions for up to ~7h.

## What Changes

- Fix deletion SQL to SQLite (`strftime('%Y-%m-%dT%H:%M:%SZ','now')` + `?`) in handler and worker.
- Revoke the refresh token provider-side (decrypt `RefreshTokenEnc`) in addition to the access token.
- Make hard-delete GDPR-complete with FK off: explicit ordered deletes for all user-owned tables (oauth tokens, oauth_states, verification/reset tokens, trackings, stars, digests, comment_exports, comment_usage_logs, workspace_members, rate_limit_buckets, platform_channels purged per-channel) inside the final-stage tx.
- `PurgeChannelData` also deletes other users' `user_comment_stars` referencing the channel and `ingestion_jobs`.
- `DeleteWorkspace` clears `active_workspace_id` for members and `tracking.workspace_id` before removing the row (safe with FK on or off).
- Default `quota_reset_at` to `now + 30d` for new users so free-tier resets are guaranteed.
- Make provider revoke retries non-blocking (async with jitter, cap total time per job, or out-of-band retry worker).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `data-lifecycle`: Account deletion is fully GDPR-complete even with foreign keys disabled; OAuth refresh tokens are revoked; revoke failures no longer block the worker.

## Impact

- `internal/api/account_handlers.go`, `internal/lifecycle/deletion_worker.go`, `internal/lifecycle/revoke.go`, `internal/db/lifecycle.go`, `internal/db/channel_mgmt.go`, `internal/db/workspace.go`, `internal/db/db.go`, `internal/db/billing.go`
- Tests: `internal/api/account_handlers_test.go` (real-DB path), `internal/lifecycle/*`, `internal/db/channel_mgmt_integration_test.go`
- Ops: none beyond existing env.
