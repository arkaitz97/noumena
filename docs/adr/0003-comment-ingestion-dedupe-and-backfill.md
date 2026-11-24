# 0003. Deduplicate comments on the platform key and advance checkpoints in the same transaction

## Status

Accepted. 2026-11-24.

## Context and Problem Statement

YouTube ingestion is paginated, rate limited, and can be interrupted by a
crash, a quota ceiling, or a token failure. A retry must resume without
duplicating comments and without skipping comments. A backfill can run for
days and is expected to be interrupted.

Comments can also change after insertion: a video title is corrected, or a
reply arrives after its parent. And the same comment can be seen again during
an incremental poll.

## Decision Drivers

- A replayed page must not create duplicate rows or double-count quota.
- A crash must lose at most the batch in flight, not the whole walk.
- The stored history must be stable: changing a backfill window must not
  silently re-fetch and rewrite old data.
- The LLM processor must see each comment once, with the latest available
  metadata.

## Considered Options

1. **Append-only inserts with duplicate rows removed at read time.**
2. **Insert-ignore on the platform key, checkpoint saved separately.**
3. **Upsert on `(platform_channel_id, platform_comment_id)` with the
   checkpoint advanced in the same transaction.**

## Decision Outcome

Chosen option 3.

The `comments` table has `UNIQUE(platform_channel_id, platform_comment_id)`
and a named unique index `comments_platform_dedup`
(`internal/db/schema.sql`). Inserts use
`ON CONFLICT (platform_channel_id, platform_comment_id) DO UPDATE` to refresh
`video_id` and `video_title` without replacing classification fields
(`internal/db/comments.go`).

`CommitCommentsAndCheckpoint` inserts a batch and upserts the matching
`channel_sync_checkpoints` row in one transaction. A failure rolls both back.
The worker checkpoints at two levels: the channel phase (`page_token`,
`boundary`, status) and the per-video watermark (`video_sync_states`).

Backfill windows are 30, 60, 90, or 180 days (`backfillDayOptions`,
`internal/db/channel_mgmt.go`). The window applies to the next backfill only.
A completed backfill is not replayed because the user widened the window; this
is stated in `docs/help/backfill-window.md`.

Retries are bounded at 5 attempts, then dead-letter. Per-job ceilings (100
videos, 25 thread pages, 100 reply fetches) produce a clean stop and a
rescheduled job, not an error.

### Consequences

#### Good

- Replaying a page is safe. The row count is stable and only metadata changes.
- The checkpoint cannot advance past data that was not committed.
- Quota metering counts processed comments, not fetch attempts, because the
  same row is not re-inserted.

#### Bad

- Upsert cost is higher than insert-ignore. On a replay of a large page, every
  row runs the conflict path.
- `INSERT ... ON CONFLICT` still consumes an autoincrement rowid on conflict in
  SQLite, so the comments table can have gaps. This is harmless but visible.
- A backfill window change is a product surprise. Users expect "widen to 180
  days" to add history; it only affects the next unfinished backfill.

#### Neutral

- Classification fields are never overwritten by ingestion. A comment edited
  on YouTube keeps its original classification until reprocessed.
- Dead-lettered jobs are not retried automatically. An operator requeues them.

## Confirmation

- Schema: `UNIQUE(platform_channel_id, platform_comment_id)` and
  `comments_platform_dedup` in `internal/db/schema.sql`.
- Transaction: `CommitCommentsAndCheckpoint` in `internal/db/comments.go`.
- `go test ./internal/db/ -run 'Test_ApplyMigrations_Idempotent'`
- `go test ./internal/ingestion/`
- `Gap`: no focused test replays the same batch and asserts zero new rows. Add
  one when this code changes.

## More Information

- `internal/ingestion/worker.go`
- `internal/ingestion/scheduler.go`
- `internal/db/ingestion.go`
- `docs/help/backfill-window.md`
- `docs/ops/runbooks/youtube-quota-budget.md`
