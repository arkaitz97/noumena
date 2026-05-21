## MODIFIED Requirements

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

## REMOVED Requirements

### Requirement: Sentiment Aggregates
**Reason**: Sentiment analysis is being replaced by actionable insights extraction. Sentiment scores do not provide actionable value for creators.
**Migration**: Clients using GET /api/insights/sentiment should migrate to GET /api/insights/actionable-insights/top and GET /api/insights/actionable-insights/distribution. Feed query params sentiment_min and sentiment_max are removed; use insight_type instead.
