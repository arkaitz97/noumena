# YouTube API Quota Budget

Scope: the `optimize-youtube-api-quota` ingestion behavior — where units go,
what budgets stop them, what state is stored, and how to reset it.

## Quota costs (YouTube Data API v3, units per request)

| Method | Units | Used for | Status |
| --- | --- | --- | --- |
| `search.list` | **100** | video discovery | **fallback only** |
| `playlistItems.list` | 1 | uploads-playlist discovery | primary |
| `channels.list` | 1 | uploads-playlist id resolve (once per channel, cached) | primary |
| `commentThreads.list` | 1 | comments + inline replies | primary |
| `comments.list` | 1 | deep replies, **only when `totalReplyCount` > embedded replies** | gated |

Costs are centralized in `internal/ingestion/youtube_client.go`
(`youtubeMethodUnits`). Add new methods there — never inline a cost.

## Budget policy

- **Daily project budget** (`YOUTUBE_DAILY_QUOTA_UNITS`, default `10000`):
  the client meters units per UTC day and stops issuing requests once spent
  (`QuotaExhaustedError`). A server-side `quotaExceeded` response arms the
  same hard stop until the next UTC day. Exhausted jobs are marked completed
  and the channel is deferred — **retry budget is never consumed**.
- **Per-job ceilings**: `INGESTION_MAX_REQUESTS_PER_JOB` (context budget,
  enforced in `doGET`), `INGESTION_MAX_VIDEOS_PER_JOB`,
  `INGESTION_MAX_THREAD_PAGES_PER_JOB`, `INGESTION_MAX_REPLY_FETCHES_PER_JOB`.
  Hitting one is a clean stop: durable checkpoints make the next tick resume.
- **Adaptive polling**: `INGESTION_POLL_INTERVAL_MINUTES` is the floor.
  Channels with new content halve their interval toward it; silent channels
  double it up to `INGESTION_POLL_INTERVAL_MAX_MULTIPLIER` (default 8) x the
  floor. Existing error backoff still wins when longer.
- **Partial responses**: every request sends a `fields` projection; nothing
  beyond the listed JSON paths is fetched.

## Discovery fallback behavior

1. Worker reads cached `platform_channels.yt_uploads_playlist_id`.
2. Empty → `channels.list(part=contentDetails)` resolves the uploads
   playlist once and caches it.
3. Resolution fails for non-auth/non-limit reasons → **bounded** fallback to
   one `search.list` walk for the job (401/429/ceiling errors propagate,
   never masked). Each fallback is logged and counted on
   `noumena_youtube_discovery_total{via="search_fallback"}` — sustained
   fallback means quota is burning at 100 units/request; investigate
   channel resolution instead of tolerating it.

Telemetry: `noumena_youtube_requests_total{method,status}`,
`noumena_youtube_quota_units_total{method}`,
`noumena_youtube_requests_suppressed_total{reason}` (`daily_budget`,
`job_ceiling`). See [`observability.md`](../../observability.md).

## Stored sync state (SQLite)

`platform_channels`:

| Column | Meaning |
| --- | --- |
| `yt_uploads_playlist_id` | cached uploads playlist id (`UU…`) for 1-unit discovery |
| `poll_interval_minutes` | adaptive interval override; `NULL` = configured floor |

`video_sync_state` (PK `platform_channel_id, video_id`):

| Column | Meaning |
| --- | --- |
| `comment_watermark_at` | newest comment `posted_at` durably persisted for the video |
| `page_checkpoint` | `commentThreads` page token to resume an interrupted walk; `NULL` when complete |
| `status` | `active`, or `skipped` for videos whose comments are disabled/deleted |

Replays are safe: the unique constraint on
`comments(platform_channel_id, platform_comment_id)` deduplicates.

## Operational reset procedure

Clear state to force a full re-walk (comments are deduped, no duplicates
land; costs one resumed walk per video):

```sql
-- Drop all per-video checkpoints and watermarks:
DELETE FROM video_sync_state;

-- Re-resolve a channel's uploads playlist (e.g. channel merged/changed):
UPDATE platform_channels SET yt_uploads_playlist_id = NULL
 WHERE platform_channel_id = 'UC…';

-- Reset adaptive polling to the configured floor:
UPDATE platform_channels SET poll_interval_minutes = NULL;

-- Un-skip a video whose comments were re-enabled:
UPDATE video_sync_state SET status = 'active'
 WHERE video_id = '…' AND status = 'skipped';
```

After any reset, `RecoverStaleJobs` + the scheduler pick the work up on the
next tick; no restart needed. Verify with
`noumena_youtube_quota_units_total` — daily spend should stay under
`YOUTUBE_DAILY_QUOTA_UNITS` minus headroom for interactive endpoints.
