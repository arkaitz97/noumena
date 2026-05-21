## ADDED Requirements

### Requirement: Digest Email Template
The system SHALL support a weekly digest email template with LLM-generated content.

#### Scenario: Render Digest Email
- **WHEN** the pipeline renders a digest_email template
- **THEN** the system SHALL include top 5 content ideas, top 5 unanswered questions, sentiment trend summary, and health score in both HTML and text formats

### Requirement: Trial Expiry Email Template
The system SHALL support a trial expiry reminder email template.

#### Scenario: Render Trial Expiry Email
- **WHEN** the pipeline renders a trial_expiry template
- **THEN** the system SHALL include days remaining, current plan features, and upgrade link in both HTML and text formats

### Requirement: Deletion Scheduled Email Template
The system SHALL support an account deletion scheduled confirmation email.

#### Scenario: Render Deletion Scheduled Email
- **WHEN** the pipeline renders a deletion_scheduled template
- **THEN** the system SHALL include reversible_until timestamp, what data will be deleted, and contact support link in both HTML and text formats

### Requirement: Deletion Completed Email Template
The system SHALL support an account deletion completion notification email.

#### Scenario: Render Deletion Completed Email
- **WHEN** the pipeline renders a deletion_completed template
- **THEN** the system SHALL include confirmation of deletion and data retention summary in both HTML and text formats

### Requirement: Payment Grace Final Email Template
The system SHALL support a final payment grace period expiry email.

#### Scenario: Render Payment Grace Final Email
- **WHEN** the pipeline renders a payment_grace_final template
- **THEN** the system SHALL include notice of account pause, data retention policy, and reactivation link in both HTML and text formats

### Requirement: Export Ready Email Template
The system SHALL support a data export ready notification email.

#### Scenario: Render Export Ready Email
- **WHEN** the pipeline renders an export_ready template
- **THEN** the system SHALL include download link, expiry timestamp, and file size in both HTML and text formats
