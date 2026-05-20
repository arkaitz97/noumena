# User Account API Reference

> Source of truth: [`specs/08-data-lifecycle.md`](../../specs/08-data-lifecycle.md).
> Handlers:
> [`internal/api/account_handlers.go`](../../internal/api/account_handlers.go)
> and the `Me` handler in
> [`internal/api/auth_handlers.go`](../../internal/api/auth_handlers.go).

The `/api/user/*` endpoints are gated by
`ACCOUNT_LIFECYCLE_API_ENABLED` (route registration in
[`cmd/api/main.go`](../../cmd/api/main.go)). When the flag is off,
these routes return `404` from chi; the underlying lifecycle workers
can still be enabled or disabled independently via
`LIFECYCLE_WORKERS_ENABLED`. Both flags are documented at length in
the root [`README.md`](../../README.md) "Lifecycle worker flags"
section.

`GET /auth/me` (see [`./auth.md`](./auth.md)) is a separate endpoint
that is **always** available — no email-verification gate, no
lifecycle flag — and exists specifically for SPA auth-store
hydration. Use it when you only need `{ id, email, email_verified }`.

## Endpoints

### GET /api/user/me

- **Auth:** access JWT + email-verification middleware +
  `ACCOUNT_LIFECYCLE_API_ENABLED`.
- **Response:** `200 OK`,
  ```json
  {
    "id":                     "<uuid>",
    "email":                  "alice@example.com",
    "email_verified":         true,
    "subscription_plan":      "creator",
    "subscription_status":    "active",
    "deletion_requested_at":  null,
    "data_export_url":        null,
    "data_export_expires_at": null,
    "created_at":             "2026-01-15T11:22:33Z"
  }
  ```
  `data_export_url` and `deletion_requested_at` are populated when a
  GDPR export is ready or a deletion has been scheduled, so the SPA
  can surface "Your export is ready" / "Account deletion in N hours"
  banners without extra round trips.
- **Errors:** `401 unauthorized`, `404 not found`.

### DELETE /api/user/account

- **Auth:** access JWT + email-verification.
- **Request:**
  ```json
  { "email_confirmation": "alice@example.com" }
  ```
  The confirmation must match the caller's email address
  (case-insensitive trim). This guards against an attacker who
  briefly steals an access token.
- **Effect:**
  1. Sets `users.deletion_requested_at = NOW()`.
  2. If `users.stripe_subscription_id` is set, calls
     `billing.CancelSubscription`. Stripe failures are logged but
     non-fatal — the user's intent is paramount.
  3. Inserts an `account_deletion_jobs` row (idempotent).
  4. Sends the `deletion_scheduled` transactional email.
- **Response:** `202 Accepted`,
  ```json
  {
    "job_id":           "<uuid>",
    "reversible_until": "2026-06-04T03:14:22Z",
    "status":           "scheduled"
  }
  ```
  `reversible_until` is `NOW() + DELETION_REVERSIBLE_HOURS` (default
  30 days). The deletion worker refuses to start before this
  timestamp.
- **Errors:**
  - `400 invalid request body` / `email_confirmation does not match
    account email`.
  - `401 unauthorized`.
  - `404 user not found`.
  - `500 failed to mark deletion` / `failed to schedule deletion`.

A second `DELETE` from the same user during the reversible window
returns `202` with the existing reversible timestamp — idempotent.

### POST /api/user/account/cancel-deletion

- **Auth:** access JWT + email-verification.
- **Effect:** Reverses a pending deletion **only while
  `account_deletion_jobs.status = 'pending'`**. Once the worker has
  claimed the job (`status = 'processing'`) cancellation is unsafe —
  OAuth tokens may have already been revoked and comments deleted.
  In that case use the support runbook at
  [`../ops/runbooks/account-deletion-and-gdpr.md`](../ops/runbooks/account-deletion-and-gdpr.md)
  (which describes the `cancel_deletion` CLI for the same gating).
- **Response:** `200 OK` (no body).
- **Errors:** `409 no cancellable deletion found`,
  `500 failed to cancel deletion`.

### POST /api/user/data-export

- **Auth:** access JWT + email-verification.
- **Effect:** Enqueues a GDPR data export job. The export worker
  produces a ZIP containing every row touching the user's account;
  see the data-lifecycle spec for the manifest.
- **Response:** `202 Accepted`,
  `{ "job_id": "<uuid>", "status": "pending" }`.
- **Errors:** `500 failed to enqueue export`.

### GET /api/user/data-export/{id}/status

- **Auth:** access JWT + email-verification.
- **Response:** `200 OK`,
  ```json
  {
    "job_id":       "<uuid>",
    "status":       "completed",
    "download_url": "/api/exports/<uuid>/download?sig=...&exp=...",
    "expires_at":   "2026-05-12T03:14:22Z"
  }
  ```
  `download_url` and `expires_at` are only present when `status ==
  "completed"`. Cross-user job ids return `404 not found` — never
  `403` — to avoid leaking job-id existence.
- **Errors:** `404 not found`, `401 unauthorized`.

### GET /api/exports/{id}/download

- **Auth:** none — the signed URL **is** the auth. The handler
  verifies `sig` against `EXPORT_SIGNED_URL_SECRET`, checks `exp`
  has not passed, and confirms the on-disk file still exists.
- **Response:** `200 OK`, `Content-Type: application/zip`,
  `Content-Disposition: attachment;
  filename="noumena-export-<id>.zip"`.
- **Errors:**
  - `403 missing signature` / `invalid expiry` /
    `invalid or expired signature`.
  - `404 not found` / `file not found`.

## Example

```sh
# Request account deletion
curl -i -X DELETE https://api.noumena/api/user/account \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"email_confirmation":"alice@example.com"}'

# Reverse it during the grace window
curl -i -X POST https://api.noumena/api/user/account/cancel-deletion \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"

# Or kick off a GDPR export
curl -i -X POST https://api.noumena/api/user/data-export \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
```

## Related

- [`./auth.md`](./auth.md) — `GET /auth/me`, register/login/refresh.
- [`./billing.md`](./billing.md) — Stripe subscription cancellation.
- [`../ops/runbooks/account-deletion-and-gdpr.md`](../ops/runbooks/account-deletion-and-gdpr.md)
  — support workflows + the `cancel_deletion` CLI.
- Configuration variables in [`.env.example`](../../.env.example):
  `ACCOUNT_LIFECYCLE_API_ENABLED`, `LIFECYCLE_WORKERS_ENABLED`,
  `DELETION_REVERSIBLE_HOURS`, `EXPORT_SIGNED_URL_SECRET`.
