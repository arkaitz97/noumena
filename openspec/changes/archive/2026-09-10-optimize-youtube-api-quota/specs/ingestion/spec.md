## ADDED Requirements

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
