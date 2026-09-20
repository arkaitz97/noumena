## ADDED Requirements

### Requirement: Demand discovery view

The dashboard SHALL provide a demand-discovery view where a workspace member can list watches, create or edit a watch (name, terms, lookback window), view a ranked creator shortlist with its score components, open the evidence for a creator, and export the shortlist. The view SHALL be hidden when the demand-discovery API is disabled.

#### Scenario: View lists watches

- **WHEN** a member opens the demand-discovery view
- **THEN** the view lists the workspace's watches with their status and opens the selected watch's results

#### Scenario: Create and edit a watch

- **WHEN** a member enters a name and terms and saves
- **THEN** the watch is created and its shortlist loads

#### Scenario: Ranked shortlist renders components

- **WHEN** a watch's shortlist loads
- **THEN** each creator row shows the composite rank and the demand intensity, engagement quality, and brand safety components

#### Scenario: Evidence opens

- **WHEN** a member opens evidence for a creator
- **THEN** the matching comments are shown with category, sentiment, and posted-at

#### Scenario: Coverage is visible

- **WHEN** the shortlist loads
- **THEN** the view shows how many opted-in channels were searched

### Requirement: Channel discovery-consent toggle

Channel settings SHALL show the creator's discovery-consent state for each tracked channel and SHALL let the creator change it, explaining that opting in makes the channel eligible for brand demand discovery.

#### Scenario: Toggle on

- **WHEN** a creator enables discovery consent for a channel
- **THEN** the UI reflects the opted-in state after the request succeeds

#### Scenario: Toggle off

- **WHEN** a creator disables discovery consent for a channel
- **THEN** the UI reflects the opted-out state and explains the channel is no longer discoverable
