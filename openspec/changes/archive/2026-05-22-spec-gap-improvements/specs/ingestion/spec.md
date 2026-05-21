## ADDED Requirements

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
