## MODIFIED Requirements

### Requirement: Digest Generation
The system SHALL generate weekly digests via LLM summarization on a scheduled cron, optionally incorporating user profile context.

#### Scenario: Successful Digest Generation
- **GIVEN** an active user with >= 10 processed comments in the last 7 days
- **WHEN** the digest cron runs (Monday 08:00 UTC)
- **THEN** the system SHALL fetch the user's comments (excluding toxic/spam), call the LLM to generate top 5 content ideas, top 5 unanswered questions, top 5 actionable insights by type, and engagement health score, store the result in weekly_digests with status='generated', and optionally email if the user has opted in

#### Scenario: Profile Context Injection
- **GIVEN** the feature flag `DIGEST_PROFILE_CONTEXT_ENABLED` is set to true and the user has a declared profile
- **WHEN** the weekly digest prompt is assembled
- **THEN** the system SHALL append sanitized user profile attributes (such as user_type, primary_goal, and content_topics) to guide LLM prioritization of content ideas and actionable insights

#### Scenario: Insufficient Comment Data
- **GIVEN** an active user with fewer than 10 processed comments in the last 7 days
- **WHEN** the digest cron runs
- **THEN** the system SHALL NOT generate a digest and SHALL NOT attempt to send an email

#### Scenario: LLM Unavailable During Digest
- **GIVEN** the LLM API is unavailable during digest generation
- **WHEN** the digest job attempts classification
- **THEN** the system SHALL retry up to 3 times over 2 hours. If all fail, mark the digest as status='deferred'
