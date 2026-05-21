# Comments Export API Reference

> Source of truth: [`specs/05-insights-api.md`](../../specs/05-insights-api.md)
> §10 (CSV/JSON encoder) and the `comment-ingestion` capability.
> Handler:
> [`internal/api/insights_handlers.go`](../../internal/api/insights_handlers.go)
> (`CreateExport`, `ExportStatus`, `ExportDownload`).
> CSV header is owned by
> [`internal/export/encoder.go`](../../internal/export/encoder.go).

The export endpoints let a user download every comment that matches a
filter set in CSV or JSON. The pipeline is gated by
`INSIGHTS_API_ENABLED`; the async tail also requires
`EXPORT_WORKER_ENABLED`.

## Sync vs. async threshold

`POST /api/comments/export` runs a `COUNT(*)` against the filtered
feed before deciding which path to take.

- **Synchronous** — returns `200 OK` with the file ready to download.
  Used when both
  - the count query returned in under `COUNT_QUERY_TIMEOUT`, AND
  - the resulting count is **at most `EXPORT_SYNC_THRESHOLD`**
    (default **50,000** rows;
    [`internal/config/config.go`](../../internal/config/config.go)).
- **Asynchronous** — returns `202 Accepted` with `{ job_id, status:
  "queued" }`. Triggered when the count exceeds the threshold OR the
  count query itself times out (we conservatively go async rather
  than wait synchronously). The export worker picks up the job and
  the client polls `GET /api/comments/export/{id}/status` for
  completion.

In both modes the file is written to
`${EXPORT_DIR}/<user_id>/<export_id>.<csv|json>` with mode `0600` and
the parent directory at `0700`. Files older than
`EXPORT_TTL_HOURS` (default 24) are swept by the cleanup worker; the
DB row is marked `expired` and subsequent download attempts return
`410 Gone`.

## CSV column schema

The CSV header order is **stable** — downstream BI tools depend on
it. A change requires a major-version bump and a coordinated rollout.
The end-user friendly version of this reference lives at
[`../help/csv-export-schema.md`](../help/csv-export-schema.md).

| # | Column | Type | Notes |
|---|--------|------|-------|
| 1 | `id` | UUID | Comment primary key. |
| 2 | `channel_name` | string | Display name of the source channel. |
| 3 | `author_handle` | string \| empty | Empty when the platform did not return one. |
| 4 | `body` | string | RFC4180-quoted; commas, quotes, and newlines preserved. |
| 5 | `posted_at` | RFC3339 UTC | Timestamp from the platform. |
| 6 | `category` | string \| empty | Best-fit category from LLM processing. |
| 7 | `actionable_insights` | JSON array \| empty | JSON-encoded array of insight objects. Empty when LLM processing has not run yet or no insights extracted. |
| 8 | `topic` | string \| empty | Best-fit topic. |
| 9 | `confidence` | float in `[0, 1]` \| empty | LLM confidence for `category`/`topic`. |
| 10 | `processed_at` | RFC3339 UTC \| empty | When LLM processing completed. |

JSON exports use the same field names with native JSON types
(numbers, ISO-8601 strings, `null` instead of empty strings).

## Endpoints

### POST /api/comments/export

- **Auth:** access JWT + email-verification + `INSIGHTS_API_ENABLED`.
- **Request body:** mirrors the `/feed` query string but as JSON:

  ```json
  {
    "format": "csv",
    "channels":     ["<uuid>", "..."],
    "categories":   ["..."],
    "insight_type": "video_idea",
    "date_from":    "2026-01-01",
    "date_to":      "2026-05-05",
    "search":       "shipping",
    "include_toxic": false,
    "sort":         "newest"
  }
  ```
  `format` MUST be `"csv"` or `"json"`.

- **Sync response:** `200 OK`,
  ```json
  {
    "export_id":    "<uuid>",
    "download_url": "/api/comments/export/<uuid>/download",
    "expires_at":   "2026-05-06T03:14:22Z",
    "row_count":    12345
  }
  ```
- **Async response:** `202 Accepted`,
  `{ "job_id": "<uuid>", "status": "queued" }`. Poll
  `GET .../status` until `status == "completed"`, then GET the
  `download_url` it surfaces.
- **Errors:**
  - `400 invalid_body` / `invalid_format` / `invalid date_from` / `invalid date_to`.
  - `500 count_failed` / `enqueue_failed` / `mkdir_failed` /
    `open_failed` / `encode_failed` / `fsync_failed` /
    `mark_completed_failed`.

### GET /api/comments/export/{id}/status

- **Auth:** access JWT + `INSIGHTS_API_ENABLED`.
- **Response:** `200 OK`,
  ```json
  {
    "job_id": "<uuid>",
    "status": "completed",
    "format": "csv",
    "row_count": 12345,
    "created_at":   "2026-05-05T03:00:00Z",
    "started_at":   "2026-05-05T03:00:01Z",
    "completed_at": "2026-05-05T03:01:13Z",
    "expires_at":   "2026-05-06T03:01:13Z",
    "download_url": "/api/comments/export/<uuid>/download"
  }
  ```
  `status` is one of `queued`, `running`, `completed`, `failed`,
  `expired`. `download_url` is only present when `status ==
  "completed"`.
- **Errors:** `400 invalid_job_id`, `404 not_found`, `500 status_failed`.

### GET /api/comments/export/{id}/download

- **Auth:** access JWT + `INSIGHTS_API_ENABLED`.
- **Response:** `200 OK` with `Content-Disposition: attachment;
  filename="noumena-export-<id>.<csv|json>"`. `Content-Type` is
  `text/csv` or `application/json`.
- **Errors:**
  - `400 invalid_export_id`.
  - `404 not_found`.
  - `410 export_expired` — the row is `expired` OR the file is
    missing on disk (treated identically; usually the cleanup worker
    swept the TTL-expired file).
  - `500 download_failed`.

## Example

```sh
# Synchronous CSV export of one channel's last week
curl -s -X POST https://api.noumena/api/comments/export \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"format":"csv","date_from":"2026-04-28"}' | jq
```

## Related

- [`./insights.md`](./insights.md) for the matching feed/filter API.
- [`../ops/runbooks/export-disk-monitoring.md`](../ops/runbooks/export-disk-monitoring.md)
  for `EXPORT_DIR` capacity planning.
- Configuration variables in [`.env.example`](../../.env.example):
  `EXPORT_DIR`, `EXPORT_TTL_HOURS`, `EXPORT_SYNC_THRESHOLD`,
  `EXPORT_WORKER_ENABLED`, `INSIGHTS_API_ENABLED`.
