## ADDED Requirements

### Requirement: Public Invite Route
The SPA SHALL expose a public `/invite` route that is reachable without authentication and reads the invite token from the query string. The route MUST NOT be gated by `requiresAuth`, `requiresVerified`, or `requiresOnboarded`.

#### Scenario: Unauthenticated visitor opens the invite route
- **WHEN** an unauthenticated visitor navigates to `/invite?token=<raw>`
- **THEN** the SPA SHALL NOT redirect to the landing page, SHALL preserve the token, and SHALL send the visitor to login with `next=/invite?token=<raw>`

#### Scenario: Authenticated visitor opens the invite route
- **WHEN** an authenticated visitor navigates to `/invite?token=<raw>`
- **THEN** the SPA SHALL attempt acceptance using the authenticated session

### Requirement: Pending Invite Persistence
The SPA SHALL persist the pending invite token across the login, signup, email-verification, and OAuth round-trips without placing it in the URL of authenticated requests.

#### Scenario: Token survives verification round-trip
- **WHEN** an unauthenticated invitee is routed to signup or verification and later returns authenticated
- **THEN** the SPA SHALL recover the pending token from `sessionStorage` and complete acceptance without requiring a second click of the email link

#### Scenario: Token cleared after resolution
- **WHEN** acceptance succeeds, or the invite is rejected as expired, consumed, or belonging to another account
- **THEN** the SPA SHALL clear the pending token from `sessionStorage`

### Requirement: Authenticated Acceptance
The SPA SHALL submit the pending token to `POST /api/invites/accept` using the authenticated API client, and SHALL refresh the workspace context on success.

#### Scenario: Successful acceptance
- **WHEN** the accept endpoint returns 200 with the workspace id
- **THEN** the SPA SHALL refresh the user's workspace list and active workspace, clear the pending token, and navigate to the dashboard or onboarding for that workspace

#### Scenario: Unverified email
- **WHEN** the accept request is rejected with the `email_not_verified` code
- **THEN** the SPA SHALL route the user to email verification while keeping the pending token for retry after verification

### Requirement: Invite Error Handling
The SPA SHALL present a clear, non-enumerating message for each invite failure and offer the appropriate next action.

#### Scenario: Wrong account
- **WHEN** the accept endpoint returns 403 because the signed-in account email differs from the invited email
- **THEN** the SPA SHALL explain that the invite belongs to a different email and offer to sign out and switch accounts without clearing the pending token

#### Scenario: Seat limit reached
- **WHEN** the accept endpoint returns 409
- **THEN** the SPA SHALL explain that the workspace has no free seats and that an Admin must upgrade the plan

#### Scenario: Expired or consumed invite
- **WHEN** the accept endpoint or preview returns 410
- **THEN** the SPA SHALL explain that the invite has expired or was already used and offer to request a new invite
