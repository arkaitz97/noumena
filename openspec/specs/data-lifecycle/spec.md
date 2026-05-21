# Data Lifecycle Specification

## Purpose
Handles account deletion (GDPR-compliant), GDPR data exports, and cleanup of orphaned shared channel data.

## Requirements

### Requirement: Account Deletion Request
The system SHALL allow users to request account deletion with email confirmation and a 30-day reversible window.

#### Scenario: Request Account Deletion
- **GIVEN** an authenticated user
- **WHEN** the user calls DELETE /api/user/account with email_confirmation matching their account email (case-insensitive)
- **THEN** the system SHALL set users.deletion_requested_at = NOW(), cancel the Stripe subscription if active (at period end for annual plans, immediately for monthly), insert an account_deletion_jobs row (idempotent), send a deletion_scheduled email with reversible_until timestamp (30 days from request), and return 202 with job_id, reversible_until, and status=scheduled

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
- **THEN** for each channel, the system SHALL: revoke OAuth token (best-effort, 3 retries), delete user's starred comments, delete the tracking row, check remaining active tracker count. If others exist: promote next ingestion token and stop. If sole tracker: chunk-delete comments (10K rows/transaction), then delete the platform_channels row

#### Scenario: New Tracker Appears During Delete
- **GIVEN** a sole-tracker channel being chunk-deleted
- **WHEN** another user starts tracking the same channel mid-delete
- **THEN** the system SHALL abort channel-level cleanup, log the partial delete count, and leave remaining comments for the new tracker

#### Scenario: Stage 3 - Final Wipe
- **GIVEN** a deletion job with all channels processed
- **WHEN** the worker runs the final stage
- **THEN** the system SHALL delete all weekly digests, remaining starred comments, and auth tokens in one transaction, send the deletion_completed email (before hard delete), hard-delete the user row, and mark the job completed

#### Scenario: User Already Gone
- **GIVEN** a deletion job whose user row no longer exists
- **WHEN** the worker attempts to load the user
- **THEN** the system SHALL mark the job completed and return success

#### Scenario: Failed Job No Auto-Retry
- **GIVEN** a deletion job that encounters an error
- **WHEN** the worker processes it
- **THEN** the system SHALL mark the job as 'failed' with last_error populated and NOT auto-retry (destructive surface is too large for blind retries)

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
The system SHALL revoke OAuth tokens when a user's tracking is removed.

#### Scenario: Revoke with Retry
- **GIVEN** an OAuth access token to revoke
- **WHEN** the deletion worker calls RevokeWithRetry
- **THEN** the system SHALL attempt revocation up to 3 times with backoff, log failures as non-fatal warnings, and continue deletion regardless of revocation outcome

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
