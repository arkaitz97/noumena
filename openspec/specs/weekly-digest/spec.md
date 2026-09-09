# Weekly Digest Specification

## Purpose
Generates AI-curated weekly insight digests summarizing top trends, unanswered questions, content ideas, and engagement health for each active user.

## Requirements

### Requirement: Digest Generation
The system SHALL generate weekly digests via LLM summarization on a scheduled cron.

#### Scenario: Successful Digest Generation
- **GIVEN** an active user with >= 10 processed comments in the last 7 days
- **WHEN** the digest cron runs (Monday 08:00 UTC)
- **THEN** the system SHALL fetch the user's comments (excluding toxic/spam), call the LLM to generate top 5 content ideas, top 5 unanswered questions, top 5 actionable insights by type, and engagement health score, store the result in weekly_digests with status='generated', and optionally email if the user has opted in

#### Scenario: Insufficient Comment Data
- **GIVEN** an active user with fewer than 10 processed comments in the last 7 days
- **WHEN** the digest cron runs
- **THEN** the system SHALL NOT generate a digest and SHALL NOT attempt to send an email

#### Scenario: LLM Unavailable During Digest
- **GIVEN** the LLM API is unavailable during digest generation
- **WHEN** the digest job attempts classification
- **THEN** the system SHALL retry up to 3 times over 2 hours. If all fail, mark the digest as status='deferred'

### Requirement: Digest Email Opt-In
The system SHALL allow users to toggle digest email delivery.

#### Scenario: Opt In to Digest Emails
- **GIVEN** an authenticated user
- **WHEN** the user calls PATCH /api/user/preferences with digest_email_opt_in=true
- **THEN** the system SHALL persist the preference and return 200

#### Scenario: Opt Out of Digest Emails
- **GIVEN** an authenticated user currently opted in
- **WHEN** the user calls PATCH /api/user/preferences with digest_email_opt_in=false
- **THEN** the system SHALL persist the preference and return 200. The user will still see digests in the dashboard but will not receive emails.

#### Scenario: Empty Preferences Body
- **GIVEN** an authenticated user
- **WHEN** the user calls PATCH /api/user/preferences with an empty body
- **THEN** the system SHALL return 400 with "no_supported_fields"

### Requirement: Digest Listing
The system SHALL provide paginated access to a user's historical digests.

#### Scenario: List Digests
- **GIVEN** an authenticated user with generated digests
- **WHEN** the user calls GET /api/digests with limit (default 10, max 50) and offset
- **THEN** the system SHALL return digests sorted by week_start_date DESC with pagination metadata

#### Scenario: Invalid Limit
- **GIVEN** a limit parameter <= 0
- **WHEN** the user calls GET /api/digests
- **THEN** the system SHALL return 400 with "invalid_limit"

### Requirement: Latest Digest
The system SHALL provide the most recent digest for quick dashboard display.

#### Scenario: Get Latest Digest
- **GIVEN** an authenticated user with at least one digest
- **WHEN** the user calls GET /api/digests/latest
- **THEN** the system SHALL return the most recent digest

#### Scenario: No Digests Yet
- **GIVEN** an authenticated user with no digests
- **WHEN** the user calls GET /api/digests/latest
- **THEN** the system SHALL return 404 with {"message": "no digest yet"}

### Requirement: Paused Channel Exclusion
The system SHALL exclude paused channels from digest computation.

#### Scenario: Digest Excludes Paused Channels
- **GIVEN** a user with both active and paused channels
- **WHEN** the digest is generated
- **THEN** the system SHALL only include comments from channels with tracking_status='active' at the time of generation

### Requirement: Digest Email Delivery
The system SHALL email generated digests to users who have opted in.

#### Scenario: Email Generated Digest
- **GIVEN** a user with digest_email_opt_in=true and a successfully generated digest
- **WHEN** the digest cron completes generation
- **THEN** the system SHALL send a digest_email via the email pipeline with the digest content, mark emailed_at on the weekly_digests row, and increment digest_emails_sent counter

#### Scenario: Skip Email for Opted-Out User
- **GIVEN** a user with digest_email_opt_in=false
- **WHEN** the digest cron generates a digest
- **THEN** the system SHALL store the digest but NOT send an email

#### Scenario: Email Delivery Failure
- **GIVEN** a digest email that fails to send
- **WHEN** the email pipeline returns an error
- **THEN** the system SHALL log the failure, mark the digest as emailed_at=NULL, and allow the email retry worker to attempt redelivery

### Requirement: Deferred Digest Retry Email
The system SHALL notify users when their digest is delayed due to LLM unavailability.

#### Scenario: Notify User of Deferred Digest
- **GIVEN** a digest marked as status='deferred' after exhausting retries
- **WHEN** the digest cron completes
- **THEN** the system SHALL send a digest_delayed email to opted-in users explaining the delay

### Requirement: Per-Channel Digest Exclusion Details
The system SHALL explicitly exclude paused channels from digest comment aggregation.

#### Scenario: Digest Query Excludes Paused
- **WHEN** the digest worker fetches comments for generation
- **THEN** the system SHALL filter to only comments from channels where the user's tracking_status='active' at generation time

#### Scenario: Channel Paused Mid-Week
- **GIVEN** a user who pauses a channel on Wednesday
- **WHEN** the Monday digest runs
- **THEN** the system SHALL exclude comments from the paused channel for the entire week (digest covers Mon-Sun, pause state at generation time applies)

### Requirement: Missed Digest Catch-Up
The system SHALL generate a weekly digest even when the scheduled trigger time was missed.

#### Scenario: Downtime Across Scheduled Time
- **GIVEN** application downtime spanning Monday 08:00 UTC
- **WHEN** the application restarts
- **THEN** the system SHALL generate the missed digest from the available data

### Requirement: Deferred Digest Re-Dispatch
The system SHALL retry digests marked `deferred` due to LLM failure instead of dropping them.

#### Scenario: Deferred Digest Retry
- **GIVEN** a digest marked `deferred` after LLM failures
- **WHEN** its retry schedule comes due
- **THEN** the system SHALL dispatch it again, eventually generating the digest or reaching the retry cap
