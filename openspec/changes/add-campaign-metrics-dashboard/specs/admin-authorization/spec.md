## ADDED Requirements

### Requirement: Global administrator role

The system SHALL support a global platform administrator flag on a user account, distinct from workspace membership roles. The flag SHALL default to false and SHALL be stored on the user record.

#### Scenario: Existing users are not administrators

- **WHEN** the flag is introduced
- **THEN** every existing user has administrator access disabled

#### Scenario: Administrator flag is persisted

- **WHEN** a user record is loaded
- **THEN** the record exposes whether the user is a global administrator

### Requirement: Administrator authorization gate

The system SHALL restrict administrator endpoints to authenticated, verified users who are global administrators. A user SHALL be authorized when their administrator flag is set or their email is present in the operator-configured bootstrap allowlist. Non-administrators SHALL receive 403 and no campaign data.

#### Scenario: Administrator with the flag is allowed

- **WHEN** an authenticated verified user whose administrator flag is set requests an administrator endpoint
- **THEN** the request is allowed

#### Scenario: Bootstrap allowlist grants access

- **WHEN** an authenticated verified user whose email is in the configured allowlist requests an administrator endpoint and the user's flag is not set
- **THEN** the request is allowed

#### Scenario: Non-administrator is denied

- **WHEN** an authenticated verified user who is neither flagged nor allowlisted requests an administrator endpoint
- **THEN** the system responds 403 and returns no campaign data

#### Scenario: Unauthenticated request is denied

- **WHEN** an unauthenticated request is sent to an administrator endpoint
- **THEN** the system responds 401

### Requirement: Administrator access audit

The system SHALL record an audit entry for each administrator campaign data access, including the acting user id and the action.

#### Scenario: Admin campaign access is logged

- **WHEN** an administrator successfully reads campaign data
- **THEN** an audit entry with action `admin_campaign_view` and the administrator's user id is written

### Requirement: Administrator flag exposure

The system SHALL expose the current user's administrator status to the authenticated client so the client can gate administrator navigation and routes. The server gate remains authoritative.

#### Scenario: Current-user endpoint reports admin status

- **WHEN** an authenticated user requests their current-user profile
- **THEN** the response includes a boolean administrator indicator

#### Scenario: Non-admin indicator is false

- **WHEN** an authenticated non-administrator requests their current-user profile
- **THEN** the administrator indicator is false
