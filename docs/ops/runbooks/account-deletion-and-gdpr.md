# Account Deletion & GDPR

Support workflow for account deletion requests and GDPR data export.

## Symptoms

- A user requests deletion of their account.
- A user asks for a copy of their personal data (GDPR export).

## Verification

- `DELETE /api/user/account` returns 202 with `job_id`, `reversible_until`,
  `status=scheduled` and sets `users.deletion_requested_at`.
- The deletion job rows appear in `account_deletion_jobs` with
  `status='pending'`.

## Recovery

Deletion is **not** a hard delete on request. It is reversible within
`DELETION_REVERSIBLE_HOURS` (default 720 h, 30 days) while the job is still
`pending`. To reverse:

```sh
# Dry-run first (validates state, changes nothing):
go run ./tools/cancel_deletion --email user@example.com

# Execute:
go run ./tools/cancel_deletion --email user@example.com --confirm
```

The tool refuses once the worker has started (`processing`) — OAuth tokens
may already be revoked and comments deleted.

Once irreversible, past either gate recovery requires restoring from the last
backup (see [`backup-restore.md`](backup-restore.md)).

Deletion stages (worker):
1. GDPR export (best-effort, `EXPORT_PRE_DELETE_TIMEOUT` default 12h).
2. Per-channel detach — OAuth token revocation, chunked comment delete
   (`DELETION_CHUNK_SIZE` default 10k rows/tx).
3. Final wipe — digests, stars, auth tokens, hard delete of the user.

## Postmortem

- Record whether export completed before purge; confirm no shared-channel
  comments owned by other trackers were touched.
