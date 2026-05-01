## Why

Noumena currently has no authentication system. Without it, no other product capability (channel connection, ingestion, insights, billing) can function securely or be scoped to a user. Spec 01 of the roadmap establishes this foundation: every downstream phase blocks on it.

## What Changes

- Introduce email/password registration and login with bcrypt (cost ≥ 12) password hashing.
- Add Google SSO via OAuth 2.0 PKCE, including new-user creation and "link to existing password account" flow.
- Add mandatory email verification (24h tokens) gating product routes; unverified users are issued JWTs but redirected to `/verify-email`.
- Add password reset via signed email link (1h expiry) with rate limiting.
- Issue short-lived JWT access tokens (15 min) and long-lived refresh tokens (30 day rolling window) stored as SHA-256 hashes in `refresh_tokens`. Refresh rotates and invalidates the old token.
- Add `logout` (revokes current refresh token) and `logout-all` (bumps `users.token_version` to invalidate every refresh token).
- Add brute-force protection: 5 failed logins in 15 min → 15 min account lock + email alert. Generic 401 messages prevent user enumeration.
- Extend the auth middleware to enforce email verification on protected routes (returns 403 `email_not_verified`), exempting `/auth/*` and `/webhooks/*`.
- Add a daily cleanup job that deletes unverified accounts older than 7 days.
- Extend the `users` table (`token_version`, `failed_login_attempts`, `locked_until`, `deletion_requested_at`) and add three new tables: `email_verification_tokens`, `password_reset_tokens`, `refresh_tokens`.
- Add transactional email templates for verification, password reset, and lockout alerts.

## Capabilities

### New Capabilities
- `user-auth`: Account registration, login, session lifecycle (JWT access + rotating refresh tokens), logout, brute-force protection, and the auth middleware contract that gates protected routes.
- `email-verification`: Token-issued email verification, resend with rate limiting, expiry handling, and the unverified-account cleanup job.
- `password-reset`: Forgot-password and reset-password flows with hashed, single-use, time-bound tokens and rate limiting.
- `google-sso`: Google OAuth 2.0 PKCE login/registration, ID token verification, and account linking when an email matches an existing password account.

### Modified Capabilities
<!-- None — this is the first auth capability in the project. -->

## Impact

- **Database**: New columns on `users`; new tables `email_verification_tokens`, `password_reset_tokens`, `refresh_tokens`. Migration required.
- **Backend code**: `internal/api/auth_handlers.go`, `internal/api/middleware.go`, `internal/auth/jwt.go`, `internal/auth/password.go`, `internal/db/db.go`, `internal/db/schema.sql`, `internal/email/client.go`.
- **APIs**: New public routes under `/auth/*` (register, login, refresh, logout, logout-all, verify-email, resend-verification, forgot-password, reset-password, google, google/callback).
- **Dependencies**: `golang.org/x/crypto/bcrypt`, `github.com/golang-jwt/jwt/v5`, `golang.org/x/oauth2`, `google.golang.org/api/oauth2/v2`.
- **Operations**: Daily scheduled job for unverified-account cleanup; outbound transactional email volume increases (verification, reset, lockout alerts).
- **Downstream**: Unblocks Spec 02 (Channels), Spec 07 (Billing), and all downstream phases that require an authenticated user context.
- **Open question (from roadmap)**: Whether the Google OAuth app for SSO is shared with the YouTube channel connection app or kept separate. This proposal assumes a **separate Google OAuth app** dedicated to SSO (cleaner separation of scopes); the YouTube channel OAuth is the responsibility of Spec 02.
