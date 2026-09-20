## MODIFIED Requirements

### Requirement: Resume Channel Tracking
The system SHALL allow users to resume a paused channel and SHALL preserve the channel's single-primary invariant: after a resume there SHALL be exactly one active tracker with `token_priority = 0`. If no active priority-0 tracker remains (for example, the previous primary was paused, which demotes it to priority 9999), the resumed tracker SHALL become the primary; otherwise the resumed tracker SHALL join after the highest active priority.

#### Scenario: Resume Paused Channel
- **GIVEN** a user with a paused channel
- **WHEN** the user calls POST /api/channels/{id}/resume
- **THEN** the system SHALL set `tracking_status = 'active'` and return 200

#### Scenario: Resume restores the primary when none remains
- **GIVEN** a channel whose previous primary was paused (priority 9999) and whose only other active tracker has priority 1
- **WHEN** the previous primary is resumed
- **THEN** that tracker SHALL become active with `token_priority = 0`, so the claim query can select the channel's comments again

#### Scenario: Resume joins as a secondary when a primary exists
- **GIVEN** a channel with an active `token_priority = 0` tracker
- **WHEN** another paused tracker is resumed
- **THEN** the resumed tracker SHALL receive `MAX(active token_priority) + 1` and SHALL NOT displace the existing primary
