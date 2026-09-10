# Comment Ingestion Specification

## Purpose
Handles polling YouTube API for new comments, deduplicating across runs, and storing comments once in a shared database for all users tracking a channel.

## Requirements

### Requirement: Ingestion Scheduler
The system SHALL scan for channels needing sync and enqueue ingestion jobs at a configurable interval.

#### Scenario: Schedule Poll Job
- **GIVEN** a channel with last_synced_at within the poll interval (default 30 minutes)
- **WHEN** the scheduler ticks (default every 30 seconds)
- **THEN** the system SHALL NOT enqueue a new job for that channel

#### Scenario: Schedule Backfill Job
- **GIVEN** a channel with last_synced_at = NULL (never synced)
- **WHEN** the scheduler ticks
- **THEN** the system SHALL enqueue a job with type='backfill' for that channel

#### Scenario: Idempotent Enqueue
- **GIVEN** a channel with a pending or running ingestion job
- **WHEN** the scheduler ticks for that channel
- **THEN** the system SHALL skip enqueueing and increment the skipped counter

### Requirement: Ingestion Worker
The system SHALL process ingestion jobs by fetching comments from YouTube API and storing them in the database.

#### Scenario: Successful Comment Ingestion
- **GIVEN** a pending ingestion job for a YouTube channel
- **WHEN** a worker claims the job
- **THEN** the system SHALL claim the job atomically via `UPDATE … WHERE status='pending' … RETURNING` (SQLite write serialization provides per-channel mutual exclusion), fetch videos and comment threads via YouTube API, batch-insert comments (100 per batch), update last_synced_at, mark the job completed, and reset any prior ingestion error

#### Scenario: Per-Channel Mutual Exclusion
- **GIVEN** two workers running concurrently
- **WHEN** both attempt to process the same channel
- **THEN** only one worker SHALL process the channel at a time (SQLite serializes writes; claim atomicity + `ON CONFLICT DO NOTHING` dedupe guarantee it). A worker that loses the claim SHALL NOT consume retry budget

#### Scenario: Comment Deduplication
- **GIVEN** a comment already stored with the same (platform_channel_id, platform_comment_id)
- **WHEN** the ingestion worker attempts to insert it again
- **THEN** the system SHALL NOT create a duplicate record (unique constraint enforcement)

#### Scenario: Token Refresh Before API Call
- **GIVEN** an encrypted access/refresh token pair for a channel
- **WHEN** the worker prepares to call the YouTube API
- **THEN** the system SHALL decrypt the tokens, construct an oauth2.TokenSource, and force a refresh to obtain a valid access token

#### Scenario: Token Invalid - Promote Fallback
- **GIVEN** a token that returns 401 from YouTube API
- **WHEN** the worker detects the failure
- **THEN** the system SHALL attempt to promote the next ingestion token (by token_priority), release the job back to pending for re-processing with the new token, and not consume the retry budget

#### Scenario: No Token Available
- **GIVEN** a channel with no active ingestion token
- **WHEN** the worker attempts to obtain a token
- **THEN** the system SHALL initialize last_synced_at if NULL and mark the job completed without error

#### Scenario: Rate Limit Handling
- **GIVEN** a YouTube API rate limit response
- **WHEN** the worker encounters it
- **THEN** the system SHALL mark the channel in backoff until the retry-after time, mark the job completed (not a failure), and not consume retry budget

#### Scenario: Job Retry on Transient Error
- **GIVEN** a job that fails with a transient error and retry_count < 5
- **WHEN** the worker processes it
- **THEN** the system SHALL mark the job as failed with incremented retry count for later re-processing

#### Scenario: Dead Letter on Exhausted Retries
- **GIVEN** a job that fails with retry_count >= 5
- **WHEN** the worker processes it
- **THEN** the system SHALL mark the job as dead-lettered, park the channel in backoff for 1 hour, log the error, and emit an alert

#### Scenario: Job Deadline Exceeded
- **GIVEN** a job running longer than the deadline (default 5 minutes)
- **WHEN** the context times out
- **THEN** the system SHALL flush any pending comment batch, mark the job completed (not a failure), and allow re-enqueue on the next tick

#### Scenario: Non-YouTube Platform
- **GIVEN** an ingestion job for a non-YouTube platform (e.g., TikTok)
- **WHEN** the worker processes it
- **THEN** the system SHALL mark the job completed without making any API calls

### Requirement: Backfill Window
The system SHALL support configurable backfill windows for initial channel sync.

#### Scenario: Backfill with Configured Days
- **GIVEN** a backfill job with backfill_days = 90
- **WHEN** the worker computes the publishedAfter window
- **THEN** the system SHALL fetch comments published after (NOW - 90 days)

#### Scenario: Poll After Initial Backfill
- **GIVEN** a channel with last_synced_at already set
- **WHEN** a poll job runs
- **THEN** the system SHALL fetch comments published after last_synced_at (incremental sync)

### Requirement: Comment Thread Structure
The system SHALL preserve comment thread structure during ingestion using parent_comment_id.

#### Scenario: Ingest Top-Level Comment
- **WHEN** the ingestion worker processes a top-level YouTube comment (no parent)
- **THEN** the system SHALL store the comment with parent_comment_id = NULL

#### Scenario: Ingest Reply Comment
- **WHEN** the ingestion worker processes a reply comment via YouTube API threads.list
- **THEN** the system SHALL store the comment with parent_comment_id set to the parent's platform_comment_id

#### Scenario: Thread Depth Tracking
- **WHEN** the ingestion worker processes a comment
- **THEN** the system SHALL compute and store reply_depth (0 for top-level, 1 for direct reply, 2 for reply-to-reply)

### Requirement: YouTube threads.list API
The system SHALL fetch comment threads (not just top-level comments) from the YouTube API.

#### Scenario: Fetch Comment Threads
- **WHEN** the ingestion worker calls the YouTube API for a video
- **THEN** the system SHALL use commentThreads.list with `textFormat=plainText` and `maxResults=100` to fetch both top-level comments and their replies

#### Scenario: Reply Pagination
- **WHEN** a comment thread has more replies than fit in a single API response
- **THEN** the system SHALL follow the nextPageToken to fetch all replies for that thread

### Requirement: Job Crash Recovery
The system SHALL recover ingestion and LLM jobs left in an in-progress state by a crash.

#### Scenario: Crash Mid-Ingestion
- **GIVEN** an ingestion job stuck in `running` due to process crash
- **WHEN** the reaper runs at startup or on its tick
- **THEN** the system SHALL reset the job to `pending` and allow the channel to resume ingesting

#### Scenario: Stuck LLM Processing
- **GIVEN** an LLM processing job stuck in `processing` due to process crash
- **WHEN** the reaper runs
- **THEN** the system SHALL reset the job without enqueuing a duplicate LLM job for the same comments

### Requirement: Expiry-Aware Token Selection
The system SHALL prefer unexpired ingestion tokens when more than one is available.

#### Scenario: Primary Token Expired
- **GIVEN** a primary token past its expiry and an unexpired fallback token
- **WHEN** an ingestion token is selected
- **THEN** the system SHALL use the unexpired fallback token

### Requirement: Quota-Efficient Video Discovery

The system SHALL discover videos via the channel's uploads playlist using `playlistItems.list` (1 unit/request), resolving the playlist id once via `channels.list(part=contentDetails)` and caching it per channel. `search.list` (100 units) SHALL only be used as a bounded fallback when playlist metadata cannot be resolved, and auth or quota errors SHALL NOT be masked by the fallback.

#### Scenario: Playlist discovery after cache

- **GIVEN** a channel with a cached uploads playlist id
- **WHEN** an ingestion job discovers videos
- **THEN** the system SHALL iterate `playlistItems.list` pages and SHALL NOT call `search.list`

#### Scenario: First-time resolution cached

- **GIVEN** a channel without a cached uploads playlist id
- **WHEN** a job resolves it successfully
- **THEN** the system SHALL persist the id on the channel row for reuse by later jobs

#### Scenario: Bounded fallback

- **GIVEN** uploads-playlist resolution fails for a non-auth, non-quota reason
- **WHEN** the job proceeds
- **THEN** the system SHALL perform at most one `search.list` walk for the job and record fallback discovery telemetry

#### Scenario: Watermark early stop

- **GIVEN** a poll job with a per-channel sync watermark
- **WHEN** the newest-first playlist walk reaches a video published before the watermark
- **THEN** the system SHALL stop iterating without fetching further pages

### Requirement: Reply-Gated Deep Reply Fetching

The system SHALL preserve `totalReplyCount` and the count of replies embedded in each `commentThreads.list` response, and SHALL call `comments.list` for a thread only when `totalReplyCount` exceeds the embedded reply count.

#### Scenario: Zero-reply thread

- **WHEN** a top-level thread reports `totalReplyCount = 0`
- **THEN** the system SHALL NOT issue a `comments.list` request for it

#### Scenario: Inline-complete thread

- **WHEN** embedded replies equal `totalReplyCount`
- **THEN** the system SHALL NOT issue a `comments.list` request

#### Scenario: Missing deep replies

- **WHEN** `totalReplyCount` exceeds the embedded replies
- **THEN** the system SHALL paginate `comments.list` until the reply set is complete

### Requirement: Resumable Incremental Sync State

The system SHALL persist, per channel, the uploads playlist id and adaptive poll interval, and per video, a comment watermark, a commentThreads page checkpoint, and a skip status. Checkpoints SHALL be committed with comment batches so deadline, failure, or restart mid-walk resumes at a durable page boundary. Replays SHALL remain safe via the existing comment deduplication constraint.

#### Scenario: Deadline mid-walk

- **GIVEN** a job interrupted by its deadline after persisting a checkpoint
- **WHEN** the next job processes the same video
- **THEN** the system SHALL resume from the stored page token and SHALL NOT re-insert comments at or before the stored watermark

#### Scenario: Comments-disabled video

- **WHEN** a video returns `commentsDisabled`
- **THEN** the system SHALL mark the video's sync state `skipped` and SHALL NOT fetch its comments on later jobs

#### Scenario: Completed walk clears checkpoint

- **WHEN** a video's comment walk finishes without interruption
- **THEN** the system SHALL advance the watermark and clear the page checkpoint

### Requirement: YouTube Quota Budgets and Suppression

The system SHALL centralize YouTube method quota costs, meter units against a configurable daily project budget, enforce per-job ceilings on requests, videos, thread pages, and reply walks, and SHALL stop issuing requests once the budget is spent or a server `quotaExceeded` is seen (until the next UTC day). Quota exhaustion and ceiling stops SHALL mark jobs completed and defer channels WITHOUT consuming retry budget.

#### Scenario: Daily budget spent

- **GIVEN** the daily unit budget is exhausted
- **WHEN** a worker attempts another YouTube request
- **THEN** the client SHALL suppress the request locally, record suppression telemetry, and the job SHALL end completed with the channel deferred to the next budget window

#### Scenario: Server quotaExceeded arms hard stop

- **WHEN** any response carries reason `quotaExceeded`
- **THEN** subsequent admissions SHALL be refused until the next UTC day

#### Scenario: Job ceiling reached

- **GIVEN** a job has consumed its configured request (or video/page/reply) ceiling
- **WHEN** it would issue more
- **THEN** the system SHALL stop cleanly, keep durable checkpoints, and reschedule the remainder via normal polling

### Requirement: Adaptive Channel Polling

The system SHALL keep the configured poll interval as a floor, increase a channel's interval when jobs find no new content (up to a configured multiplier), and decrease it toward the floor when new content appears, while honoring existing error backoff.

#### Scenario: Silent channel backs off

- **GIVEN** a poll job that found no new videos and no new comments
- **WHEN** the job completes
- **THEN** the channel's poll interval SHALL grow toward the configured maximum

#### Scenario: Active channel speeds up

- **GIVEN** a poll job that ingested new content
- **WHEN** the job completes
- **THEN** the channel's poll interval SHALL shrink toward the configured floor

### Requirement: Partial Response Projections

All YouTube Data API requests issued by ingestion SHALL send a `fields` projection limited to the JSON paths the worker consumes.

#### Scenario: Request carries projection

- **WHEN** the client builds any ingestion request
- **THEN** the query SHALL include a `fields` parameter covering exactly the needed fields
