## MODIFIED Requirements

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
- **THEN** the system SHALL delete every user-owned row in one transaction before the user row: email-verification, password-reset and refresh tokens, oauth_states, user_comment_stars, comment_usage_logs, comment_exports, workspace_members, rate_limit_buckets, user_channel_tracking, user_profiles, weekly_digests and data_export_jobs; send the deletion_completed email (before hard delete); hard-delete the user row; and mark the job completed

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
- **THEN** no rows referencing that user remain in any user-owned table (tokens, oauth states, stars, usage logs, exports, memberships, rate-limit buckets, digests, tracking, user_profiles)

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
- **THEN** the system SHALL load the user's full bundle (account, profile, tracking, comments, digests, stars), serialize each section to indented JSON, write them into a ZIP file at EXPORT_STORAGE_DIR/<user_id>/<job_id>.zip, enforce EXPORT_MAX_BYTES cap (500MB, mark partial=true if exceeded with README.md), mint a signed download URL with TTL (default 7 days), mark the job completed, and send an export_ready email

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
