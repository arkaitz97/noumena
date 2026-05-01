# Spec: Channel Management & YouTube OAuth

## Overview
Implement the channel connection system allowing users to connect YouTube channels via OAuth, with support for the shared channel model where one `platform_channels` record is shared across multiple users via `user_channel_tracking`.

## Requirements
- CM-01: User can connect a YouTube channel via OAuth 2.0
- CM-04: User can add tracked channels from the dashboard (backfill if new, instant access if existing)
- CM-05a: User can stop tracking a channel (pause, not delete)
- CM-05b: User can permanently remove a tracked channel
- CM-06: System notifies user when their OAuth token for a channel is about to expire (7 days before)
- CM-07: User can set a per-tracking comment backfill window (30/60/90/180 days)
- ING-07: Primary ingestion token promotion when token expires

## Database Changes

No new tables needed (existing `platform_channels` and `user_channel_tracking` are sufficient), but update `user_channel_tracking`:
- Add `backfill_days INT NOT NULL DEFAULT 90` (choices: 30, 60, 90, 180)
- Add `token_expiry_notified_at TIMESTAMPTZ` — to prevent duplicate expiry notifications

## API Endpoints

### Authenticated Routes

#### GET /api/channels
- Returns list of channels tracked by the current user
- Includes: channel id, platform, display_name, tracking_status, created_at, last_synced_at
- Current stub in `channel_handlers.go` only returns `PlatformChannel`; extend to include tracking status

#### GET /api/channels/connect/youtube
- Returns the Google OAuth authorization URL instead of redirecting (SPA compatibility)
- Response: `{ "auth_url": string }`
- Includes `state` parameter with signed CSRF token (stored temporarily, e.g. in a short-lived cookie or cache)
- Scopes required: `https://www.googleapis.com/auth/youtube.readonly`
- Add `access_type=offline` and `prompt=consent` to ensure refresh token is received

#### POST /api/channels/exchange/youtube
- Body: `{ "code": string, "state": string }` (state for CSRF validation)
- Exchanges code for tokens using `oauth2.Config`
- Fetches channel info via YouTube Data API v3 (`channels?mine=true&part=snippet`)
- Upserts `platform_channels`:
  - If `(platform='youtube', platform_channel_id)` exists: reuse existing record
  - If new: insert with `last_synced_at = NULL` (triggers backfill)
- Creates `user_channel_tracking` row with encrypted tokens, `tracking_status = 'active'`, `token_priority = 0` if first tracker else next available priority
- If channel is new: enqueue backfill job (see Ingestion spec)
- If channel already exists: user immediately sees all existing processed comments
- Response: 201 with channel info
- Edge Cases:
  - Invalid/missing code: 400
  - User denies OAuth: client should not call this endpoint; if called with invalid code, 400
  - Channel already tracked by this user: 409

#### PATCH /api/channels/{id}/pause
- Sets `user_channel_tracking.tracking_status = 'paused'` for this user only
- Shared `platform_channels` and `comments` untouched
- If this was the last active tracker, ingestion worker stops polling (next cycle checks tracker count)
- Response: `{ "message": "channel paused" }`

#### POST /api/channels/{id}/resume
- Sets `user_channel_tracking.tracking_status = 'active'`
- Updates `last_seen_at` to current time if null
- New comments ingested after pause immediately appear (already in shared DB)
- Response: `{ "message": "channel resumed" }`

#### DELETE /api/channels/{id}
- Permanently removes user's tracking record
- Deletes all `user_comment_stars` for this user + this channel's comments
- Checks if other active trackers exist:
  - Yes: only delete `user_channel_tracking` row
  - No: schedule `platform_channels` for deletion (set `deletion_scheduled_at = NOW() + 24 hours`) and enqueue shared comment purge job
- Response: `{ "message": "channel removed" }`
- Requires confirmation modal on frontend

#### PATCH /api/channels/{id}/backfill-window
- Body: `{ "days": 30 | 60 | 90 | 180 }`
- Only affects first-time additions going forward; does not re-backfill existing channels

#### GET /api/channels/{id}/status
- Returns detailed status: tracking_status, token_expiry, last_synced_at, primary_token_holder (boolean for current user), comment count

### Token Expiry Notifications
- Daily cron job checks `user_channel_tracking` for tokens expiring within 7 days where `token_expiry_notified_at IS NULL`
- Sends email notification + sets `token_expiry_notified_at`
- On login, API checks for expiring tokens and returns banner data in a header or dedicated endpoint

### Primary Token Management
- `GetIngestionToken` in DB layer should return the token with `token_priority = 0` (lowest number) and `tracking_status = 'active'`
- `PromoteNextIngestionToken`: when primary token holder pauses, removes channel, or token expires:
  1. Set current primary's `token_priority` to a high number or mark invalid
  2. Find next active tracker with lowest `token_priority` > 0, set their priority to 0
  3. If no active trackers remain, stop ingestion

## Backend Implementation

### OAuth Exchange (`internal/oauth/youtube.go`)
- Ensure `oauth2.Config` is properly initialized with client ID/secret from env
- `ExchangeAndFetchChannel` should handle token exchange and YouTube API call
- Return both oauth2.Token and channel metadata

### Token Encryption
- Use existing `auth.Encrypt` / `auth.Decrypt` (AES-256) for access/refresh tokens
- Decrypt happens in ingestion worker when it needs to call YouTube API

### YouTube API Client
- Create `internal/ingestion/youtube.go` with functions:
  - `FetchComments(ctx, token, channelID, since) ([]Comment, error)`
  - `FetchChannelInfo(ctx, token) (*ChannelInfo, error)`
- Implement exponential backoff for API errors

### DB Layer Updates
Add to `DBQuery` interface:
```go
GetChannelByPlatformID(ctx context.Context, platform, platformChannelID string) (*PlatformChannel, error)
CreatePlatformChannel(ctx context.Context, platform, platformChannelID, displayName string) (*PlatformChannel, error)
GetUserTracking(ctx context.Context, userID, platformChannelID string) (*UserChannelTracking, error)
ResumeChannelTracking(ctx context.Context, userID, platformChannelID string) error
UpdateTokenExpiry(ctx context.Context, trackingID string, expiry time.Time) error
GetExpiringTokens(ctx context.Context, withinDays int) ([]*UserChannelTracking, error)
GetActiveTrackerCount(ctx context.Context, platformChannelID string) (int, error)
ScheduleChannelDeletion(ctx context.Context, platformChannelID string) error
PurgeChannelData(ctx context.Context, platformChannelID string) error
```

## Frontend Requirements

### Views
- `/settings/channels` (ChannelsView.vue):
  - List of tracked channels with status badges (Active, Paused)
  - "Connect YouTube Channel" button → opens OAuth popup
  - Pause/Resume/Remove actions per channel
  - Backfill window selector (for new connections)
  - Token expiry warning banner

### OAuth Flow (SPA)
1. User clicks "Connect YouTube"
2. Frontend calls `GET /api/channels/connect/youtube` to get auth URL
3. Opens popup window to Google consent screen
4. Popup redirects to a frontend callback route (e.g. `/oauth/callback?code=...&state=...`)
5. Callback page extracts code and state, posts to `POST /api/channels/exchange/youtube`
6. Closes popup, parent window refreshes channel list

## Testing Checklist
- [ ] Connect new YouTube channel creates platform_channels + user_channel_tracking
- [ ] Second user connecting same channel reuses platform_channels, instant access to comments
- [ ] Pause channel sets status to paused, other users unaffected
- [ ] Resume channel restores access to new comments
- [ ] Remove channel deletes tracking row; shared data retained if other trackers exist
- [ ] Last tracker removal schedules deletion and purges comments
- [ ] Token expiry notification sent 7 days before expiry
- [ ] Primary token promotion works when primary pauses/removes

## Files to Create/Modify
- `internal/api/channel_handlers.go` — extend with all endpoints
- `internal/db/channels.go` — add DB methods
- `internal/db/tracking.go` — add tracking methods
- `internal/oauth/youtube.go` — ensure proper exchange flow
- `internal/ingestion/youtube.go` — YouTube API client
- `internal/email/client.go` — add token expiry email template
- `frontend/src/views/settings/ChannelsView.vue` — implement channel management UI
