# Proposal: Optimize YouTube API Quota Usage

## Why

YouTube ingestion spends most quota on `search.list`, which costs 100 units per request. Comment ingestion also issues `comments.list` for every top-level comment, including threads with no additional replies. Fixed polling, broad pagination, and missing request budgets amplify waste and can exhaust the 10,000-unit daily project quota.

## What Changes

- Replace video discovery through `search.list` with uploads-playlist `playlistItems.list`.
- Persist and reuse each channel's uploads playlist ID.
- Fetch deep replies only when `totalReplyCount` exceeds replies embedded in `commentThreads.list`.
- Add incremental per-video watermarks and resumable page checkpoints.
- Add adaptive polling and per-job/daily quota budgets.
- Add YouTube method/unit telemetry and quota-aware backoff.
- Add partial-response `fields` projections and metadata caching.
- Preserve existing comment deduplication, thread structure, and token fallback behavior.

## Capabilities

### Modified Capabilities

- `ingestion`: quota-efficient video discovery, incremental comments, checkpoints, and adaptive scheduling.
- `observability-metrics`: YouTube request, quota, and suppression metrics.

## Impact

- `internal/ingestion/youtube_client.go`
- `internal/ingestion/worker.go`
- `internal/ingestion/scheduler.go`
- ingestion database schema and migrations
- ingestion tests and operational documentation

Expected primary reduction: replace 100-unit `search.list` calls with 1-unit `playlistItems.list` calls. Secondary reduction: eliminate unnecessary `comments.list` calls.
