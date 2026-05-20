# Auth API Reference

> Source of truth: [`specs/01-auth.md`](../../specs/01-auth.md),
> [`specs/01-auth-api.md`](../../specs/01-auth-api.md), and the
> `auth-google-popup-endpoints` OpenSpec change.
> Handler: [`internal/api/auth_handlers.go`](../../internal/api/auth_handlers.go),
> routes: [`cmd/api/main.go`](../../cmd/api/main.go).

All `/auth/*` routes are mounted at the root of the HTTP server (no
`/api` prefix). They issue an opaque refresh token in an `HttpOnly;
Secure; SameSite=Lax` cookie scoped to `/auth`, plus a JWT access token
in the JSON response body.

Refresh-token cookie name: `refresh_token`. The refresh-token row is
revoked on rotation, on `POST /auth/logout`, and en-masse on
`POST /auth/logout-all` and `POST /auth/reset-password`.

## Email + password

### POST /auth/register

- **Auth:** none.
- **Request:** `{ "email": string, "password": string }`. Password
  policy is enforced server-side (`auth.ValidatePasswordPolicy`).
- **Response:** `201 Created`,
  `{ "access_token": string, "message": "check inbox to verify account" }`.
  Sets `refresh_token` cookie. A verification email is dispatched
  asynchronously via the email pipeline; failure to enqueue does **not**
  fail the request.
- **Errors:**
  - `400` — invalid body, empty email/password, or password-policy
    violation.
  - `409` — `An account with this email already exists.` (password
    account) or `This email is registered with Google. Please sign in
    with Google.` (Google-only account).
  - `500` — hash or token issuance failure.

```sh
curl -i -X POST https://api.noumena/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"correct horse battery staple 9"}'
```

### POST /auth/login

- **Auth:** none.
- **Request:** `{ "email": string, "password": string }`.
- **Response:** `200 OK`, `{ "access_token": string }`. Sets
  `refresh_token` cookie. Successful login clears the failed-login
  counter and silently rehashes the password if the cost factor has
  drifted below the configured target.
- **Errors:**
  - `401 Unauthorized — Invalid email or password` for: unknown user,
    bad password, locked account, or Google-only account. The same
    message is returned in every branch so the response cannot be used
    to enumerate emails.
  - On the 5th failure within 15 minutes the user is locked for 15
    minutes and a lockout email is sent.

### POST /auth/refresh

- **Auth:** valid `refresh_token` cookie.
- **Request body:** none.
- **Response:** `200 OK`, `{ "access_token": string }`. The refresh
  token is rotated: the old token is revoked and a new one is set in
  the cookie. Reuse of a revoked token returns `401` and clears the
  cookie.
- **Errors:** `401` for missing/invalid/expired token; `500` for token
  issuance failure.

### POST /auth/logout

- **Auth:** valid `refresh_token` cookie (best effort — call still
  returns 204 if absent).
- **Response:** `204 No Content`. The refresh token is revoked and the
  cookie is cleared.

### POST /auth/logout-all

- **Auth:** access-token JWT (`Authorization: Bearer …`).
- **Response:** `204 No Content`. Increments `users.token_version`
  (invalidating every outstanding access token) and deletes every
  refresh token row for the user.

### POST /auth/verify-email

- **Auth:** none — auth is the one-time token in the URL.
- **Request:** `?token=<raw>` (no body required).
- **Response:** `200 OK`, `{ "message": "email verified" }`.
- **Errors:** `410 Gone — invalid or expired token`. Tokens are
  consumed on first use.

### POST /auth/resend-verification

- **Auth:** access-token JWT.
- **Response:** `200 OK`. Returns `{ "message": "already verified" }`
  when the email is already verified, or
  `{ "message": "verification email sent" }` after issuing a fresh
  token.
- **Errors:** `429` once 3 resends have been issued within the last
  hour for the calling user.

### POST /auth/forgot-password

- **Auth:** none.
- **Request:** `{ "email": string }`.
- **Response:** `200 OK`,
  `{ "message": "If an account exists with this email, a reset link has been sent." }`
  — returned regardless of whether the account exists, to avoid email
  enumeration. Rate-limited at 3 emails per address per hour. For
  Google-only accounts, a "this account uses SSO" notice is sent
  instead of a reset link.

### POST /auth/reset-password

- **Auth:** none — auth is the one-time token in the URL.
- **Request:** `?token=<raw>` query string + `{ "password": string }`.
- **Response:** `200 OK`, `{ "message": "password updated" }`.
  Increments `users.token_version` and deletes every existing refresh
  token, forcing every other session to log in again.
- **Errors:** `410` for invalid/expired token; `400` for password
  policy violation.

## Google SSO — redirect mode

Two redirect-flow endpoints exist for clients that prefer a full-page
redirect to Google's consent screen. State + PKCE verifier are
persisted in a signed `google_oauth_state` cookie (10-minute TTL).

### GET /auth/google

- **Auth:** none.
- **Effect:** Redirects (`302`) to Google's authorization endpoint with
  PKCE + state. Returns `503` when `GOOGLE_SSO_CLIENT_ID` is unset.

### GET /auth/google/callback

- **Auth:** Google supplies `?code=&state=` plus the
  `google_oauth_state` cookie set by `/auth/google`.
- **Outcome — terminal state determines the redirect:**
  - **Returning user / re-link existing Google user** → sign in, set
    `refresh_token` cookie, and `302` to `${APP_BASE_URL}/dashboard`.
  - **New user** → create row, sign in, and `302` to
    `${APP_BASE_URL}/onboarding`.
  - **Email collides with a password-only account** → `302` to
    `${APP_BASE_URL}/auth/link-google?email=…&sub=…` (the SPA then
    confirms via `POST /auth/google/link`).
- **Errors:** `400` for `state mismatch`, `missing code`, or
  `invalid id token`; `500` for `failed to create user`.

## Google SSO — popup mode (`auth-google-popup-endpoints`)

Popup-mode endpoints return JSON instead of `302` redirects so the
SPA can drive a `window.open(...)` flow without losing scroll/route
state. The state cookie is shared with the redirect handlers.

### GET /auth/google/auth-url

- **Auth:** none.
- **Response:** `200 OK`,
  `{ "auth_url": string, "state": string }`. The SPA opens `auth_url`
  in a popup and receives the `code` via a one-shot `postMessage`
  back to the opener.
- **Errors:** `503` when `GOOGLE_SSO_CLIENT_ID` is unset; `500` for
  PKCE/cookie failures.

### POST /auth/google/callback

- **Auth:** none — the popup posts the harvested
  `{ code, state }` back here. The `google_oauth_state` cookie set by
  `GET /auth/google/auth-url` MUST still be present.
- **Request:** `{ "code": string, "state": string }`.
- **Response:** `200 OK`, `{ "access_token": string }`. Sets the
  `refresh_token` cookie just like `POST /auth/login`.
- **Errors:**
  - `400 invalid_state` — state cookie missing/mismatched.
  - `400 invalid_code` — request body missing `code`.
  - `400 invalid_id_token` — Google ID-token verification failed.
  - `409 link_required` with `{ "error": "link_required", "email":
    string, "sub": string }` — the email already has a password-only
    account; the SPA must call `POST /auth/google/link` after user
    confirmation.
  - `500 failed to create user` / `failed to issue access token`.

### POST /auth/google/link

- **Auth:** none — the SPA passes the same `(email, sub)` returned by
  `link_required` after the user explicitly confirms the link.
- **Request:** `{ "email": string, "sub": string }`.
- **Response:** `200 OK`, `{ "message": "linked" }`. Sets the
  `refresh_token` cookie. The `X-Access-Token` response header carries
  the new access token (so SPAs that arrived here via a redirect can
  pick it up without parsing the body).
- **Errors:** `400 no matching account`; `500` on link/sign-in
  failure.

### GET /auth/me

- **Auth:** access-token JWT only — no email-verification gate, no
  `ACCOUNT_LIFECYCLE_API_ENABLED` gate. Always available, so the SPA
  can hydrate the auth store immediately after login.
- **Response:** `200 OK`,
  `{ "id": string, "email": string, "email_verified": bool }`.
- **Errors:** `401 unauthorized`.

```sh
curl -i https://api.noumena/auth/me \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

## Related

- Configuration: see [`.env.example`](../../.env.example) for
  `JWT_SECRET`, `APP_BASE_URL`, `GOOGLE_SSO_CLIENT_ID`,
  `GOOGLE_SSO_CLIENT_SECRET`, `GOOGLE_SSO_REDIRECT_URL`, and
  `GOOGLE_SSO_POPUP_REDIRECT_URL`.
- Operations: account-deletion runbook lives at
  [`../ops/runbooks/account-deletion-and-gdpr.md`](../ops/runbooks/account-deletion-and-gdpr.md).
