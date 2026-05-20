# Auth API Reference (Spec 01 — Implementation)

This document describes the runtime contract of the `/auth/*` endpoints as
implemented by `internal/api/auth_handlers.go`. The conceptual spec lives in
`specs/01-auth.md`; this file is the operator/integrator reference.

All bodies are JSON unless otherwise noted. All responses use `Content-Type:
application/json` except 204 No Content responses.

## Cookies

Refresh tokens are delivered exclusively via cookie:

| Attribute  | Value                |
|------------|----------------------|
| Name       | `refresh_token`      |
| Path       | `/auth`              |
| HttpOnly   | true                 |
| Secure     | true                 |
| SameSite   | Lax                  |
| Max-Age    | 30 days (configurable via `JWT_REFRESH_TTL`) |

The Google SSO short-lived state cookie is `google_oauth_state` with the same
attributes and a 10-minute Max-Age.

## Endpoints

### `POST /auth/register`

**Body** `{ "email": string, "password": string }`

Password policy: ≥ 8 chars, ≥ 1 uppercase, ≥ 1 digit (server-enforced).

| Status | Body                                                              |
|--------|-------------------------------------------------------------------|
| 201    | `{ "access_token": <jwt>, "message": "check inbox to verify…" }` (also sets `refresh_token` cookie) |
| 400    | `{ "error": "<reason>" }` — body invalid or password fails policy |
| 409    | `{ "error": "An account with this email already exists." }` *or* `{ "error": "This email is registered with Google. …" }` |

Side effects: a row is inserted in `email_verification_tokens` (24h TTL) and
the verification email is sent to the supplied address.

### `POST /auth/login`

**Body** `{ "email": string, "password": string }`

| Status | Body |
|--------|------|
| 200    | `{ "access_token": <jwt> }` (also rotates `refresh_token` cookie) |
| 400    | invalid request body |
| 401    | `{ "error": "Invalid email or password" }` — generic, used for unknown email, bad password, or locked account |

Side effects: increments `failed_login_attempts` on bad password; on the 5th
failure within 15 minutes sets `locked_until = NOW() + 15 minutes` and sends
the lockout email. Successful login resets the counter and transparently
rehashes the password if the stored bcrypt cost is below the configured cost.

### `POST /auth/refresh`

Reads the `refresh_token` cookie. No request body required.

| Status | Body |
|--------|------|
| 200    | `{ "access_token": <jwt> }` (cookie rotated) |
| 401    | `{ "error": "..." }` (cookie cleared) |

The old refresh row is deleted before a new one is inserted, so a replay of
the old cookie returns 401.

### `POST /auth/logout`

Reads the `refresh_token` cookie. Deletes that single row and clears the
cookie.

| Status |
|--------|
| 204    |

### `POST /auth/logout-all` (authenticated)

Requires a valid access token. Increments `users.token_version` and deletes
every `refresh_tokens` row for the user. All previously issued access tokens
become invalid.

| Status |
|--------|
| 204    |

### `POST /auth/verify-email?token=<raw>`

Hashes the supplied token, looks it up in `email_verification_tokens`, and
sets `users.email_verified_at = NOW()` on success.

| Status | Notes |
|--------|-------|
| 200    | `{ "message": "email verified" }` |
| 410    | unknown / expired / already-used token |

### `POST /auth/resend-verification` (authenticated)

Issues a new 24h verification token, invalidates prior unused tokens.
Rate-limited to 3 issued tokens per user per rolling 60-minute window.

| Status | Notes |
|--------|-------|
| 200    | OK |
| 429    | rate limit exceeded |

### `POST /auth/forgot-password`

**Body** `{ "email": string }`

| Status | Body |
|--------|------|
| 200    | `{ "message": "If an account exists with this email, a reset link has been sent." }` |

Always 200 with the same message regardless of whether the email exists, the
account uses Google SSO, or the rate limit has been hit. Side effects vary
by branch:

- **Existing password account** → inserts a `password_reset_tokens` row (1h
  TTL) and emails the link.
- **Existing Google-only account** → emails the SSO-only notice instead.
- **Unknown email** → no token, no email.
- **More than 3 prior emails in the last hour** → no token, no email.

### `POST /auth/reset-password?token=<raw>`

**Body** `{ "password": string }`

| Status | Body |
|--------|------|
| 200    | `{ "message": "password updated" }` |
| 400    | weak password |
| 410    | unknown / expired / already-used token |

Side effects on success: updates `password_hash`, marks the reset token
used, increments `users.token_version`, deletes every refresh-token row for
the user, and clears `failed_login_attempts` / `locked_until`.

### `GET /auth/google`

Initiates Google OAuth 2.0 PKCE. Generates `state` and the PKCE
`code_verifier`, stores them in the `google_oauth_state` cookie, and
302-redirects the browser to Google with scopes `openid email profile`.

### `GET /auth/google/callback`

Validates the state cookie against the `state` query param, exchanges the
authorization code (with the PKCE verifier) for tokens, verifies the ID
token against Google's JWKS (`aud`, `iss`, `exp`, signature), then routes:

| Branch                                                         | Outcome |
|----------------------------------------------------------------|---------|
| `google_sub` already matches a user                            | sign in, redirect to `${APP_BASE_URL}/dashboard` |
| Email matches an existing **password** account, no `google_sub`| redirect to `${APP_BASE_URL}/auth/link-google?email=…&sub=…` (no session) |
| No matching user                                                | create user (`email_verified_at = NOW()`), redirect to `${APP_BASE_URL}/onboarding` |
| Bad state / invalid id_token                                    | 400 |

### `POST /auth/google/link`

**Body** `{ "email": string, "sub": string }`

Used by the link-confirmation view to attach a Google `sub` to an existing
password account. Sets `users.google_sub` and signs the user in.

## Auth middleware

The middleware on `/api/*`:

1. Parses `Authorization: Bearer <jwt>`.
2. Validates the signature, expiry (with 30s leeway), and the embedded
   `token_version` against the user's current `token_version` in the
   database.
3. If `email_verified_at` is NULL, returns `403 { "code": "email_not_verified" }`.
4. Otherwise attaches `user_id` and `token_version` to the request context.

Routes mounted under `/auth/*` and `/webhooks/*` are exempt from the
verified-email check.

---

## Google SSO operator runbook

1. Create a **dedicated** Google OAuth 2.0 Client ID in Google Cloud Console
   (separate from the YouTube channel-connection client).
2. Authorized redirect URI: `${APP_BASE_URL}/auth/google/callback`.
3. Scopes used: `openid email profile` only.
4. Provide credentials via env: `GOOGLE_SSO_CLIENT_ID`,
   `GOOGLE_SSO_CLIENT_SECRET`, `GOOGLE_SSO_REDIRECT_URL`.

The YouTube channel-connection OAuth client (`internal/oauth/youtube.go`,
env keys `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`) is intentionally
distinct so its scope set and credential rotation are isolated.

## Email deliverability

Transactional emails (verification, password reset, lockout, Google-only
notice) are sent through Resend. To prevent verification emails from being
silently dropped to spam, the sending domain MUST publish:

- **SPF**: include `_spf.resend.com` (or your provider's SPF record) in the
  domain's TXT record.
- **DKIM**: add the CNAME records Resend supplies for the sending domain.
- **DMARC**: at minimum `v=DMARC1; p=none; rua=mailto:dmarc-reports@<your-domain>`;
  raise to `quarantine` after the SPF/DKIM alignment is verified in
  production.

These records must be configured before enabling email-gated routes (the
unverified user cleanup deletes accounts after 7 days).

## Configuration keys

| Key                       | Default | Notes |
|---------------------------|---------|-------|
| `JWT_SECRET`              | (insecure dev default) | Required when `STRICT_CONFIG=1` |
| `JWT_ACCESS_TTL`          | `15m`   | `time.ParseDuration` format |
| `JWT_REFRESH_TTL`         | `720h`  | 30 days |
| `BCRYPT_COST`             | `12`    | Must be ≥ 12 |
| `EMAIL_VERIFICATION_TTL`  | `24h`   | |
| `PASSWORD_RESET_TTL`      | `1h`    | |
| `APP_BASE_URL`            | `http://localhost:3000` | Used to build email links |
| `GOOGLE_SSO_CLIENT_ID`    | —       | |
| `GOOGLE_SSO_CLIENT_SECRET`| —       | |
| `GOOGLE_SSO_REDIRECT_URL` | —       | |
| `STRICT_CONFIG`           | `0`     | Set to `1` in production to fail fast on missing values |
