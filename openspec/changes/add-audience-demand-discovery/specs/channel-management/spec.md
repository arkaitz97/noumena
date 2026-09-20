## ADDED Requirements

### Requirement: Channel discovery consent

The system SHALL record a per-tracking-row discovery-consent state, defaulting to not opted in. A user SHALL be able to opt a channel they track in or out of brand-facing demand discovery, and the consent state SHALL be reflected in the channel status response.

#### Scenario: Default consent is off

- **GIVEN** a newly connected channel
- **WHEN** its consent state is read
- **THEN** it is not opted in, and the channel is excluded from discovery until the user opts in

#### Scenario: Opt in a tracked channel

- **WHEN** a user calls `PATCH /api/channels/{id}/discovery-consent` with `{opted_in: true}`
- **THEN** the system upserts the consent row for the user's tracking row with `opted_in = true` and an `opted_in_at` timestamp, and returns 200

#### Scenario: Opt out

- **WHEN** a user calls `PATCH /api/channels/{id}/discovery-consent` with `{opted_in: false}`
- **THEN** the system sets `opted_in = false` and the channel is excluded from subsequent discovery

#### Scenario: Consent on a non-tracked channel

- **WHEN** a user sets consent for a channel they do not track
- **THEN** the system returns 404 and creates no consent row

#### Scenario: Consumed consent state

- **WHEN** a user calls `GET /api/channels/{id}/status` for a channel they track
- **THEN** the response includes the caller's current discovery-consent state
