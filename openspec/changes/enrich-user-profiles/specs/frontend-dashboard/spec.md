## MODIFIED Requirements

### Requirement: Onboarding Wizard
The system SHALL guide new users through profile setup and their first channel connection.

#### Scenario: First-Time Onboarding
- **GIVEN** a newly registered user (via Google SSO, auto-verified)
- **WHEN** the user lands on /onboarding
- **THEN** the frontend SHALL present an initial "About your work" profile step before channel connection, allowing the user to provide persona/goals or skip to channel connection

#### Scenario: Profile Step Skipped
- **GIVEN** a user on the onboarding profile step
- **WHEN** the user clicks "Skip for now"
- **THEN** the frontend SHALL proceed directly to channel connection without blocking onboarding progress

#### Scenario: Empty Channel State
- **GIVEN** a user with no connected channels
- **WHEN** the user views the dashboard
- **THEN** the frontend SHALL display an empty state prompting channel connection

## ADDED Requirements

### Requirement: User Profile Settings Editor
The system SHALL provide an interface in Account Settings allowing users to view and update their profile context.

#### Scenario: Edit Profile in Account Settings
- **WHEN** an authenticated user navigates to Account Settings
- **THEN** the frontend SHALL display the current profile fields, allow editing of goals, role, topics, and workflow preferences, and submit updates via `PATCH /api/user/profile` with success notification
