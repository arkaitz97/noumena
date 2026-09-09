# Auth Specification

## Purpose
Handles user registration, authentication, session management, email verification, password reset, and Google SSO.
## Requirements
### Requirement: Email/Password Registration
The system SHALL create user accounts with email and password, enforce password policy, and issue a verification email.

#### Scenario: Successful Registration
- **GIVEN** an email not yet registered
- **WHEN** the user submits a valid email and password meeting policy (>=8 chars, >=1 uppercase, >=1 digit)
- **THEN** the system SHALL hash the password with bcrypt (cost >=12), create a user row with `email_verified_at = NULL`, issue JWT access + refresh tokens, send a verification email with a signed link (24h TTL), and return 201 with the access token

#### Scenario: Duplicate Email (Password Account)
- **GIVEN** an email already registered with a password
- **WHEN** a registration attempt is made with that email
- **THEN** the system SHALL return 409 with "An account with this email already exists."

#### Scenario: Duplicate Email (Google SSO Account)
- **GIVEN** an email already registered via Google SSO
- **WHEN** a registration attempt is made with that email
- **THEN** the system SHALL return 409 with "This email is registered with Google. Please sign in with Google."

#### Scenario: Weak Password
- **GIVEN** a password that does not meet policy
- **WHEN** the user submits the registration form
- **THEN** the system SHALL return 400 with the specific policy violation message

### Requirement: Email/Password Login
The system SHALL authenticate users with email and password, enforce brute-force protection, and issue session tokens.

#### Scenario: Successful Login
- **GIVEN** a registered user with verified or unverified email
- **WHEN** the user provides correct email and password
- **THEN** the system SHALL verify the bcrypt hash, reset login failure counters, issue a JWT access token (15min TTL) and refresh token (30-day TTL, HttpOnly/Secure/SameSite=Lax cookie), and return 200 with the access token

#### Scenario: Invalid Credentials
- **GIVEN** any email address
- **WHEN** the user provides a wrong password or non-existent email
- **THEN** the system SHALL return 401 with "Invalid email or password" (generic, no enumeration)

#### Scenario: Brute-Force Lockout
- **GIVEN** a user account
- **WHEN** 5 failed login attempts occur within 15 minutes
- **THEN** the system SHALL lock the account for 15 minutes, send a lockout notification email, and return 401 with the generic error for all subsequent attempts during the lock window

#### Scenario: Login During Account Lockout
- **GIVEN** a user account locked until a future time
- **WHEN** the user attempts login with correct credentials
- **THEN** the system SHALL return 401 with the generic error without checking the password

#### Scenario: Transparent Password Rehash
- **GIVEN** a user whose password hash uses a bcrypt cost below the configured value
- **WHEN** the user logs in successfully
- **THEN** the system SHALL rehash the password with the current cost and update the stored hash

### Requirement: Token Refresh
The system SHALL rotate refresh tokens and reissue access tokens on valid refresh requests.

#### Scenario: Successful Refresh
- **GIVEN** a valid, unexpired refresh token cookie
- **WHEN** the user calls POST /auth/refresh
- **THEN** the system SHALL validate the token hash, delete the old row, issue a new refresh token (rotated) and access token, set the new refresh cookie, and return 200

#### Scenario: Expired Refresh Token
- **GIVEN** an expired or revoked refresh token cookie
- **WHEN** the user calls POST /auth/refresh
- **THEN** the system SHALL clear the cookie and return 401 with "invalid or expired refresh token"

### Requirement: Logout
The system SHALL revoke session tokens on logout requests.

#### Scenario: Single Device Logout
- **GIVEN** an authenticated user with a refresh token cookie
- **WHEN** the user calls POST /auth/logout
- **THEN** the system SHALL revoke that specific refresh token, clear the cookie, and return 204

#### Scenario: Logout All Devices
- **GIVEN** an authenticated user
- **WHEN** the user calls POST /auth/logout-all
- **THEN** the system SHALL increment the user's token_version, delete all refresh tokens, clear the cookie, and return 204. All existing access tokens with the old version SHALL become invalid.

### Requirement: Email Verification
The system SHALL require email verification before granting product access.

#### Scenario: Successful Verification
- **GIVEN** an unverified user with a valid, unexpired verification token
- **WHEN** the user clicks the verification link (GET /auth/verify-email?token=...)
- **THEN** the system SHALL consume the token (one-time use), set `email_verified_at = NOW()`, and return 200

#### Scenario: Expired Verification Token
- **GIVEN** a verification token older than 24 hours
- **WHEN** the user clicks the verification link
- **THEN** the system SHALL return 410 with "invalid or expired token"

#### Scenario: Resend Verification Email
- **GIVEN** an authenticated but unverified user
- **WHEN** the user calls POST /auth/resend-verification
- **THEN** the system SHALL invalidate old tokens, generate a new token (24h TTL), send a verification email, and return 200. If the user has already requested 3 resends within the last hour, the system SHALL return 429.

### Requirement: Password Reset
The system SHALL allow users to reset passwords via email link without email enumeration.

#### Scenario: Successful Password Reset Request
- **GIVEN** a registered user with a password
- **WHEN** the user submits their email to POST /auth/forgot-password
- **THEN** the system SHALL invalidate old reset tokens, generate a new cryptographically random token (SHA-256 hashed in DB, 1h TTL), send a reset email, and return 200 with "If an account exists with this email, a reset link has been sent."

#### Scenario: Password Reset for Non-Existent Email
- **GIVEN** an email not registered in the system
- **WHEN** the user submits it to POST /auth/forgot-password
- **THEN** the system SHALL return the same 200 response as for a valid email (no enumeration)

#### Scenario: Successful Password Reset
- **GIVEN** a valid, unused reset token
- **WHEN** the user submits the token and a new valid password to POST /auth/reset-password
- **THEN** the system SHALL consume the token (one-time), update the password hash, increment token_version, delete all refresh tokens, reset login failures, and return 200

#### Scenario: Google-Only Account Password Reset
- **GIVEN** a user registered only via Google SSO (no password_hash)
- **WHEN** the user submits their email to POST /auth/forgot-password
- **THEN** the system SHALL send a notice email explaining the account uses Google Sign-In and return the generic 200 response

#### Scenario: Password Reset Rate Limit
- **GIVEN** any email address
- **WHEN** 3+ reset emails have been sent within the last hour
- **THEN** the system SHALL return the generic 200 response without sending another email

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

### Requirement: Auth Middleware
The system SHALL validate JWT access tokens on protected routes.

#### Scenario: Valid Access Token
- **GIVEN** a valid, unexpired JWT with matching token_version
- **WHEN** a request includes it in the Authorization header as "Bearer <token>"
- **THEN** the middleware SHALL parse the token, verify the token_version matches the database, attach user_id to context, and pass the request through

#### Scenario: Missing Authorization Header
- **GIVEN** a request to a protected route
- **WHEN** the Authorization header is absent
- **THEN** the middleware SHALL return 401 with "authorization header missing"

#### Scenario: Revoked Token Version
- **GIVEN** a JWT issued before a logout-all or password-reset
- **WHEN** the user makes a request with that token
- **THEN** the middleware SHALL detect the token_version mismatch and return 401

### Requirement: Email Verification Gate
The system SHALL block unverified users from accessing product routes.

#### Scenario: Unverified User Access
- **GIVEN** an authenticated user with `email_verified_at = NULL`
- **WHEN** the user accesses a route behind the RequireVerifiedEmail middleware
- **THEN** the system SHALL return 403 with `{"code": "email_not_verified"}`

### Requirement: User Profile
The system SHALL provide the authenticated user's profile information.

#### Scenario: Get Profile
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/user/me
- **THEN** the system SHALL return the user's id, email, email_verified status, subscription_plan, subscription_status, deletion_requested_at, data_export_url, data_export_expires_at, and created_at

#### Scenario: Get Minimal Profile (Auth Route)
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /auth/me
- **THEN** the system SHALL return id, email, and email_verified (no email-verification gate, always available for SPA auth-store hydration)

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

