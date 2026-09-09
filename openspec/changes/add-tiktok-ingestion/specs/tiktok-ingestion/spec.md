## ADDED Requirements

### Requirement: TikTok OAuth connect flow

The system SHALL let a user connect a TikTok channel via OAuth 2.0 using the TikTok Research API, following the same state/CSRF, token-encryption, and plan-limit semantics as the YouTube flow. The system SHALL reject the exchange when the OAuth state is invalid, expired, or replayed. The system SHALL store the user's access and refresh tokens encrypted in `user_channel_tracking`. The system SHALL reuse an existing `platform_channels` row when the same real TikTok channel is connected by another user.

#### Scenario: Connect a new TikTok channel
- **WHEN** a user completes the TikTok OAuth exchange for a channel not yet in `platform_channels`
- **THEN** the system creates the platform channel, creates an `active` tracking row with `token_priority = MAX+1`, and enqueues a backfill job

#### Scenario: Connect a TikTok channel already tracked by another user
- **WHEN** the OAuth exchange resolves to an existing `platform_channels` row
- **THEN** the system creates only the tracking row and does NOT enqueue a backfill job

#### Scenario: Invalid OAuth state
- **WHEN** the exchange request carries an unknown, expired, or replayed state
- **THEN** the system returns 400 and creates no records

### Requirement: TikTok token lifecycle

The system SHALL refresh expired TikTok access tokens via the Research API refresh flow and SHALL promote the next tracker's token when the primary token holder pauses, removes, or deletes the tracking. The system SHALL notify the token holder 7 days before expiry using the existing token-expiry notification job.

#### Scenario: Token refresh succeeds
- **WHEN** an ingestion run finds the TikTok access token expired
- **THEN** the system refreshes it and continues the fetch

#### Scenario: Primary token holder leaves
- **WHEN** the primary token holder pauses or removes their TikTok tracking and other active trackers exist
- **THEN** the system promotes the lowest-`token_priority` active tracker to primary and ingestion continues

### Requirement: TikTok backfill window

The system SHALL limit TikTok backfill to a 30-day window, matching the Research API data availability, and SHALL expose that bound in the backfill-window UI and help documentation.

#### Scenario: Backfill requested beyond the window
- **WHEN** a user requests a backfill window larger than 30 days for a TikTok channel
- **THEN** the system clamps the window to 30 days and reflects the clamp in the channel status response

### Requirement: Feature gating

The TikTok surface SHALL be gated by a runtime flag; when disabled, connect/exchange routes SHALL return 503 and the ingestion worker SHALL skip TikTok channels without error.

#### Scenario: TikTok disabled
- **WHEN** `TIKTOK_ENABLED=false` and a user requests the TikTok connect URL
- **THEN** the system returns 503 with a platform-unavailable error body