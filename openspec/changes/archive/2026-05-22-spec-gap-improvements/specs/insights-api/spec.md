## ADDED Requirements

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
