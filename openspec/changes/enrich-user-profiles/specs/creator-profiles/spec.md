## ADDED Requirements

### Requirement: User Profile Retrieval
The system SHALL provide an endpoint `GET /api/user/profile` returning the authenticated user's progressive profile attributes.

#### Scenario: Profile Exists
- **WHEN** an authenticated user calls `GET /api/user/profile` and a profile record exists
- **THEN** the system SHALL return HTTP 200 with the profile object containing display_name, user_type, primary_goal, primary_platform, content_topics, audience_size_bucket, weekly_comment_volume_bucket, success_metric, role, team_size_bucket, publishing_frequency_bucket, insight_priorities, current_workflow, biggest_pain, updated_at, and created_at

#### Scenario: Profile Not Initialized
- **WHEN** an authenticated user calls `GET /api/user/profile` and no profile record exists yet
- **THEN** the system SHALL return HTTP 200 with an empty or null-populated default profile object and HTTP status 200 without creating a dirty record

#### Scenario: Unauthenticated Access
- **WHEN** an unauthenticated request is made to `GET /api/user/profile`
- **THEN** the system SHALL reject the request with HTTP 401 Unauthorized

### Requirement: User Profile Upsert and Validation
The system SHALL provide an endpoint `PATCH /api/user/profile` that validates and upserts profile attributes for the authenticated user.

#### Scenario: Valid Profile Update
- **WHEN** an authenticated user submits a valid JSON payload containing supported profile fields
- **THEN** the system SHALL upsert the `user_profiles` record for the user, update `updated_at`, and return HTTP 200 with the updated profile

#### Scenario: Invalid Enum Values
- **WHEN** a user submits an unknown value for an enum field (such as `user_type='unknown_tier'`)
- **THEN** the system SHALL return HTTP 400 Bad Request with a clear validation error identifying the invalid field and permissible values

#### Scenario: Content Topics Exceed Limit
- **WHEN** a user submits more than 10 items in `content_topics` or any single topic exceeds 40 characters
- **THEN** the system SHALL reject the update with HTTP 400 Bad Request

#### Scenario: Insight Priorities Exceed Limit
- **WHEN** a user submits more than 3 items in `insight_priorities` or unrecognized priorities
- **THEN** the system SHALL reject the update with HTTP 400 Bad Request

#### Scenario: Display Name Exceeds Limit
- **WHEN** a user submits a `display_name` exceeding 100 characters
- **THEN** the system SHALL reject the update with HTTP 400 Bad Request

