## MODIFIED Requirements

### Requirement: Accept Invite
The system SHALL allow invited users to join a workspace through a public SPA deep link plus an authenticated acceptance endpoint. The invite token SHALL be single-use with a 7-day TTL and SHALL authorize acceptance only for an authenticated account whose email matches the invited email.

#### Scenario: Invite link opens the SPA
- **WHEN** an invite email link `/invite?token=<raw>` is opened
- **THEN** the SPA SHALL render the invite view and, if the visitor is not authenticated, SHALL preserve the token and route to login or signup with `next` pointing back to the invite view

#### Scenario: Accept Invite (Existing User)
- **WHEN** an authenticated user whose account email matches the invite email calls POST /api/invites/accept with `{token}`
- **THEN** the system SHALL validate the token and email, add the user to the workspace with the specified role via the atomic seat-guarded insert, consume the token, set the workspace active, and return 200 with the workspace id and name

#### Scenario: Accept Invite (New User)
- **WHEN** a new user opens the invite link, completes registration and email verification, and the pending token is submitted
- **THEN** the system SHALL add the user to the workspace with the specified role and route them to onboarding or dashboard for that workspace

#### Scenario: Wrong Signed-In Account
- **WHEN** an authenticated user whose account email does not match the invite email calls the accept endpoint
- **THEN** the system SHALL return 403 and SHALL NOT consume the invite

#### Scenario: Expired or Consumed Invite
- **WHEN** an invite token is older than 7 days, already consumed, or unknown
- **THEN** the system SHALL return 410 "invite expired or already used" and the SPA SHALL offer to request a new invite

#### Scenario: Seat Cap Reached
- **WHEN** acceptance would exceed the workspace plan seat cap
- **THEN** the system SHALL return 409 "seat limit reached, upgrade your plan" and SHALL NOT consume the invite

#### Scenario: Already a Member
- **WHEN** the accepting user is already a member of the workspace
- **THEN** the system SHALL return 200 idempotently, set the workspace active, and SHALL NOT create a duplicate membership row or fail

#### Scenario: Legacy Link Redirect
- **WHEN** GET /invites/accept?token=<raw> is requested
- **THEN** the system SHALL respond 302 to the SPA deep link `${AppBaseURL}/invite?token=<raw>` and SHALL NOT perform any membership change

## ADDED Requirements

### Requirement: Invite Preview
The system SHALL expose a public, rate-limited preview of an invite so the SPA can present the workspace being joined before authentication. The preview MUST NOT disclose whether an account exists for the invited email.

#### Scenario: Preview a Valid Invite
- **WHEN** GET /api/invites/preview?token=<raw> is requested for a valid, unconsumed, unexpired invite
- **THEN** the system SHALL return 200 with the workspace name, invited email, and role

#### Scenario: Preview an Invalid Invite
- **WHEN** the token is unknown, expired, or consumed
- **THEN** the system SHALL return 410 with a generic error and MUST NOT reveal account existence

### Requirement: Invite Email Link Target
The system SHALL send invite emails whose link targets the public SPA deep link rather than the backend accept route.

#### Scenario: Invite Email Link
- **WHEN** an Admin creates an invite
- **THEN** the invite email link SHALL be `${AppBaseURL}/invite?token=<raw>` where `<raw>` is the single-use token
