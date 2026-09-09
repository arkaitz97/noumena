## ADDED Requirements

### Requirement: TikTok as a connectable platform

The channel management surface SHALL support TikTok alongside YouTube for connect, exchange, list, pause, resume, remove, reauthorize, and status operations. The 501 placeholder handlers SHALL be replaced by real handlers that enforce the same plan-limit and shared-channel rules as YouTube.

#### Scenario: List includes TikTok channels
- **WHEN** a user lists their tracked channels after connecting a TikTok channel
- **THEN** the response includes the TikTok channel with `platform = "tiktok"` and the same status fields as YouTube channels

#### Scenario: Placeholder removal
- **WHEN** the TikTok connect or exchange route is hit after this change
- **THEN** the system responds with the real flow result, never 501