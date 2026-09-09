## MODIFIED Requirements

### Requirement: Google SSO

The system SHALL support Google OAuth 2.0 login/registration with PKCE flow, and SHALL only link a Google identity to an existing password account when the caller is authenticated as that account AND provides a verified Google ID token whose `sub` matches the requested `sub`.

#### Scenario: New User via Google SSO
- **GIVEN** a Google account not yet registered in Noumena
- **WHEN** the user completes the Google OAuth flow (PKCE, ID token verified against JWKS)
- **THEN** the system SHALL create a user row with `google_sub`, `email_verified_at = NOW()`, issue tokens, and redirect to /onboarding

#### Scenario: Returning User via Google SSO
- **GIVEN** a user previously registered via Google SSO
- **WHEN** the user completes the Google OAuth flow
- **THEN** the system SHALL look up by `google_sub`, issue tokens, and redirect to /dashboard

#### Scenario: Email Collision with Password Account
- **GIVEN** a Google account whose email matches an existing password-only account
- **WHEN** the user completes the Google OAuth flow
- **THEN** the system SHALL surface the collision (link_required) to the client along with the server-verified ID token; linking completes only after the user authenticates as the existing account

#### Scenario: Unauthenticated Link Request
- **GIVEN** no authenticated session
- **WHEN** a client calls the Google account-linking endpoint
- **THEN** the request is rejected with 401 and no account mutation occurs

#### Scenario: Authenticated Request with Mismatched Sub
- **GIVEN** an authenticated session for user A
- **WHEN** a client attempts to link a Google `sub` that does not match the verified ID token's subject
- **THEN** the request is rejected and no account mutation occurs

#### Scenario: Authenticated Request with Valid Proof
- **GIVEN** an authenticated session for user A and a verified Google ID token with matching `sub`
- **WHEN** the link request is submitted
- **THEN** the `google_sub` is set on user A and the request completes successfully

#### Scenario: Invalid ID Token
- **GIVEN** a tampered or invalid Google ID token
- **WHEN** the callback attempts to verify it against Google's JWKS
- **THEN** the system SHALL reject with 400 and abort the flow

## ADDED Requirements

### Requirement: Mandatory JWT signing secret

The system SHALL refuse to start when a valid JWT signing secret is not provided. A valid secret SHALL be at least 32 bytes and MUST NOT equal the legacy default or any documented placeholder.

#### Scenario: Missing JWT secret
- **GIVEN** `JWT_SECRET` is unset
- **WHEN** the application starts
- **THEN** startup fails with a clear error naming the required variable

#### Scenario: Placeholder or short JWT secret
- **GIVEN** `JWT_SECRET` is the legacy default, a placeholder, or shorter than 32 bytes
- **WHEN** the application starts
- **THEN** startup fails and the process does not serve traffic

### Requirement: Random workspace invite tokens

The system SHALL generate workspace invite tokens from a cryptographically secure random source, such that tokens are unique and unpredictable.

#### Scenario: Two invites are generated
- **GIVEN** two workspace invites are created
- **WHEN** their tokens are compared
- **THEN** the tokens differ and neither equals the legacy deterministic value