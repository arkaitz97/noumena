# Insights API Reference

> Source of truth: [`specs/05-insights-api.md`](../../specs/05-insights-api.md).
> Handlers:
> [`internal/api/insights_handlers.go`](../../internal/api/insights_handlers.go)
> and [`internal/api/comment_handlers.go`](../../internal/api/comment_handlers.go).
> Routes: [`cmd/api/main.go`](../../cmd/api/main.go).

The insights surface is gated by `INSIGHTS_API_ENABLED`. When the flag
is `false`, every route below returns `503 insights_api_disabled`.
Access control is enforced at the SQL layer — every query joins
through `user_channel_tracking` so a request can never see comments
belonging to a channel the caller does not track.

Errors are returned as `{ "error": "<code>" }` (compact, semantic
codes — no human-friendly messages, since the SPA owns user-visible
copy).

## Comment feeds

### GET /api/comments

Legacy paged feed. Continues to work for compatibility, but the
filtered/sorted feed below is preferred.

- **Auth:** access JWT + email-verification middleware.
- **Query:** `limit` (default 50), `offset` (default 0).
- **Response:** `200 OK`, JSON array of comments.

### GET /api/comments/feed

Filtered feed used by the dashboard.

- **Auth:** access JWT + email-verification + `INSIGHTS_API_ENABLED`.
- **Query parameters** (all optional):

  | Param | Format | Notes |
  |---|---|---|
  | `channels` | CSV of channel UUIDs | Restricts to those channels (still subject to access control). |
  | `categories` | CSV of category labels | High-signal default categories — see the spec. |
  | `insight_type` | insight type label | Filters to comments containing at least one insight of the specified type. |
  | `date_from` / `date_to` | `YYYY-MM-DD` or RFC3339 | |
  | `search` | free text | Full-text search on the comment body. |
  | `include_toxic` | `true` / `1` | Two-layer toxic comment opt-in. |
  | `limit` | int, clamped to `FEED_MAX_LIMIT` | |
  | `offset` | int | |
  | `sort` | one of the `db.AllowedSorts` values | Default `newest`. |

- **Response:** `200 OK`,
  ```json
  {
    "comments": [ ... ],
    "total":    12345,
    "has_more": true
  }
  ```
  `total` is `null` when the count query times out
  (`COUNT_QUERY_TIMEOUT`); in that case `has_more` is derived from
  whether the page returned exactly `limit` rows.
- **Errors:** `400` for any invalid query param (`invalid sort`,
  `invalid insight_type`, etc.), `408 feed_query_timeout` when the
  feed query exceeds `FEED_QUERY_TIMEOUT`, `500 feed_query_failed`.

### GET /api/comments/saved

Same response shape as `/feed` but returns only the caller's starred
comments. The `sort` query param is ignored — `/saved` enforces its
own ordering.

### POST /api/comments/{id}/star

- **URL param:** `{id}` is the comment UUID.
- **Effect:** Idempotent insert into `user_comment_stars`. Access
  control: the comment must belong to a channel the caller tracks; a
  foreign comment id returns `404 not_found`.
- **Response:** `200 OK`,
  `{ "comment_id": string, "starred": "true" }`.
- **Errors:** `400 invalid_comment_id`, `404 not_found`,
  `500 star_failed`.

### DELETE /api/comments/{id}/star

- **Effect:** Removes the caller's star.
- **Response:** `204 No Content`.
- **Errors:** `400 invalid_comment_id`, `404 not_found`,
  `500 unstar_failed`.

## Insights aggregates

All `/insights/*` endpoints accept an optional `channel_id` query
parameter. Omit it (or pass `all`) to aggregate across every channel
the caller tracks. A foreign UUID returns `403 not_tracked`.

### GET /api/insights/actionable-insights

- **Query:** `window` ∈ `{7d, 30d, 90d}` (default `7d`), optional
  `channel_id`.
- **Response:** `200 OK`,
  ```json
  {
    "channel_id": "all",
    "window": "7d",
    "total_insights": 456,
    "distribution": [
      { "type": "video_idea", "count": 120 },
      { "type": "editing_tip", "count": 85 }
    ],
    "top_insights": [
      { "type": "video_idea", "description": "Make a color grading tutorial", "count": 12 },
      { "type": "editing_tip", "description": "Use smoother transitions", "count": 8 }
    ]
  }
  ```
- **Errors:** `400 not_tracked` (malformed UUID), `403 not_tracked`,
  `500 aggregate_failed`.

### GET /api/insights/toxicity-shield

- **Response:** Same shape, with `windows` keyed by `7d` and `30d`,
  each value a `ToxicityBucket` (counts of total / shielded
  comments).
- **Errors:** Same set as `/sentiment`.

### GET /api/insights/trends

- **Query:** `days` ∈ `{7, 30, 90}` (default `30`), optional
  `channel_id`.
- **Response:**
  ```json
  {
    "channel_id": "all",
    "days": 30,
    "top_topics":     [ { "topic": "...", "count": 12 }, ... ],
    "top_categories": [ { "category": "...", "count": 9 }, ... ]
  }
  ```
- **Errors:** `400 invalid_days`, `403 not_tracked`,
  `500 trends_failed`.

### GET /api/insights/digest/latest

Returns the most recent weekly digest for the caller (read-only;
production is owned by the weekly-digest worker).

- **Response:** `200 OK`,
  ```json
  {
    "id": "...",
    "week_start": "2026-04-27",
    "week_end":   "2026-05-03",
    "top_ideas":      [ ... ],
    "top_questions":  [ ... ],
    "top_actionable_insights": [
      { "type": "video_idea", "description": "Make a tutorial", "count": 5 }
    ],
    "health_score":   87
  }
  ```
  The non-base keys (`top_ideas`, `top_questions`, etc.) are passed
  through verbatim from the `weekly_digests.content` JSON TEXT column —
  the producer (weekly-digest spec) owns their shape.
- **Errors:** `404 no_digest_yet`, `500 digest_failed`.

## Comment exports

See the dedicated reference at [`./comments.md`](./comments.md) for
the export lifecycle, sync vs. async semantics, and the CSV column
schema.

## Example

```sh
# All comments from a single channel, last 7 days, newest first
curl -s -G https://api.noumena/api/comments/feed \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  --data-urlencode 'channels=11111111-2222-3333-4444-555555555555' \
  --data-urlencode 'date_from=2026-04-28' \
  --data-urlencode 'sort=newest' \
  --data-urlencode 'include_toxic=true' \
  --data-urlencode 'limit=100' | jq
```

## Related

- Configuration: see [`.env.example`](../../.env.example) for
  `INSIGHTS_API_ENABLED`, `FEED_DEFAULT_LIMIT`, `FEED_MAX_LIMIT`,
  `FEED_QUERY_TIMEOUT`, `COUNT_QUERY_TIMEOUT`, `EXPORT_DIR`,
  `EXPORT_TTL_HOURS`, `EXPORT_SYNC_THRESHOLD`,
  `EXPORT_WORKER_ENABLED`.
- Operations: [`../ops/runbooks/export-disk-monitoring.md`](../ops/runbooks/export-disk-monitoring.md).
