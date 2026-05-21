## ADDED Requirements

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
