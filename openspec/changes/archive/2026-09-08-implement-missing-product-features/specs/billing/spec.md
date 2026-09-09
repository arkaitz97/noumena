## ADDED Requirements

### Requirement: Billing usage history

The system SHALL record per-period usage and expose it to the user.

#### Scenario: Usage history requested
- **GIVEN** an authenticated user with billing enabled
- **WHEN** `GET /api/billing/usage` is called
- **THEN** recent periods with comments used and quota are returned

### Requirement: Workspace seat limits

The system SHALL enforce plan seat limits on workspace membership.

#### Scenario: Invite at seat capacity
- **GIVEN** a workspace at its plan seat limit
- **WHEN** a new member is invited or role-changed to member
- **THEN** the action is rejected with a seat-limit error

### Requirement: Overage invoicing

The system SHALL invoice logged overage usage for billable plans.

#### Scenario: Overage accrues
- **GIVEN** a Creator/Studio account with overage in a period
- **WHEN** the overage-invoicing job runs
- **THEN** an invoice/charge is created exactly once per (user, period)

## MODIFIED Requirements

### Requirement: Trial lifecycle

The system SHALL assign trials to new accounts and surface trial state.

#### Scenario: New account signs up
- **GIVEN** a new account not created via invite
- **WHEN** the account is created
- **THEN** a trial period (14 days) is assigned

#### Scenario: Trial countdown
- **GIVEN** an account in its trial period
- **WHEN** the billing view is shown
- **THEN** the remaining trial days are displayed
