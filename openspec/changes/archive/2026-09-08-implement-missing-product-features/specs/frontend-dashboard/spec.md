## ADDED Requirements

### Requirement: Password change from account settings

The system SHALL let a signed-in user change their password from the account settings.

#### Scenario: Correct current password
- **GIVEN** an authenticated user
- **WHEN** they change their password with the correct current password
- **THEN** the password updates, other sessions are invalidated, and the current session remains valid

#### Scenario: Incorrect current password
- **GIVEN** an authenticated user
- **WHEN** they submit an incorrect current password
- **THEN** the change is rejected and no session is invalidated

### Requirement: Token-expiry in-app banner

The system SHALL show an in-app banner when any channel's OAuth token is expiring within the warning window.

#### Scenario: Token near expiry
- **GIVEN** a channel whose token expires within the warning window
- **WHEN** the user loads the dashboard
- **THEN** a banner identifies the channel and offers a reauthorize action
