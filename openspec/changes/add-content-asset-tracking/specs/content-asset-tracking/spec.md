## ADDED Requirements

### Requirement: Content asset persistence

The system SHALL persist one content asset per real video for each tracked platform channel, deduplicated by `(platform_channel_id, platform_asset_id)`. A content asset SHALL record its platform, platform asset id, title, description, url, published-at, and current engagement counts.

#### Scenario: Asset created on discovery

- **WHEN** the ingestion worker discovers a video it has not seen for a channel
- **THEN** the system inserts a content asset with its metadata and `published_at`

#### Scenario: Re-discovery does not duplicate

- **WHEN** the worker discovers a video already stored for that channel
- **THEN** no duplicate asset is created and the existing row is retained

#### Scenario: Asset cascades with its channel

- **WHEN** a `platform_channels` row is deleted
- **THEN** all of its content assets are deleted

### Requirement: Engagement metric refresh

The system SHALL periodically refresh each asset's view, like, and comment counts from the platform, batched to conserve API quota, and SHALL NOT consume quota reserved for comment ingestion when the daily budget is spent.

#### Scenario: Metrics refresh updates counts

- **WHEN** the metrics-refresh pass runs for an asset whose counts changed on the platform
- **THEN** the stored `view_count`, `like_count`, and `comment_count` are updated and `metrics_refreshed_at` advances

#### Scenario: Refresh is batched

- **WHEN** the refresh pass processes multiple assets
- **THEN** it requests statistics in batches rather than one call per asset

#### Scenario: Budget spent defers refresh

- **GIVEN** the daily YouTube quota budget is exhausted
- **WHEN** the metrics-refresh pass runs
- **THEN** it defers without consuming retry budget and without failing ingestion

### Requirement: Sponsored-content signals

The system SHALL detect deterministic sponsored-content markers in an asset's title and description, SHALL set `is_sponsored` when any marker matches, and SHALL store the matched markers verbatim. Detection SHALL NOT produce a suitability score or judgment.

#### Scenario: Marker detected

- **WHEN** an asset's title or description contains a configured marker such as `#ad`, `#sponsored`, or "sponsored by"
- **THEN** `is_sponsored` is true and the matched marker is recorded in `sponsored_signals`

#### Scenario: No marker

- **WHEN** an asset's metadata contains no configured marker
- **THEN** `is_sponsored` is false and `sponsored_signals` is empty

#### Scenario: Signals are the raw matches

- **WHEN** sponsored signals are returned
- **THEN** they list the literal matched markers, not a score or inferred category

### Requirement: Content asset read API

The system SHALL expose read endpoints to list assets for a tracked channel and fetch a single asset, scoped to channels the caller's workspace can access, and SHALL expose an on-demand metric refresh.

#### Scenario: List channel assets

- **WHEN** a member calls `GET /api/assets?channel_id=...` for a channel their workspace can access
- **THEN** the system returns the channel's assets, newest published first, with metrics and sponsored signals

#### Scenario: Fetch one asset

- **WHEN** a member calls `GET /api/assets/{id}` for an accessible asset
- **THEN** the system returns the asset detail

#### Scenario: Cross-workspace access denied

- **WHEN** a member requests an asset whose channel their workspace cannot access
- **THEN** the system returns 403 or 404 without exposing the asset

#### Scenario: On-demand refresh

- **WHEN** a member with at least the `analyst` role calls `POST /api/assets/{id}/refresh`
- **THEN** the system refreshes that asset's metrics within the quota budget and returns the updated asset

### Requirement: Content asset feature gate

The content-asset API SHALL be gated by `CONTENT_ASSET_API_ENABLED`. When disabled, the routes SHALL return 503 and SHALL expose no asset data.

#### Scenario: API disabled

- **WHEN** `CONTENT_ASSET_API_ENABLED=false` and any `/api/assets*` request is made
- **THEN** the system returns 503 with a `content_asset_disabled` code
