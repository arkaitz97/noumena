# Channel Management Specification

## Purpose
Handles connecting, pausing, resuming, and removing YouTube channel tracking relationships between users and platform channels. Implements the shared channel model where one real-world channel is tracked by multiple independent users.

## Requirements

### Requirement: List Tracked Channels
The system SHALL return all channels tracked by the authenticated user.

#### Scenario: List User Channels
- **GIVEN** an authenticated user with tracked channels
- **WHEN** the user calls GET /api/channels
- **THEN** the system SHALL return the list of channels with their tracking status, display name, last_synced_at, and token expiry

### Requirement: Connect YouTube Channel via OAuth
The system SHALL allow users to connect a YouTube channel through the OAuth 2.0 flow.

#### Scenario: Start YouTube OAuth Flow
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/channels/connect/youtube
- **THEN** the system SHALL generate a cryptographically random CSRF state, persist its SHA-256 hash with the user_id and expiry (10min TTL), and return the YouTube OAuth authorization URL with `access_type=offline` and `prompt=consent`

#### Scenario: Complete YouTube OAuth Exchange
- **GIVEN** a valid OAuth code and CSRF state from the Google redirect
- **WHEN** the user submits POST /api/channels/exchange/youtube with code, state, and backfill_days (30/60/90/180, default 90)
- **THEN** the system SHALL validate and consume the CSRF state, exchange the code for tokens, fetch the channel metadata, encrypt tokens (AES-256), upsert the `platform_channels` record, create a `user_channel_tracking` row with computed token_priority (0 for first tracker, MAX+1 otherwise), enqueue a backfill job if the channel is new, and return 201 with the tracking details

#### Scenario: Duplicate Channel Connection
- **GIVEN** a user already tracking a specific channel
- **WHEN** the user attempts to connect the same channel again
- **THEN** the system SHALL return 409 with "channel is already tracked"

#### Scenario: Invalid Backfill Days
- **GIVEN** a backfill_days value not in {30, 60, 90, 180}
- **WHEN** the user submits the exchange request
- **THEN** the system SHALL return 400 with "backfill_days must be 30, 60, 90, or 180"

#### Scenario: Plan Limit Reached
- **GIVEN** a user at their plan's channel limit (non-Agency)
- **WHEN** the user attempts to connect a new channel after valid CSRF
- **THEN** the system SHALL return 402 with "channel limit reached for the current plan; upgrade to add more channels"

#### Scenario: Token Priority Race Condition
- **GIVEN** two users connecting the same channel simultaneously
- **WHEN** both attempt to create tracking rows with token_priority=0
- **THEN** the system SHALL retry once with priority=1 on unique constraint violation, ensuring one gets priority=0 and the other gets priority=1

### Requirement: Pause Channel Tracking
The system SHALL allow users to pause tracking a channel without deleting shared data.

#### Scenario: Pause Active Channel
- **GIVEN** a user actively tracking a channel
- **WHEN** the user calls PATCH /api/channels/{id}/pause
- **THEN** the system SHALL set `tracking_status = 'paused'`, and if the user was the primary token holder (priority=0), promote the next ingestion token. Return 200.

#### Scenario: Pause Non-Tracked Channel
- **GIVEN** a channel the user does not track
- **WHEN** the user calls PATCH /api/channels/{id}/pause
- **THEN** the system SHALL return 404 with "channel not tracked"

### Requirement: Resume Channel Tracking
The system SHALL allow users to resume a paused channel.

#### Scenario: Resume Paused Channel
- **GIVEN** a user with a paused channel
- **WHEN** the user calls POST /api/channels/{id}/resume
- **THEN** the system SHALL set `tracking_status = 'active'` and return 200

### Requirement: Remove Channel Tracking
The system SHALL allow users to permanently remove their tracking of a channel.

#### Scenario: Remove Channel (Other Trackers Exist)
- **GIVEN** a user tracking a channel that other users also track
- **WHEN** the user calls DELETE /api/channels/{id}
- **THEN** the system SHALL delete the user's tracking row and starred comments for that channel, promote the next ingestion token if the user was primary, and return 200. Shared comments and platform_channels SHALL be retained.

#### Scenario: Remove Channel (Last Tracker)
- **GIVEN** a user who is the sole tracker of a channel
- **WHEN** the user calls DELETE /api/channels/{id}
- **THEN** the system SHALL delete the tracking row, schedule the platform_channels record for deletion after the grace period (default 24h), and return 200. Shared comments will be purged when the scheduled deletion runs.

### Requirement: Update Backfill Window
The system SHALL allow users to configure their preferred backfill window for future first-time channel additions.

#### Scenario: Update Backfill Window
- **GIVEN** an authenticated user
- **WHEN** the user calls PATCH /api/channels/{id}/backfill-window with days in {30, 60, 90, 180}
- **THEN** the system SHALL update the user's backfill_days setting and return 200 with a note that this affects future first-time additions only

### Requirement: Channel Status
The system SHALL provide detailed status for a tracked channel.

#### Scenario: Get Channel Status
- **GIVEN** a user tracking a channel
- **WHEN** the user calls GET /api/channels/{id}/status
- **THEN** the system SHALL return tracking_status, token_expires_at, last_synced_at, is_primary_token_holder, comment_count, and backfill_days

#### Scenario: Get Channel Sync Status
- **GIVEN** a user tracking a channel
- **WHEN** the user calls GET /api/channels/{id}/sync-status
- **THEN** the system SHALL return last_synced_at, next_sync_at, backfill_progress_percent (nullable), and recent_error (nullable). If not tracked, return 404.

### Requirement: Plan-Based Channel Limits
The system SHALL enforce channel limits based on the user's subscription plan.

#### Scenario: Agency Plan Unlimited Channels
- **GIVEN** a user on the Agency plan
- **WHEN** the user attempts to connect a new channel
- **THEN** the system SHALL NOT block the connection regardless of current channel count

#### Scenario: Free Plan Single Channel Limit
- **GIVEN** a user on the Free plan with 1 active/paused channel
- **WHEN** the user attempts to connect a second channel
- **THEN** the system SHALL return 402 blocking the connection

### Requirement: TikTok OAuth Placeholder
The system SHALL support TikTok account connection via OAuth 2.0 as a placeholder for future implementation.

#### Scenario: Start TikTok OAuth Flow
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/channels/connect/tiktok
- **THEN** the system SHALL return 501 with `{"code": "platform_not_supported", "message": "TikTok integration is coming soon"}`

#### Scenario: TikTok Exchange Attempt
- **GIVEN** an authenticated user
- **WHEN** the user calls POST /api/channels/exchange/tiktok
- **THEN** the system SHALL return 501 with `{"code": "platform_not_supported", "message": "TikTok integration is coming soon"}`

### Requirement: Instagram OAuth Placeholder
The system SHALL support Instagram account connection via Meta Graph API as a placeholder for future implementation.

#### Scenario: Start Instagram OAuth Flow
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/channels/connect/instagram
- **THEN** the system SHALL return 501 with `{"code": "platform_not_supported", "message": "Instagram integration is coming soon"}`

#### Scenario: Instagram Exchange Attempt
- **GIVEN** an authenticated user
- **WHEN** the user calls POST /api/channels/exchange/instagram
- **THEN** the system SHALL return 501 with `{"code": "platform_not_supported", "message": "Instagram integration is coming soon"}`

### Requirement: Platform Validation
The system SHALL validate the platform parameter on all channel connection endpoints.

#### Scenario: Valid Platform
- **WHEN** a user calls a channel connection endpoint with platform=youtube
- **THEN** the system SHALL proceed with the YouTube OAuth flow

#### Scenario: Invalid Platform
- **WHEN** a user calls a channel connection endpoint with platform=twitter
- **THEN** the system SHALL return 400 with `{"code": "unsupported_platform", "message": "Supported platforms: youtube"}`
