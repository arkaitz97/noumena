# Email Pipeline Specification

## Purpose
Handles transactional email delivery via SMTP with idempotency, template rendering, and automatic retry for transient failures.

## Requirements

### Requirement: Email Send Pipeline
The system SHALL send transactional emails through an SMTP pipeline with idempotency.

#### Scenario: Successful Email Send
- **GIVEN** EMAIL_ENABLED=true and valid SMTP configuration
- **WHEN** Pipeline.Send is called with a valid Message (template, to_address, idempotency_key, variables)
- **THEN** the system SHALL render the template (HTML + text), insert an email_send_log row with status='pending', attempt_count=1, and the persisted rendered body (html_body, text_body), recipient (to_address), and subject; send via SMTP; and on success update the row to status='sent' with sent_at=NOW()

#### Scenario: Idempotency Short-Circuit
- **GIVEN** an idempotency_key already associated with a sent email
- **WHEN** Pipeline.Send is called with the same key
- **THEN** the system SHALL return nil (no-op) without re-sending

#### Scenario: Idempotency Race Resolution
- **GIVEN** two concurrent Send calls with the same idempotency_key
- **WHEN** both attempt to insert the log row
- **THEN** the system SHALL enforce the unique constraint; the second caller SHALL read the existing row and only treat it as a no-op when the row status is 'sent'

#### Scenario: Duplicate key after failed first send
- **GIVEN** a first send failed leaving the row `pending` (or the attempt cap left it `failed`)
- **WHEN** the same idempotency key is enqueued again
- **THEN** the email is not dropped and is eventually delivered (the send is re-attempted from the persisted body; the 3-attempt cap still applies)

#### Scenario: SMTP Transient Failure
- **GIVEN** a valid email send that encounters a transient SMTP error
- **WHEN** Pipeline.Send attempts delivery
- **THEN** the system SHALL leave the row at status='pending' with last_error set, return an error to the caller, and allow the retry worker to pick it up

#### Scenario: Email Disabled Kill-Switch
- **GIVEN** EMAIL_ENABLED=false
- **WHEN** Pipeline.Send is called
- **THEN** the system SHALL log "email_disabled_skipped" and return nil without writing to email_send_log

#### Scenario: Missing Required Fields
- **GIVEN** a Message with empty idempotency_key, template, or to_address
- **WHEN** Pipeline.Send is called
- **THEN** the system SHALL return an error specifying which field is required

### Requirement: Email Templates
The system SHALL support HTML and text template rendering with shared variables.

#### Scenario: Template Rendering
- **GIVEN** a registered template name matching the templates registry
- **WHEN** Send renders the message
- **THEN** the system SHALL use html/template for the HTML body and text/template for the text body, sharing the same Variables map

#### Scenario: Unknown Template
- **GIVEN** a template name not in the registry
- **WHEN** Send attempts to render
- **THEN** the system SHALL return an error before writing any log row

### Requirement: Email Retry Worker
The system SHALL retry failed email sends with exponential backoff.

#### Scenario: Retry Pending Email
- **GIVEN** an email_send_log row with status='pending' and attempt_count < 3
- **WHEN** the retry worker ticks (default every 60 seconds)
- **THEN** the system SHALL claim the row and re-send the persisted recipient, subject, and rendered body (html_body/text_body); on success mark the row sent; on failure bump attempt_count, and after the cap is reached mark the row failed with a terminal metric

#### Scenario: Retry Budget Exhausted
- **GIVEN** an email_send_log row whose next attempt would exceed the cap
- **WHEN** the retry worker processes it
- **THEN** the system SHALL mark the row as status='failed' with "retry budget exhausted", log the failure, and emit the terminal metric

#### Scenario: Retry Without Recipient
- **GIVEN** a pending email log row with an empty persisted to_address (legacy row)
- **WHEN** the retry worker processes it
- **THEN** the system SHALL mark the row as failed with "retry-without-recipient" and log a structured event for operator intervention

### Requirement: Email Address Redaction
The system SHALL redact email addresses from log output.

#### Scenario: Redact Email in Logs
- **GIVEN** an email address user@example.com
- **WHEN** the pipeline logs a send event
- **THEN** the system SHALL output "u***@example.com" (first character + *** + domain)

### Requirement: Digest Email Template
The system SHALL support a weekly digest email template with LLM-generated content.

#### Scenario: Render Digest Email
- **WHEN** the pipeline renders a digest_email template
- **THEN** the system SHALL include top 5 content ideas, top 5 unanswered questions, sentiment trend summary (when sentiment data is available), and health score in both HTML and text formats, and NEVER render Go's "<no value>" placeholder

### Requirement: Unsubscribe on digest mail
The system SHALL provide a `List-Unsubscribe` mechanism and a working unsubscribe path for digest emails.

#### Scenario: Recipient unsubscribes
- **GIVEN** a digest email with an unsubscribe link
- **WHEN** the recipient follows the link
- **THEN** digest opt-in is disabled for that user without further emails

#### Scenario: Forged unsubscribe link
- **GIVEN** an unsubscribe link whose HMAC signature does not verify (or whose expiry has passed)
- **WHEN** the recipient follows the link
- **THEN** the request SHALL be rejected (403) and the user's opt-in SHALL NOT change

### Requirement: SMTP command deadlines
The system SHALL bound every SMTP command with a deadline so a stalled relay cannot hang a send indefinitely.

#### Scenario: Relay stalls mid-command
- **GIVEN** an SMTP relay that stops responding after connect
- **WHEN** a send is attempted
- **THEN** the send fails within the configured timeout (SMTP_CMD_TIMEOUT, default 10s) rather than hanging

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