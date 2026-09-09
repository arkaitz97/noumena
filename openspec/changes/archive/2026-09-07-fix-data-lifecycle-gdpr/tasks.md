## 1. Fix deletion SQL for SQLite

- [x] 1.1 Add shared `nowUTC()` helper in `internal/db`
- [x] 1.2 Fix `account_handlers.go:96` UPDATE to `strftime(...)` + `?`
- [x] 1.3 Fix `deletion_worker.go:193` `$1` → `?`
- [x] 1.4 Real-DB integration test: request deletion → row marked, job enqueued

## 2. Revoke refresh token provider-side

- [x] 2.1 Decrypt `RefreshTokenEnc` in revoke path
- [x] 2.2 Revoke refresh token (treat Google 400 as already-revoked success)
- [x] 2.3 Keep access-token revocation
- [x] 2.4 Test: both tokens revoked; already-invalid token not an error

## 3. GDPR-complete final wipe

- [x] 3.1 Enumerate all user-owned tables from schema (incl. comment_exports, oauth_states, comment_usage_logs, workspace_members, rate_limit_buckets)
- [x] 3.2 Explicit ordered DELETEs in final-stage tx before user row
- [x] 3.3 Preserve detach-only semantics for shared channels
- [x] 3.4 Test: after delete, no rows for user across all listed tables (FK on AND off)

## 4. Orphan cleanup in purge paths

- [x] 4.1 `PurgeChannelData`: delete `user_comment_stars` by channel subquery + `ingestion_jobs`
- [x] 4.2 `DeleteStaleUnverifiedUsers`: explicit deletes for tokens/states/trackings/exports
- [x] 4.3 Tests for both

## 5. Workspace delete cleanup

- [x] 5.1 `DeleteWorkspace` NULLs `active_workspace_id` for members
- [x] 5.2 `DeleteWorkspace` clears `tracking.workspace_id`
- [x] 5.3 Test with FK on and off

## 6. Non-blocking provider revoke

- [x] 6.1 Cap in-job revoke retries (3 tries / ~30s)
- [x] 6.2 Record revoke status on job; out-of-band sweeper retries failures
- [x] 6.3 Test: flaky endpoint doesn't stall worker tick

## 7. Free-tier quota reset schedule

- [x] 7.1 Set `quota_reset_at = now+30d` on user creation
- [x] 7.2 One-time backfill for existing NULL users (via migration)
- [x] 7.3 Test: new free user resets after 30d; existing NULL user gets schedule

## 8. Verify

- [x] 8.1 `go vet ./...`, `go build ./...`, `go test ./...` green
- [ ] 8.2 Manual: delete account → job completes → all user tables empty
- [ ] 8.3 Manual: workspace delete → members' active_workspace_id NULL
