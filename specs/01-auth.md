# Spec: Core Authentication & User Management

## Overview
Implement the complete authentication system for Noumena including email/password registration, Google SSO, email verification, password reset, JWT session management, and brute-force protection.

## Requirements
- AUTH-01: Users register with email + password or Google SSO. Passwords hashed with bcrypt (cost factor ≥ 12). Google OAuth 2.0 PKCE flow.
- AUTH-02: Email verification required before accessing the product.
- AUTH-03: Password reset via email link (expires in 1 hour).
- AUTH-04: Sessions expire after 30 days of inactivity. JWT access tokens expire in 15 minutes; refresh tokens in 30 days with rolling window.
- AUTH-05: Multi-seat workspaces (Should have — can be deferred to later spec).

## Database Changes

Add to `users` table (existing columns should be preserved):
- `token_version INT NOT NULL DEFAULT 0` — incremented to invalidate all refresh tokens globally
- `failed_login_attempts INT NOT NULL DEFAULT 0`
- `locked_until TIMESTAMPTZ`
- `deletion_requested_at TIMESTAMPTZ`

Create new table:
```sql
CREATE TABLE email_verification_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL, -- SHA-256 of the raw token
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL, -- SHA-256 of the raw token
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## API Endpoints

### Public Routes (no auth)

#### POST /auth/register
- Body: `{ "email": string, "password": string }`
- Password requirements: min 8 chars, 1 uppercase, 1 number (validated server-side)
- Returns 201 with `{ "access_token": string, "message": "check inbox to verify account" }`
- Sets HttpOnly refresh token cookie
- Edge Cases:
  - Email already registered (password): 409, message "An account with this email already exists."
  - Email already registered via Google: 409 with hint to use Google sign-in

#### POST /auth/login
- Body: `{ "email": string, "password": string }`
- Returns 200 with `{ "access_token": string }` and refresh cookie
- Brute-force protection: after 5 failed attempts within 15 minutes, lock account for 15 minutes. Send email alert.
- Returns generic 401 "Invalid email or password" — never distinguish which field is wrong
- If email is unverified: login succeeds (JWT issued) but product routes redirect to `/verify-email`

#### POST /auth/refresh
- Reads `refresh_token` HttpOnly cookie
- Validates refresh token against stored hash and token_version
- Returns new access token; may rotate refresh token
- If refresh token expired or revoked: 401, client redirects to login

#### POST /auth/logout
- Clears refresh token cookie
- Revokes current refresh token (delete from DB)

#### POST /auth/logout-all
- Requires authentication
- Increments `users.token_version` by 1
- All existing refresh tokens for this user become invalid

#### POST /auth/verify-email
- Query param: `token` (the raw verification token)
- Hashes token and looks up in `email_verification_tokens`
- Validates not expired and not used
- Sets `users.email_verified_at = NOW()`, marks token used
- Redirects to onboarding wizard
- Edge Cases:
  - Expired token (24h): 410, UI shows resend option. Generate fresh token on resend.
  - Already used: 410

#### POST /auth/resend-verification
- Requires authentication (but not verified)
- Generates new token, invalidates old ones for this user
- Rate limit: max 3 per hour per user

#### POST /auth/forgot-password
- Body: `{ "email": string }`
- Always returns 200: "If an account exists with this email, a reset link has been sent."
- Rate limit: 3 reset emails per hour per email address
- If user has Google SSO only (password_hash IS NULL): send email explaining they use Google sign-in

#### POST /auth/reset-password
- Query param: `token`
- Body: `{ "password": string }`
- Validates token (not expired, not used)
- Updates `password_hash`, marks token used
- Invalidates all existing refresh tokens (increment token_version)
- Redirects to login with success message
- Edge Cases:
  - Expired (>1h): 410
  - Already used: 410

### Google SSO Routes

#### GET /auth/google
- Initiates Google OAuth 2.0 PKCE flow
- Generates state parameter with CSRF token
- Redirects to Google consent screen

#### GET /auth/google/callback
- Exchanges code for ID token
- Verifies token signature against Google's JWKS endpoint
- Extracts `sub` and email:
  - New user: create with `google_sub`, `email_verified_at = NOW()`, redirect to onboarding
  - Returning user: issue JWTs, redirect to dashboard
  - Existing password account with same email: prompt to link Google account (set `google_sub` on confirmation)

## Backend Implementation

### JWT Tokens
- Access token: 15-minute TTL, contains `user_id`, `token_version`
- Refresh token: cryptographically random 32-byte string, SHA-256 hashed in DB, 30-day expiry, rolling window extends on refresh
- Store refresh token hash in a new table:
```sql
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Email Verification Flow
- On registration: generate random 32-byte token, hash with SHA-256, store in DB, send raw token in signed link
- Link format: `https://app.noumena.com/verify-email?token=<raw_token>`
- Frontend view calls `POST /auth/verify-email` with the token

### Unverified Account Cleanup
- Scheduled job (daily) deletes unverified accounts older than 7 days to prevent email enumeration

### Auth Middleware (`internal/api/middleware.go`)
- Update `AuthMiddleware` to also check `users.email_verified_at` for protected routes (return 403 with code `email_not_verified` for routes that require it)
- Exclude `/auth/*` and `/webhooks/*` from verification check

### Token Rotation
- On `/auth/refresh`: issue new access token + new refresh token; delete old refresh token from DB

## Frontend Requirements

### Views to create/update
- `/signup` — Email/password registration form with real-time validation
- `/login` — Login form
- `/verify-email` — Holding page shown post-registration and if unverified user tries to access product
- `/forgot-password` — Email input, generic success message
- `/reset-password` — New password + confirm, with token validation

### Auth Store (Pinia)
- Store access token in memory (not localStorage)
- On 401 from API, silently call `/auth/refresh`
- On refresh failure, redirect to `/login`
- Handle `email_not_verified` 403 by redirecting to `/verify-email`

## Testing Checklist
- [ ] Registration with valid email/password creates user, sends verification
- [ ] Duplicate email returns 409
- [ ] Login with correct credentials returns access token + refresh cookie
- [ ] Login with wrong password increments failed attempts; 5th failure locks account
- [ ] Refresh token rotation works
- [ ] Logout clears cookie and invalidates refresh token
- [ ] Logout-all invalidates all sessions
- [ ] Email verification succeeds with valid token
- [ ] Expired verification token returns 410 and allows resend
- [ ] Password reset full flow works end-to-end
- [ ] Google SSO new user registration works
- [ ] Google SSO linking to existing password account works

## Dependencies
- `golang.org/x/crypto/bcrypt`
- `github.com/golang-jwt/jwt/v5`
- `golang.org/x/oauth2`
- `google.golang.org/api/oauth2/v2` (for Google token verification)

## Files to Create/Modify
- `internal/api/auth_handlers.go` — extend with all new endpoints
- `internal/auth/jwt.go` — update token generation/validation, add refresh token logic
- `internal/auth/password.go` — ensure bcrypt cost ≥ 12
- `internal/db/db.go` — add DBQuery methods for tokens, token_version, lock fields
- `internal/db/schema.sql` — add new tables and columns
- `internal/email/client.go` — add verification and reset email templates
- `frontend/src/views/auth/*.vue` — create/update auth views
- `frontend/src/services/api.ts` — add auth endpoints, interceptors
