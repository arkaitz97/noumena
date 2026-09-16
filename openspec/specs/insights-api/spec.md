# Insights API Specification

## Purpose
Serves the comment feed, saved/starred comments, insights aggregates (actionable insights, toxicity, trends), and the export lifecycle for filtered comment data.

## Requirements

### Requirement: Comment Feed
The system SHALL provide a filterable, paginated feed of high-signal comments.

#### Scenario: Feed with Default Filters
- **GIVEN** an authenticated user with tracked channels
- **WHEN** the user calls GET /api/comments/feed with no query params
- **THEN** the system SHALL return up to 20 comments (default limit), sorted by newest, excluding toxic/spam by default, with total count and has_more pagination metadata

#### Scenario: Feed with Combined Filters
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/comments/feed with channels, categories, insight_type, date_from, date_to, search, and include_toxic parameters
- **THEN** the system SHALL apply all filters combinably and return matching comments within the configured query timeout (default 2000ms)

#### Scenario: Feed Query Timeout
- **GIVEN** a feed query that exceeds FEED_QUERY_TIMEOUT
- **WHEN** the query executes
- **THEN** the system SHALL return 408 with "feed_query_timeout"

#### Scenario: Count Query Timeout Degradation
- **GIVEN** a feed query where the count subquery exceeds COUNT_QUERY_TIMEOUT
- **WHEN** the feed is requested
- **THEN** the system SHALL return results with total=null and derive has_more from whether len(rows) == limit

#### Scenario: Limit Clamping
- **GIVEN** a limit parameter exceeding FEED_MAX_LIMIT (default 100)
- **WHEN** the user requests the feed
- **THEN** the system SHALL clamp the limit to FEED_MAX_LIMIT

#### Scenario: Invalid Sort Parameter
- **GIVEN** a sort value not in the allowed set
- **WHEN** the user requests the feed
- **THEN** the system SHALL return 400 with "invalid sort"

### Requirement: Saved Comments
The system SHALL provide a dedicated view of user-starred comments.

#### Scenario: Get Saved Feed
- **GIVEN** an authenticated user with starred comments
- **WHEN** the user calls GET /api/comments/saved
- **THEN** the system SHALL return starred comments sorted by starred_at DESC (sort parameter ignored), with has_more pagination metadata

### Requirement: Star/Unstar Comments
The system SHALL allow users to save and unsave comments for later review.

#### Scenario: Star a Comment
- **GIVEN** an authenticated user who tracks the channel containing a comment
- **WHEN** the user calls POST /api/comments/{id}/star
- **THEN** the system SHALL insert a user_comment_stars row (idempotent) and return 200. If the user does not have access to the comment, return 404.

#### Scenario: Unstar a Comment
- **GIVEN** an authenticated user who has starred a comment
- **WHEN** the user calls DELETE /api/comments/{id}/star
- **THEN** the system SHALL remove the star and return 204. If the user does not have access, return 404.

#### Scenario: Star Invalid Comment ID
- **GIVEN** a comment_id that is not a valid UUID
- **WHEN** the user calls POST /api/comments/{id}/star
- **THEN** the system SHALL return 400 with "invalid_comment_id"

### Requirement: Toxicity Shield
The system SHALL provide the percentage of comments filtered as toxic or spam.

#### Scenario: Get Toxicity Metrics
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/insights/toxicity-shield
- **THEN** the system SHALL return toxicity percentages (toxic_pct, spam_pct, count) for 7d and 30d windows

### Requirement: Trends
The system SHALL provide top topics and category distributions.

#### Scenario: Get Trends
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/insights/trends with days in {7, 30, 90} (default 30)
- **THEN** the system SHALL return top 10 topics and category distributions for the specified window

### Requirement: Export Lifecycle
The system SHALL support asynchronous and synchronous export of filtered comments to CSV or JSON.

#### Scenario: Create Small Export (Sync)
- **GIVEN** a filtered comment set under EXPORT_SYNC_THRESHOLD (default 50000)
- **WHEN** the user calls POST /api/comments/export with format=csv
- **THEN** the system SHALL encode to disk inline, mark the export completed, and return 200 with export_id, download_url, expires_at, and row_count

#### Scenario: Create Large Export (Async)
- **GIVEN** a filtered comment set exceeding EXPORT_SYNC_THRESHOLD or count query times out
- **WHEN** the user calls POST /api/comments/export
- **THEN** the system SHALL enqueue the export job and return 202 with job_id and status=queued

#### Scenario: Check Export Status
- **GIVEN** an export job
- **WHEN** the user calls GET /api/comments/export/{job_id}/status
- **THEN** the system SHALL return job_id, status, format, row_count, created_at, and when completed: started_at, completed_at, expires_at, download_url, or error

#### Scenario: Download Export
- **GIVEN** a completed, non-expired export
- **WHEN** the user calls GET /api/comments/export/{export_id}/download
- **THEN** the system SHALL serve the file with appropriate Content-Type (text/csv or application/json) and Content-Disposition header

#### Scenario: Expired Export Download
- **GIVEN** an export whose TTL has elapsed (default 24 hours)
- **WHEN** the user attempts to download
- **THEN** the system SHALL return 410 with "export_expired"

### Requirement: Insights API Feature Gate
The system SHALL gate all insights endpoints behind a feature flag.

#### Scenario: API Disabled
- **GIVEN** INSIGHTS_API_ENABLED=false
- **WHEN** any request hits an insights route
- **THEN** the system SHALL return 503 with "insights_api_disabled"

### Requirement: Latest Digest
The system SHALL provide the most recent weekly digest.

#### Scenario: Get Latest Digest
- **GIVEN** an authenticated user with at least one generated digest
- **WHEN** the user calls GET /api/insights/digest/latest
- **THEN** the system SHALL return the digest with id, week_start, week_end, and the JSONB content fields (top_ideas, top_questions, top_actionable_insights, health_score)

#### Scenario: No Digest Yet
- **GIVEN** an authenticated user with no generated digests
- **WHEN** the user calls GET /api/insights/digest/latest
- **THEN** the system SHALL return 404 with "no_digest_yet"

### Requirement: Global Search
The system SHALL provide a search endpoint that searches across all tracked channels.

#### Scenario: Global Search with Query
- **GIVEN** an authenticated user with tracked channels
- **WHEN** the user calls GET /api/comments/search?q=keyword with optional channel_id, category, date_from, date_to filters
- **THEN** the system SHALL return matching comments across all tracked channels (or scoped to channel_id if provided), sorted by relevance, with pagination

#### Scenario: Global Search Empty Results
- **GIVEN** an authenticated user
- **WHEN** the search query matches no comments
- **THEN** the system SHALL return 200 with empty results and a suggestion to broaden filters

#### Scenario: Global Search Minimum Query Length
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/comments/search with q shorter than 3 characters
- **THEN** the system SHALL return 400 with "search query must be at least 3 characters"

### Requirement: Bulk Star Operations
The system SHALL allow users to star or unstar multiple comments in a single request.

#### Scenario: Bulk Star Comments
- **GIVEN** an authenticated user who tracks the channels containing the comments
- **WHEN** the user calls POST /api/comments/bulk-star with `{comment_ids: [uuid, ...]}`
- **THEN** the system SHALL insert user_comment_stars rows for all accessible comments (idempotent), return 200 with `{starred_count, skipped_count}` where skipped are comments the user cannot access

#### Scenario: Bulk Unstar Comments
- **GIVEN** an authenticated user who has starred the comments
- **WHEN** the user calls POST /api/comments/bulk-unstar with `{comment_ids: [uuid, ...]}`
- **THEN** the system SHALL remove the stars for all accessible comments and return 200 with `{unstarred_count, skipped_count}`

### Requirement: Comment Thread View
The system SHALL provide a threaded view of comments with parent-child relationships.

#### Scenario: Get Comment Thread
- **GIVEN** an authenticated user who tracks the channel containing a comment
- **WHEN** the user calls GET /api/comments/{id}/thread
- **THEN** the system SHALL return the comment and all direct replies (parent_comment_id = comment.platform_comment_id), sorted by posted_at, up to 3 levels deep

#### Scenario: Thread Access Control
- **GIVEN** a comment from a channel the user does not track
- **WHEN** the user calls GET /api/comments/{id}/thread
- **THEN** the system SHALL return 403

### Requirement: Sentiment scoring
The system SHALL assign a closed-set sentiment (negative/neutral/positive) to processed comments and expose an aggregate endpoint.

#### Scenario: Comment processed by LLM
- **GIVEN** a comment processed by the classification pipeline
- **WHEN** classification completes
- **THEN** the comment's sentiment is persisted with the comment

#### Scenario: Sentiment trend requested
- **GIVEN** a channel the user can access
- **WHEN** `GET /api/insights/sentiment` is called with a valid `days` value
- **THEN** per-day sentiment aggregates are returned for the requested window

### Requirement: Comment Feed Filters
The feed API SHALL apply category, insight, intent, date, search, channel, toxicity, and pagination filters according to one documented contract, with predicates composed deterministically.

#### Scenario: Insight type appears after the first JSON element
- **GIVEN** a comment has multiple actionable insights and the requested type matches only the second or later element
- **WHEN** the feed is requested with that insight type
- **THEN** the comment SHALL be returned

#### Scenario: Ideas intent uses OR semantics
- **GIVEN** a comment has category `content_idea` but no `video_idea` insight, or has a `video_idea` insight but another category
- **WHEN** the feed is requested with `intent=ideas`
- **THEN** the comment SHALL be returned in either case
- **AND** additional channel/date/search/toxicity filters SHALL still apply

#### Scenario: Explicit empty categories
- **GIVEN** the request explicitly encodes an empty category selection
- **WHEN** the feed is queried
- **THEN** the category predicate SHALL match no comments
- **AND** it SHALL not silently fall back to the default high-signal categories

#### Scenario: Omitted categories
- **GIVEN** the request omits the categories parameter
- **WHEN** the feed is queried
- **THEN** the API SHALL apply the documented canonical default category set
- **AND** that set SHALL be `question`, `content_idea`, `collaboration_ask`, `praise`, and `constructive_criticism`

#### Scenario: Date-only end boundary
- **GIVEN** `date_to` is a date-only value
- **WHEN** the feed is queried
- **THEN** comments posted at any time on that UTC calendar day SHALL be included
- **AND** comments on the following day SHALL be excluded

#### Scenario: Literal search characters
- **GIVEN** a user searches for `%` or `_`
- **WHEN** the feed is queried
- **THEN** those characters SHALL be treated literally rather than as SQL `LIKE` wildcards

### Requirement: Intent and aggregate scope
The API SHALL expose intent predicates and SHALL apply the same complete feed scope to dashboard aggregate responses.

#### Scenario: Multi-channel aggregate scope
- **GIVEN** a user selects multiple channels and a custom date range in the feed
- **WHEN** the dashboard requests aggregates
- **THEN** every aggregate SHALL cover the same selected channels and date range

#### Scenario: All-time scope reset
- **GIVEN** a user changes a date range from a bounded range to all-time
- **WHEN** feed and aggregate requests are issued
- **THEN** neither request SHALL retain the previous bounded dates

### Requirement: Saved comments defaults
The Saved comments endpoint SHALL return all accessible starred comments by default, sorted by `starred_at` descending, without applying the ordinary high-signal category/toxicity defaults unless the caller explicitly requests them.

#### Scenario: Starred low-signal comment
- **GIVEN** a user starred a comment categorized as `other`, toxic, or spam
- **WHEN** the user opens Saved without explicit filters
- **THEN** the starred comment SHALL be present
