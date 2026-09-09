# Data Lifecycle Specification

## Purpose
Handles account deletion (GDPR-compliant), GDPR data exports, and cleanup of orphaned shared channel data.

## Requirements

### Requirement: Account Deletion Request
The system SHALL accept an account-deletion request and enqueue an asynchronous deletion job.

#### Scenario: Request Account Deletion
- **GIVEN** an authenticated user
- **WHEN** the user calls DELETE /api/user/account with email_confirmation matching their account email (case-insensitive)
- **THEN** the system SHALL set users.deletion_requested_at to the current UTC timestamp (SQLite `strftime('%Y-%m-%dT%H:%M:%SZ','now')`), cancel the Stripe subscription if active (at period end for annual plans, immediately for monthly), insert an account_deletion_jobs row with status='pending' (idempotent), send a deletion_scheduled email with reversible_until timestamp (30 days from request), and return 202 with job_id, reversible_until, and status=scheduled

#### Scenario: Deletion Email Mismatch
- **GIVEN** an authenticated user
- **WHEN** the user submits an email_confirmation that does not match their account email
- **THEN** the system SHALL return 400

#### Scenario: Idempotent Deletion Request
- **GIVEN** a user who has already requested deletion (deletion_requested_at is set)
- **WHEN** the user calls DELETE /api/user/account again during the reversible window
- **THEN** the system SHALL return 202 with the existing reversible_until timestamp (no-op)

### Requirement: Cancel Pending Deletion
The system SHALL allow users to reverse a pending deletion within 30 days of request.

#### Scenario: Cancel Pending Deletion
- **GIVEN** a user with a deletion job in status='pending' and within 30 days of request
- **WHEN** the user calls POST /api/user/account/cancel-deletion
- **THEN** the system SHALL clear deletion_requested_at and return 200

#### Scenario: Cancel Non-Cancellable Deletion
- **GIVEN** a user whose deletion job is already claimed (status != 'pending') or past 30 days
- **WHEN** the user calls POST /api/user/account/cancel-deletion
- **THEN** the system SHALL return 409 with "no cancellable deletion found"

### Requirement: Deletion Worker
The system SHALL process account deletion jobs through a multi-stage pipeline.

#### Scenario: Stage 1 - GDPR Pre-Export
- **GIVEN** a claimed deletion job
- **WHEN** the worker runs the export stage
- **THEN** the system SHALL enqueue a data export, poll until completion or EXPORT_PRE_DELETE_TIMEOUT (default 12h), mark gdpr_export_generated=true on success, and proceed even on timeout (non-fatal)

#### Scenario: Stage 2 - Per-Channel Detach
- **GIVEN** a deletion job in the 'deleting' stage
- **WHEN** the worker iterates the user's tracked channels
- **THEN** for each channel, the system SHALL: revoke OAuth refresh token first, then access token (best-effort, bounded retries; already-invalid token not an error), delete user's starred comments, delete the tracking row, check remaining active tracker count. If others exist: promote next ingestion token and stop. If sole tracker: chunk-delete comments (10K rows/transaction), then delete the platform_channels row

#### Scenario: New Tracker Appears During Delete
- **GIVEN** a sole-tracker channel being chunk-deleted
- **WHEN** another user starts tracking the same channel mid-delete
- **THEN** the system SHALL abort channel-level cleanup, log the partial delete count, and leave remaining comments for the new tracker

#### Scenario: Stage 3 - Final Wipe
- **GIVEN** a deletion job with all channels processed
- **WHEN** the worker runs the final stage
- **THEN** the system SHALL delete every user-owned row in one transaction before the user row: email-verification, password-reset and refresh tokens, oauth_states, user_comment_stars, comment_usage_logs, comment_exports, workspace_members, rate_limit_buckets, user_channel_tracking, weekly_digests and data_export_jobs; send the deletion_completed email (before hard delete); hard-delete the user row; and mark the job completed

#### Scenario: User Already Gone
- **GIVEN** a deletion job whose user row no longer exists
- **WHEN** the worker attempts to load the user
- **THEN** the system SHALL mark the job completed and return success

#### Scenario: Failed Job No Auto-Retry
- **GIVEN** a deletion job that encounters an error
- **WHEN** the worker processes it
- **THEN** the system SHALL mark the job as 'failed' with last_error populated and NOT auto-retry (destructive surface is too large for blind retries)

### Requirement: Complete data erasure
When a user is hard-deleted, the system SHALL remove every row owned by that user across all tables, regardless of foreign-key enforcement state.

#### Scenario: Hard delete completes
- **GIVEN** a deletion job reaching its final stage
- **WHEN** the user row is removed
- **THEN** no rows referencing that user remain in any user-owned table (tokens, oauth states, stars, usage logs, exports, memberships, rate-limit buckets, digests, tracking)

#### Scenario: Shared channel with another owner
- **GIVEN** a channel also tracked by another user
- **WHEN** one user is deleted
- **THEN** only that user's tracking/grants are removed and the shared channel data is preserved

### Requirement: GDPR Data Export
The system SHALL allow users to export all their data as a ZIP file.

#### Scenario: Request Data Export
- **GIVEN** an authenticated user
- **WHEN** the user calls POST /api/user/data-export
- **THEN** the system SHALL enqueue a data_export_jobs row with status='pending' and return 202 with job_id and status

#### Scenario: Export Worker Processing
- **GIVEN** a pending data export job
- **WHEN** the export worker claims it
- **THEN** the system SHALL load the user's full bundle (account, tracking, comments, digests, stars), serialize each section to indented JSON, write them into a ZIP file at EXPORT_STORAGE_DIR/<user_id>/<job_id>.zip, enforce EXPORT_MAX_BYTES cap (500MB, mark partial=true if exceeded with README.md), mint a signed download URL with TTL (default 7 days), mark the job completed, and send an export_ready email

#### Scenario: Partial Export
- **GIVEN** a user whose data exceeds the 500MB export cap
- **WHEN** the export worker writes the ZIP
- **THEN** the system SHALL include a README.md explaining the truncation, mark the export as partial=true, and still send the export_ready email (partial is not a failure)

#### Scenario: Get Export Status
- **GIVEN** an authenticated user with an export job
- **WHEN** the user calls GET /api/user/data-export/{job_id}/status
- **THEN** the system SHALL return job_id, status, and when completed: download_url and expires_at. If the job belongs to another user, return 404 (not 403, to avoid leaking existence)

#### Scenario: Download Export via Signed URL
- **GIVEN** a completed export with a valid signed URL (sig + exp query params)
- **WHEN** a request hits GET /api/exports/{job_id}/download (no auth middleware)
- **THEN** the system SHALL verify the signature against EXPORT_SIGNED_URL_SECRET, check the file exists on disk, and serve the ZIP with Content-Type=application/zip. If signature is invalid/expired, return 403. If file missing, return 404.

### Requirement: Data-export job recovery
The system SHALL recover data-export jobs that were interrupted, without orphaning files.

#### Scenario: Crash mid-export
- **GIVEN** a data-export job stuck in `exporting`
- **WHEN** the application restarts or the reaper tick runs
- **THEN** the job is reset to a retryable state and any partial ZIP for a non-completed job is removed

#### Scenario: Mark-completion failure
- **GIVEN** a ZIP written but its completion row update fails
- **WHEN** the failure is handled
- **THEN** the orphaned file is removed and the job returns to a retryable state

### Requirement: Exports do not block writers
The system SHALL stream exports without holding the main write path.

#### Scenario: Export while ingestion writes
- **GIVEN** an ongoing export
- **WHEN** ingestion or quota writes occur concurrently
- **THEN** the writes proceed without unbounded blocking

### Requirement: Cleanup Worker
The system SHALL clean up orphaned shared channel data when the last tracker is removed.

#### Scenario: Schedule Channel Deletion
- **GIVEN** a platform channel with zero active trackers
- **WHEN** the last tracking row is deleted
- **THEN** the system SHALL set deletion_scheduled_at = NOW() + grace period (default 24h)

#### Scenario: Execute Channel Deletion
- **GIVEN** a platform channel whose deletion_scheduled_at has passed
- **WHEN** the cleanup worker processes it
- **THEN** the system SHALL hard-delete all comments for that channel and delete the platform_channels row

### Requirement: OAuth Token Revocation
The system SHALL revoke the user's OAuth refresh token in addition to the access token when their tracking is removed.

#### Scenario: Revoke with Retry
- **GIVEN** an OAuth token to revoke
- **WHEN** the deletion worker calls RevokeWithRetry
- **THEN** the system SHALL attempt revocation up to 3 times over a bounded ~30s window, log failures as non-fatal warnings, and continue deletion regardless of revocation outcome

#### Scenario: Delete revokes provider tokens
- **GIVEN** a user with linked YouTube OAuth tokens
- **WHEN** the deletion job runs the revoke stage
- **THEN** both access and refresh tokens are revoked at the provider (refresh first — it is the durable credential), and an already-invalid token is not treated as an error

#### Scenario: Revoke failure retried out-of-band
- **GIVEN** a deletion job whose in-job revocation failed
- **WHEN** the periodic revoke sweeper runs
- **THEN** the system SHALL retry revocation from the job's recorded revoke_error state while the user's credentials still exist, and clear the error on success

### Requirement: Audit Trail for Deletion Events
The system SHALL log all deletion-related events to an audit trail for compliance and debugging.

#### Scenario: Deletion Request Logged
- **WHEN** a user requests account deletion
- **THEN** the system SHALL insert an audit_log row with action='deletion_requested', user_id, ip_address, and timestamp

#### Scenario: Deletion Cancelled Logged
- **WHEN** a pending deletion is cancelled
- **THEN** the system SHALL insert an audit_log row with action='deletion_cancelled', user_id, and timestamp

#### Scenario: Deletion Completed Logged
- **WHEN** the deletion worker completes a deletion job
- **THEN** the system SHALL insert an audit_log row with action='deletion_completed', user_id, channels_deleted count, and timestamp

### Requirement: Annual Plan Cancellation at Period End
The system SHALL handle annual plan cancellations by allowing access until the paid period expires.

#### Scenario: Annual Plan Deletion Request
- **WHEN** a user on an annual plan requests account deletion
- **THEN** the system SHALL cancel the Stripe subscription at period end (not immediately), set deletion_requested_at, and inform the user they retain access until the paid period expires

#### Scenario: Annual Plan Period Expires
- **WHEN** the annual plan period end date passes for a user with a pending deletion
- **THEN** the system SHALL proceed with the deletion worker pipeline

### Requirement: Workspace deletion cleanup
The system SHALL clear member references when a workspace is deleted.

#### Scenario: Workspace deleted while active for members
- **GIVEN** members whose `active_workspace_id` points at the workspace
- **WHEN** the workspace is deleted
- **THEN** the members' `active_workspace_id` is NULLed and channel tracking is cleared, with no dangling references

### Requirement: Free-tier quota reset schedule
The system SHALL assign every free user a `quota_reset_at` so monthly resets always apply.

#### Scenario: New free user created
- **GIVEN** a new user on a free plan
- **WHEN** the account is created
- **THEN** `quota_reset_at` is set to approximately now + 30 days
