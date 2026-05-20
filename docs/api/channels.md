# Channels API Reference

> Source of truth: [`specs/02-channels.md`](../../specs/02-channels.md).
> Handler: [`internal/api/channel_handlers.go`](../../internal/api/channel_handlers.go).
> Routes: [`cmd/api/main.go`](../../cmd/api/main.go) under
> `/api/channels/*`.

All channels endpoints sit behind the access-token JWT middleware and
the email-verification middleware. Errors are returned as
`{ "code": string, "message": string }`.

## Backfill window semantics

`backfill_days` may be `30`, `60`, `90`, or `180`. It only governs the
**first** time a `platform_channel` is added to the system; once a
backfill has run, **changing the window does not trigger a
re-backfill**. See the user-facing explanation at
[`../help/backfill-window.md`](../help/backfill-window.md).

## Endpoints

### GET /api/channels

- **Auth:** access JWT.
- **Response:** `200 OK`, JSON array of the caller's tracked channels.
  Each entry includes `id` (platform channel id), `display_name`,
  `tracking_status`, `last_synced_at`, and metadata derived from the
  shared `platform_channels` row.
- **Errors:** `500 list_failed`.

### GET /api/channels/connect/youtube

- **Auth:** access JWT.
- **Response:** `200 OK`, `{ "auth_url": string, "state": string }`.
  The SPA navigates the user (or opens a popup) to `auth_url`. The
  `state` is also persisted server-side hashed in `oauth_states`
  scoped to the calling user with `OAUTH_STATE_TTL` (default 10
  minutes).
- **Errors:** `500 state_generation_failed` or `state_persist_failed`.

### POST /api/channels/exchange/youtube

- **Auth:** access JWT.
- **Request:**
  ```json
  {
    "code": "<google authorization code>",
    "state": "<value returned by /connect/youtube>",
    "backfill_days": 90
  }
  ```
  `backfill_days` is optional; defaults to `90`.
- **Response:** `201 Created`,
  ```json
  {
    "id": "<user_channel_tracking row id>",
    "platform_channel_id": "<shared platform_channels id>",
    "display_name": "Channel Name",
    "tracking_status": "active",
    "token_priority": 0,
    "backfill_days": 90,
    "token_expires_at": "2026-05-12T10:00:00Z"
  }
  ```
- **Errors:**
  - `400 invalid_body` / `invalid_code` / `invalid_state` /
    `invalid_backfill_days` — validation.
  - `400 no_refresh_token` — Google did not return a refresh token.
    The caller should ask the user to revoke the app at
    `myaccount.google.com/permissions` and retry.
  - `400 exchange_failed` — opaque OAuth exchange failure.
  - `402 plan_limit_reached` — the user is on a plan whose channel
    limit has been reached. See
    [`./billing.md`](./billing.md) for plan limits.
  - `409 already_tracked` — the caller already tracks this channel.
  - `500 *` — encryption, upsert, tracking insert, etc.

A new `platform_channels` row triggers a backfill enqueue (see
[`../ops/runbooks/ingestion-deployment.md`](../ops/runbooks/ingestion-deployment.md)).

### PATCH /api/channels/{id}/pause

- **Auth:** access JWT.
- **URL params:** `{id}` is the `platform_channel_id`.
- **Effect:** Sets the caller's `user_channel_tracking.tracking_status`
  to `paused`. If the caller was the primary token holder
  (`token_priority = 0`), the next-priority active tracker is promoted.
- **Response:** `200 OK`, `{ "message": "channel paused" }`.
- **Errors:** `404 not_found`, `500 pause_failed`.

### POST /api/channels/{id}/resume

- **Auth:** access JWT.
- **Effect:** Sets `tracking_status = 'active'`.
- **Response:** `200 OK`, `{ "message": "channel resumed" }`.
- **Errors:** `404 not_found`, `500 resume_failed`.

### DELETE /api/channels/{id}

- **Auth:** access JWT.
- **Effect:** Permanently removes the caller's tracking record. If the
  caller was the primary token holder, the next-priority tracker is
  promoted. When the active tracker count for the shared channel
  reaches zero, `platform_channels.deletion_scheduled_at` is set to
  `NOW() + CHANNEL_DELETION_GRACE_HOURS` (default 24h) and a purge
  job is enqueued. See the support runbook at
  [`../ops/runbooks/channel-grace-window.md`](../ops/runbooks/channel-grace-window.md).
- **Response:** `200 OK`, `{ "message": "channel removed" }`.
- **Errors:** `404 not_found`, `500 delete_failed`.

### PATCH /api/channels/{id}/backfill-window

- **Auth:** access JWT.
- **Request:** `{ "days": 30 | 60 | 90 | 180 }`.
- **Response:** `200 OK`,
  ```json
  {
    "message": "backfill window updated",
    "days": 90,
    "note": "this setting affects future first-time channel additions only; it does not re-backfill this channel"
  }
  ```
- **Errors:** `400 invalid_body` / `invalid_backfill_days`,
  `404 not_found`, `500 update_failed`.

### GET /api/channels/{id}/status

- **Auth:** access JWT.
- **Response:** `200 OK`,
  ```json
  {
    "tracking_status": "active",
    "token_expires_at": "2026-05-12T10:00:00Z",
    "last_synced_at": "2026-05-05T03:14:22Z",
    "is_primary_token_holder": true,
    "comment_count": 12345,
    "backfill_days": 90
  }
  ```
- **Errors:** `404 not_found`, `500 lookup_failed`.

### GET /api/channels/{id}/sync-status

- **Auth:** access JWT.
- **Response:** `200 OK`,
  ```json
  {
    "last_synced_at": "2026-05-05T03:14:22Z",
    "next_sync_at":   "2026-05-05T04:14:22Z",
    "backfill_progress_percent": 73,
    "recent_error": {
      "at": "2026-05-05T03:00:00Z",
      "message": "rate_limited"
    }
  }
  ```
  `backfill_progress_percent` and `recent_error` are nullable.
- **Errors:** `404 not_found`, `500 lookup_failed`,
  `501 not_implemented` (sync-status reader not configured).

## Example

```sh
# Start the OAuth flow
curl -s https://api.noumena/api/channels/connect/youtube \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" | jq

# After the popup/redirect returns ?code=…&state=…
curl -i -X POST https://api.noumena/api/channels/exchange/youtube \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"code":"4/0A...","state":"abc","backfill_days":90}'
```

## Related

- Operations: [`../ops/runbooks/youtube-oauth-setup.md`](../ops/runbooks/youtube-oauth-setup.md),
  [`../ops/runbooks/channel-grace-window.md`](../ops/runbooks/channel-grace-window.md).
- Configuration: see [`.env.example`](../../.env.example) for
  `YOUTUBE_CLIENT_ID`, `YOUTUBE_CLIENT_SECRET`, `YOUTUBE_REDIRECT_URI`,
  `OAUTH_STATE_TTL`, `CHANNEL_DELETION_GRACE_HOURS`.
