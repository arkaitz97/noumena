## Context

The ingestion worker discovers videos with `search.list`, fetches comment threads with `commentThreads.list`, then fetches replies independently with `comments.list`. `search.list` costs 100 quota units; `playlistItems.list`, `channels.list`, `commentThreads.list`, and `comments.list` cost 1 unit per request. Current code does not retain uploads-playlist metadata, reply counts, per-video cursors, or quota consumption.

## Goals / Non-Goals

**Goals:**

- Reduce steady-state discovery cost by using the channel uploads playlist.
- Avoid deep-reply requests when inline replies are complete.
- Make incremental sync resumable at video/page boundaries.
- Prevent backfills and active channels from consuming all daily quota.
- Expose enough telemetry to explain quota consumption and suppression.

**Non-Goals:**

- Changing stored comment semantics or deduplication rules.
- Parallelizing requests without quota admission control.
- Calling YouTube batch HTTP endpoints; batching does not reduce Data API units.
- Retrospectively recovering quota already consumed.

## Decisions

1. **Uploads playlist discovery.** Resolve `contentDetails.relatedPlaylists.uploads` once per channel using `channels.list`, persist it, and use `playlistItems.list(part=snippet,contentDetails,maxResults=50)` for discovery. Keep `search.list` only as a guarded compatibility fallback when playlist metadata is unavailable.
2. **Reply gating.** Preserve `totalReplyCount` on each thread. `commentThreads.list(part=snippet,replies)` remains primary. Call `comments.list` only when `totalReplyCount` is greater than the number of embedded replies, and paginate only until complete.
3. **Incremental state.** Store channel uploads-playlist ID, per-video comment watermark, and page checkpoint. Commit checkpoints with comment batches. A replay remains safe through existing unique constraints.
4. **Quota admission.** Centralize method costs in the YouTube client. Record request method, units, channel, status, and suppression reason. Enforce daily project budget and per-job ceilings before requests. Never aggressively retry quota-exhaustion errors.
5. **Polling.** Keep configured minimum interval, but increase intervals for channels with no new videos/comments and reduce them within configured limits for active channels. Honor existing channel backoff.
6. **Partial responses and caching.** Add `fields` projections for required JSON fields. Cache stable channel metadata and uploads-playlist IDs with invalidation on channel changes.
7. **Compatibility.** Preserve public worker behavior, token refresh, comments-disabled handling, job retry semantics, and thread parent/depth mapping.

## Failure Handling

- Missing playlist metadata: use bounded fallback resolution; do not repeatedly invoke `search.list`.
- Quota exhausted: stop current job, record suppression, defer channel, and avoid retry-budget consumption.
- Expired page checkpoint: restart from last durable video watermark; rely on deduplication.
- Deleted/private video: mark video state skipped and continue channel ingestion.
- API 429/5xx: apply existing bounded backoff; charge telemetry once per actual request.

## Verification

- Unit-test quota cost accounting and request construction.
- Test playlist pagination and fallback behavior.
- Test reply gating for zero, inline-complete, and paginated replies.
- Test checkpoint recovery after timeout/failure.
- Test daily/job budget suppression and scheduler adaptation.
- Integration-test unchanged comment persistence and deduplication.
