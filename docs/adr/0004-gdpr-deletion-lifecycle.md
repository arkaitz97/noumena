# 0004. Delete accounts through a reversible 30-day job with a pre-delete export

## Status

Accepted. 2026-11-24.

## Context and Problem Statement

A creator account holds personal data: email, password hash, OAuth tokens,
tracking rows, stars, exports, digests, audit entries, and email logs. Some of
it is shared with other users: a channel's comments are visible to every
tracker of that channel.

Deletion must satisfy GDPR erasure, must not destroy another user's data, and
must be reversible long enough for a user to change their mind. It must also
handle the case where the user was the only tracker of a channel.

## Decision Drivers

- Legal erasure must be complete, including files on disk and email logs.
- A mistaken deletion must be recoverable for a stated window.
- Shared data must survive if another user still depends on it.
- The destructive step must not run twice or half-run.

## Considered Options

1. **Immediate hard delete on request.**
2. **Soft delete forever.** Keep the row and mark it inactive.
3. **Reversible grace window, staged job, export before wipe, then hard
   delete.**

## Decision Outcome

Chosen option 3.

`DELETE /api/user/account` requires the account email as typed confirmation.
It sets `deletion_requested_at`, cancels the Stripe subscription, and enqueues
an `account_deletion_jobs` row (`internal/api/account_handlers.go`). The
default reversible window is 720 hours
(`DeletionReversibleHours`, `internal/config/config.go`). The user can cancel
with `POST /api/user/account/cancel-deletion` while the job is still pending
and the window has not passed.

The worker (`internal/lifecycle/deletion_worker.go`) runs four stages:

1. `pending` until the grace window ends.
2. `exporting`: generate the GDPR bundle, wait up to
   `EXPORT_PRE_DELETE_TIMEOUT` (12 h).
3. `deleting`: per channel, revoke OAuth tokens, delete the user's stars and
   tracking row. If another tracker exists, promote the next ingestion token
   and keep the comments. If none exists, delete comments in chunks and drop
   the channel row. A tracker that appears mid-delete aborts the channel purge.
4. Final: remove the export directory from disk, wipe 15 user-scoped tables in
   one transaction, send the completion email, delete the email log rows, and
   hard-delete the user in a second transaction.

A failed job is not retried automatically; it stays visible for an operator.
`tools/cancel_deletion` reverses a pending deletion out of band.

### Consequences

#### Good

- Erasure is complete for the user's own data, including the export ZIP and
  the email log, which most implementations forget.
- Shared comments survive while another tracker needs them, so deletion cannot
  break a teammate's dashboard.
- The irreversible step is bounded and auditable. `DeleteUserGDPRData` lists
  every table in one place.

#### Bad

- The wipe is split across two transactions. A crash between the GDPR wipe and
  the hard delete leaves a user row without related data and the job stuck in
  `deleting`. `ClaimNextDeletionJob` only claims `pending` jobs, so recovery is
  manual. This is a deliberate trade-off: the destructive step is never
  retried blindly.
- Comment deletion is chunked and can be slow on a channel with millions of
  comments.

#### Neutral

- Audit entries for the deletion are retained with anonymized details, not
  removed. The row's `user_id` becomes null through `ON DELETE SET NULL`.
- The deletion API is behind `ACCOUNT_LIFECYCLE_API_ENABLED`.

## Confirmation

- `go test ./internal/db/ -run 'TestDeleteUserGDPRData|TestPurgeChannelData'`
- `go test ./internal/lifecycle/`
- The FK-on and FK-off variants prove the wipe does not rely on cascade.
- `TestPurgeChannelData_OrphanCleanup` proves another user's stars do not block
  the channel purge once the last tracker is gone.

## More Information

- `internal/lifecycle/deletion_worker.go`
- `internal/lifecycle/sweeper.go`
- `internal/db/lifecycle.go`
- `docs/ops/runbooks/account-deletion-and-gdpr.md`
- `docs/design.md` section 3.6

Resolution (2026-11-24): 720 hours (30 days) is authoritative. The config
default, `docs/api/user.md`, `docs/api/billing.md`, and the README already said
30 days. `.env.example` and the two account-deletion runbooks were corrected
from 48 hours. The 30-day figures in `docs/compliance/` govern YouTube raw-data
retention, not the account-deletion grace window. `TestLoadDeletionReversibleHoursDefault`
asserts the default.
