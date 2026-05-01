# Spec: Comment Ingestion Pipeline

## Overview
Build a fault-tolerant, rate-limited ingestion pipeline that polls YouTube channels every 30 minutes, fetches new comments, deduplicates them, and stores them in the shared `comments` table. Only one ingestion job runs per platform channel at a time via distributed locking.

## Requirements
- ING-01: Poll every 30 minutes per platform channel (shared, not per user)
- ING-02: Ingest video-level comments and top-level replies with `parent_comment_id`
- ING-03: Deduplicate across runs using `platform_comment_id`
- ING-04: Exponential backoff retry (max 5 retries: 1s, 2s, 4s, 8s, 16s), then dead-letter + ops alert
- ING-05: Respect platform API rate limits via header reading and queue throttling
- ING-06: Distributed lock per platform channel using `pg_try_advisory_lock`
- ING-07: Use primary ingestion token (lowest `token_priority = 0` among active trackers)
- ING-08: Backfill on first-time connection (up to configured days, default 90)

## Database Changes

Add to `platform_channels`:
- `ingestion_lock_id BIGINT` — derived from `platform_channel_id` hash for advisory locks
- `ingestion_error_count INT NOT NULL DEFAULT 0`
- `ingestion_last_error_at TIMESTAMPTZ`
- `ingestion_backoff_until TIMESTAMPTZ`

Create job queue table (if not using River yet):
```sql
CREATE TABLE ingestion_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform_channel_id UUID NOT NULL REFERENCES platform_channels(id) ON DELETE CASCADE,
    job_type TEXT NOT NULL, -- 'poll', 'backfill'
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'running', 'completed', 'failed', 'dead_lettered'
    retry_count INT NOT NULL DEFAULT 0,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_ingestion_jobs_pending ON ingestion_jobs(status, scheduled_at) WHERE status = 'pending';
```

## Architecture

### Ingestion Worker (`internal/ingestion/worker.go`)

A background goroutine that runs continuously:

```
Every 30 seconds:
  1. Query for platform_channels that need polling:
     - last_synced_at IS NULL (new, needs backfill) OR
     - last_synced_at < NOW() - interval '30 minutes'
     - ingestion_backoff_until IS NULL OR ingestion_backoff_until < NOW()
     - Has at least one active tracker
  2. For each eligible channel:
     a. Try to acquire advisory lock: pg_try_advisory_lock(ingestion_lock_id)
     b. If lock fails: skip (another worker is processing)
     c. Get primary ingestion token (decrypt it)
     d. Call YouTube API to fetch comments
     e. Upsert comments into DB (skip duplicates)
     f. Update last_synced_at, release advisory lock
```

### Advisory Lock ID
- Compute from hash of `platform_channel_id` (UUID string) cast to BIGINT:
  `SELECT ('x' || substr(md5(id::text), 1, 16))::bit(64)::bigint`
- This ensures deterministic lock ID per channel across all workers

### YouTube Data API v3 Integration

#### Fetching Videos
- `GET https://www.googleapis.com/youtube/v3/search?part=id&channelId={channelID}&type=video&publishedAfter={since}&maxResults=50&order=date`
- Paginate via `nextPageToken`

#### Fetching Comments
- For each video: `GET https://www.googleapis.com/youtube/v3/commentThreads?part=snippet,replies&videoId={videoID}&maxResults=100&order=time`
- Extract top-level comments and replies
- Store fields:
  - `platform_comment_id` = `comment.id`
  - `parent_comment_id` = `snippet.parentId` (null for top-level)
  - `author_handle` = `snippet.topLevelComment.snippet.authorDisplayName`
  - `body` = `snippet.topLevelComment.snippet.textDisplay`
  - `posted_at` = `snippet.topLevelComment.snippet.publishedAt`

### Rate Limit Handling
- Read `X-Quota-User-Remaining` and retry-after headers
- If rate limited: set `ingestion_backoff_until = NOW() + retry_after_duration`
- Global rate limiter: ensure no more than N concurrent API calls across all channels

### Deduplication
- Unique constraint on `(platform_channel_id, platform_comment_id)` handles race conditions
- On conflict, ignore insert

### Fault Tolerance & Retry
```
Retry schedule for a failed job:
  Attempt 1: immediate
  Attempt 2: +1s
  Attempt 3: +2s
  Attempt 4: +4s
  Attempt 5: +8s
  Attempt 6: +16s
  Attempt 7: dead-letter
```
- Increment `ingestion_error_count` and set `ingestion_last_error_at`
- On dead-letter: set status, log error, send alert (email to ops or log to monitoring)
- Reset `ingestion_error_count` to 0 on successful completion

### Backfill Job
- Triggered when a new `platform_channels` record is created
- Fetches comments from `NOW() - backfill_days` to present
- Uses same deduplication and rate limiting
- Can take hours for large channels; process in batches and checkpoint via `last_synced_at`
- Do NOT block user onboarding; run asynchronously

## API Endpoints

No direct user-facing API endpoints for ingestion (it's all background work), but:

#### GET /api/channels/{id}/sync-status
- Returns: `last_synced_at`, `next_sync_at`, `backfill_progress_percent` (if backfilling), `recent_error` (if any)

## Backend Implementation

### New Files
- `internal/ingestion/scheduler.go` — finds channels needing sync, dispatches to workers
- `internal/ingestion/youtube_client.go` — YouTube API wrapper with rate limiting
- `internal/ingestion/lock.go` — advisory lock helpers

### Update Files
- `internal/ingestion/worker.go` — refactor to use scheduler + lock mechanism
- `internal/db/db.go` — add DBQuery methods for ingestion state

### DB Methods to Add
```go
GetChannelsNeedingSync(ctx context.Context, intervalMinutes int) ([]*PlatformChannel, error)
UpdateSyncTime(ctx context.Context, id string) error // already exists, extend
InsertComment(ctx context.Context, c *Comment) error // already exists, ensure upsert
UpdateIngestionError(ctx context.Context, platformChannelID string, errMsg string, retryAfter time.Duration) error
ResetIngestionError(ctx context.Context, platformChannelID string) error
GetPrimaryToken(ctx context.Context, platformChannelID string) (*UserChannelTracking, error)
```

## Testing Checklist
- [ ] Ingestion job runs every 30 minutes per channel
- [ ] Duplicate comments are not inserted (unique constraint)
- [ ] Advisory lock prevents concurrent jobs on same channel
- [ ] Exponential backoff retries work and eventually dead-letter
- [ ] Rate limit headers are respected
- [ ] Backfill fetches historical comments up to configured days
- [ ] Primary token is used; fallback to next token on failure
- [ ] If no active trackers, ingestion stops for that channel

## Files to Create/Modify
- `internal/ingestion/worker.go` — refactor existing
- `internal/ingestion/scheduler.go` — new
- `internal/ingestion/youtube_client.go` — new (merge with existing youtube.go)
- `internal/ingestion/lock.go` — new
- `internal/db/channels.go` — add ingestion state methods
- `internal/db/comments.go` — ensure upsert logic
