## MODIFIED Requirements

### Requirement: Role-based UI reflects server role

The system SHALL fetch and apply the user's workspace role so role-gated UI (export, member management) matches server enforcement.

#### Scenario: Analyst loads dashboard
- **GIVEN** a user with the `analyst` role
- **WHEN** the dashboard loads
- **THEN** the export control is enabled

#### Scenario: Viewer loads dashboard
- **GIVEN** a user with the `viewer` role
- **WHEN** the dashboard loads
- **THEN** the export control is disabled/hidden

### Requirement: Email change flow

The system SHALL let a user change their account email through `PATCH /user/me`, keeping the old email until the new one is verified.

#### Scenario: Email change requested
- **GIVEN** an authenticated user
- **WHEN** they submit a new email
- **THEN** a verification email is sent, the account stays on the old email, and the new email activates only after verification

### Requirement: Channel reauthorization

The system SHALL let a user re-link an expired or revoked YouTube token to an existing channel without creating a duplicate.

#### Scenario: Expired token re-linked
- **GIVEN** a channel whose OAuth token expired
- **WHEN** the user reauthorizes the channel
- **THEN** the new token is stored on the existing channel tracking row and the expiry banner clears

### Requirement: Export result surfaced

The system SHALL show the user the result of an async export (status, download link, expiry).

#### Scenario: Export completes
- **GIVEN** a requested data export
- **WHEN** the export completes
- **THEN** the UI shows a working download link and expiry, including for partial exports
