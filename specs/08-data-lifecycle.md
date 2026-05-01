# Spec: Data Lifecycle, Account Deletion & GDPR

## Overview
Implement robust data lifecycle management ensuring shared comments are only purged when the last tracker removes a channel, account deletion properly handles shared data, and GDPR export/deletion rights are supported.

## Requirements
- DATA-01: Pausing retains shared comments; re-activation resumes from last_seen_at
- DATA-02: Shared comments purged only when last tracker removes channel
- DATA-03: Account deletion removes user tracking; shared comments purged only if last tracker
- DATA-04: GDPR data export (ZIP of JSON) within 24 hours
- CM-05b: Permanent channel removal with confirmation modal
- CM-06: OAuth token expiry notifications
- EC-6.1 through EC-6.5: All account deletion edge cases

## Database Changes

Add to `users`:
- `deletion_requested_at TIMESTAMPTZ` — locks account immediately
- `deletion_completed_at TIMESTAMPTZ`
- `data_export_url TEXT` — secure link to GDPR export
- `data_export_expires_at TIMESTAMPTZ`

Create account deletion jobs table:
```sql
CREATE TABLE account_deletion_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'exporting', 'deleting', 'completed', 'failed'
    gdpr_export_generated BOOLEAN NOT NULL DEFAULT false,
    channels_processed INT NOT NULL DEFAULT 0,
    total_channels INT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);
```

## API Endpoints

### Authenticated Routes

#### DELETE /api/user/account
- Initiates account deletion process
- Two-step confirmation handled on frontend; backend accepts final confirmation
- Body: `{ "email_confirmation": "user@example.com" }` — must match user's email
- On success:
  1. Set `users.deletion_requested_at = NOW()`
  2. Lock account (subsequent logins return 403 "account scheduled for deletion")
  3. Cancel Stripe subscription immediately (or at period end for annual plans)
  4. Enqueue `account_deletion` job
  5. Return `{ "message": "Account deletion scheduled" }`

#### GET /api/user/export
- Initiates GDPR data export
- Returns `{ "job_id": "...", "status": "queued" }`
- Background job generates ZIP of JSON files:
  - `user.json` — account metadata
  - `tracking.json` — all `user_channel_tracking` rows
  - `comments.json` — all processed comments from tracked channels (even shared)
  - `digests.json` — all weekly digests
  - `stars.json` — all starred comments
- ZIP uploaded/served via secure, time-limited signed URL (expires in 7 days)
- Email sent with download link

#### GET /api/user/export/{job_id}/status
- Returns export status and download URL when ready

### Additional User Settings Endpoints

#### GET /api/user/me
- Returns current user profile:
```json
{
  "id": "uuid",
  "email": "user@example.com",
  "email_verified_at": "2026-03-01T12:00:00Z",
  "subscription_plan": "creator",
  "created_at": "2026-03-01T10:00:00Z"
}
```

#### PATCH /api/user/me
- Body: `{ "email": "..." }` (with re-verification required if email changes)

## Account Deletion Job

### Async Worker (`internal/lifecycle/deletion_worker.go`)

Runs queued deletion jobs within 30 days (target: 24 hours):

1. **Generate GDPR export** (if not already done):
   - Query all user data
   - Write JSON files to temp directory
   - Create ZIP archive
   - Store at secure URL, email link to user

2. **Process each tracked channel**:
   a. Revoke OAuth token via platform API (attempt 3 times over 6 hours)
   b. Check active tracker count for `platform_channel_id`:
      - If count > 0: delete only `user_channel_tracking` row
      - If count = 0: hard-delete shared `comments` in chunks of 10K rows, then delete `platform_channels` record
   c. Delete `user_comment_stars` for this user + channel

3. **Delete user-specific data**:
   - `weekly_digests` (hard delete)
   - `user_comment_stars` (any remaining)
   - `email_verification_tokens`, `password_reset_tokens`, `refresh_tokens`

4. **Stripe cleanup**:
   - Cancel subscription if still active
   - Do NOT delete Stripe customer (retained for legal/financial records)

5. **Final steps**:
   - Hard-delete `users` row
   - Send final confirmation email
   - Mark job as completed

### Reversible Deletion
- Within 48 hours of `deletion_requested_at`: user can contact support to cancel deletion
- After 48 hours or once purge begins: irreversible
- Login during reversible period returns 403 with message and support contact info

### Token Promotion During Deletion
- Before revoking a user's OAuth token, check if they are the primary token holder (priority = 0)
- If yes: call `PromoteNextIngestionToken` first
- Then revoke the token

## GDPR Export Job

### Worker (`internal/lifecycle/export_worker.go`)

1. Query all tables for user data
2. Serialize to JSON (pretty-printed)
3. Create ZIP with descriptive filenames:
   - `account/user.json`
   - `channels/tracking.json`
   - `comments/comments.json` (all comments from channels they tracked)
   - `insights/digests.json`
   - `activity/stars.json`
4. Save to temporary storage with signed URL (7-day expiry)
5. Update `users.data_export_url` and `data_export_expires_at`
6. Send email with download link
7. Cleanup temp files after expiry

## Backend Implementation

### New Package
- `internal/lifecycle/` — deletion and export workers

### DB Methods to Add
```go
// Deletion
ScheduleAccountDeletion(ctx context.Context, userID string) error
GetPendingDeletionJobs(ctx context.Context) ([]*AccountDeletionJob, error)
GetUserTrackingChannels(ctx context.Context, userID string) ([]*UserChannelTracking, error)
GetActiveTrackerCount(ctx context.Context, platformChannelID string) (int, error)
DeleteCommentsByChannelChunked(ctx context.Context, platformChannelID string, chunkSize int) error
DeletePlatformChannel(ctx context.Context, platformChannelID string) error
DeleteUserStars(ctx context.Context, userID string) error
DeleteUserDigests(ctx context.Context, userID string) error
DeleteUserTokens(ctx context.Context, userID string) error
HardDeleteUser(ctx context.Context, userID string) error
RevokeOAuthToken(ctx context.Context, platform, accessToken string) error

// Export
GetUserExportData(ctx context.Context, userID string) (*UserExportBundle, error)
```

## Testing Checklist
- [ ] Account deletion locks account immediately
- [ ] GDPR export generates valid ZIP with all data
- [ ] Shared comments retained when other trackers exist
- [ ] Shared comments purged when last tracker deletes account
- [ ] OAuth tokens revoked during deletion
- [ ] Primary token promoted before revocation if other trackers exist
- [ ] Stripe subscription canceled on deletion
- [ ] Login to deleted account returns 403 with support info
- [ ] Chunked deletion handles 1M+ comments without timeout
- [ ] Re-download of export link works within 7 days

## Files to Create/Modify
- `internal/lifecycle/deletion_worker.go` — new
- `internal/lifecycle/export_worker.go` — new
- `internal/db/db.go` — add lifecycle methods
- `internal/db/schema.sql` — add deletion jobs table
- `internal/api/auth_handlers.go` — add account endpoints
- `internal/email/client.go` — add deletion/export email templates
- `frontend/src/views/settings/AccountView.vue` — deletion UI, export button
