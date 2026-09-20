## ADDED Requirements

### Requirement: Channel asset panel

The channel detail view SHALL show the channel's content assets with their title, published date, engagement counts, and a sponsored badge when `is_sponsored` is true, and SHALL let a member with sufficient role trigger an on-demand metric refresh. The panel SHALL be hidden when the content-asset API is disabled.

#### Scenario: Assets listed

- **WHEN** a member opens a channel whose assets are loaded
- **THEN** the view lists assets newest first with views, likes, and comment counts

#### Scenario: Sponsored badge

- **WHEN** an asset has `is_sponsored = true`
- **THEN** the asset row displays a sponsored badge

#### Scenario: Refresh control

- **WHEN** a member with at least the `analyst` role triggers refresh on an asset
- **THEN** the displayed metrics update after the request succeeds
