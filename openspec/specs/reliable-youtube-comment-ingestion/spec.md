# Reliable YouTube Comment Ingestion Specification

## Purpose
Provides durable, resumable YouTube comment polling and backfill with complete reply retrieval and explicit error and credential scope handling.

## Requirements

### Requirement: Resumable YouTube channel polling
The ingestion system SHALL persist channel poll discovery/comment progress at resumable page boundaries and SHALL advance the channel watermark only after the scan reaches its declared boundary.

#### Scenario: Poll stops after a page ceiling
- **GIVEN** a channel poll stops after processing only part of the available comment pages
- **WHEN** the next poll runs
- **THEN** it SHALL resume from durable progress or a safe overlap boundary
- **AND** it SHALL not treat unscanned older pages as synchronized

#### Scenario: Poll reaches its boundary
- **GIVEN** a channel poll has consumed all pages through its scan boundary and completed required reply reconciliation
- **WHEN** the worker finalizes the poll
- **THEN** it SHALL persist the new watermark and mark the poll progress complete

#### Scenario: Poll page fails
- **GIVEN** a comment page request returns an unexpected API error
- **WHEN** the worker handles the error
- **THEN** it SHALL retain the last committed checkpoint, record the failure, and schedule retry
- **AND** it SHALL not advance the watermark past the failed page

### Requirement: Resumable YouTube backfill discovery
The ingestion system SHALL persist uploads discovery progress independently of the per-video comment watermark and SHALL not restart a partial backfill at the newest video.

#### Scenario: Backfill video ceiling
- **GIVEN** a backfill reaches its configured maximum videos after examining a prefix of uploads
- **WHEN** a continuation backfill runs
- **THEN** it SHALL continue from the durable discovery cursor
- **AND** it SHALL remain incomplete until all required uploads and comment walks are covered

#### Scenario: Recent comment on an old video
- **GIVEN** a video was published before the configured backfill window
- **AND** the video contains a comment posted inside the configured window
- **WHEN** the backfill enumerates uploads and fetches comments
- **THEN** the comment SHALL be eligible for ingestion based on its own posted time

### Requirement: Complete reply retrieval
The ingestion system SHALL reconcile replies for eligible old and new threads, follow every available reply page, and preserve each reply's actual parent comment ID.

#### Scenario: Late reply to an old thread
- **GIVEN** a stored top-level comment is older than the current comment watermark
- **AND** a new retrievable reply has been added to that thread
- **WHEN** the thread enters the overlap or reply-reconciliation pass
- **THEN** the new reply SHALL be fetched and stored

#### Scenario: Reply page failure
- **GIVEN** one page of a multi-page reply walk fails
- **WHEN** the worker handles the failure
- **THEN** it SHALL preserve the reply checkpoint and retry the incomplete walk
- **AND** it SHALL not report the thread's reply walk as complete

#### Scenario: Reply parent normalization
- **GIVEN** a reply resource contains a parent ID
- **WHEN** the resource is normalized
- **THEN** `parent_comment_id` SHALL equal that resource parent ID

### Requirement: Safe ingestion error and scope handling
The ingestion system SHALL distinguish retrievable published video comments from unsupported or unauthorized data and SHALL never silently convert an API page error into synchronization completion.

#### Scenario: Comments-disabled video
- **GIVEN** YouTube explicitly reports that comments are disabled for a video
- **WHEN** the worker processes the response
- **THEN** it SHALL mark that video as skipped with the reason
- **AND** it SHALL not retry the video as if it were a transient page failure

#### Scenario: Channel-level comment without video association
- **GIVEN** a channel-wide response contains a comment with no associated video ID
- **WHEN** the worker normalizes the response
- **THEN** it SHALL exclude it from the video comment corpus or store it in an explicitly separate channel-comment scope
- **AND** it SHALL not assign a fabricated video ID

#### Scenario: Credential scope is limited
- **GIVEN** ingestion is using a public/API-key credential
- **WHEN** comments are fetched
- **THEN** the system SHALL report the published-only retrieval scope
- **AND** it SHALL use owner-authorized credentials when the configured product scope includes creator-visible moderation states
