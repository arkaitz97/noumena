## ADDED Requirements

### Requirement: TikTok channel badges

The channel UI SHALL render a platform badge for each tracked channel and SHALL offer a TikTok connect entry in the channel-connect flow when the platform is enabled.

#### Scenario: TikTok badge rendered
- **WHEN** a user opens channel settings with a connected TikTok channel
- **THEN** the channel card shows a "TikTok" platform badge alongside the channel name

#### Scenario: TikTok connect entry
- **WHEN** the TikTok platform flag is enabled and a user opens the connect modal
- **THEN** the modal offers a TikTok connect option in addition to YouTube