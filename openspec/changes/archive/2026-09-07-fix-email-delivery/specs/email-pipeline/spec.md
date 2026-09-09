## MODIFIED Requirements

### Requirement: Retried emails are delivered

The system SHALL actually send emails that were previously enqueued and failed transiently, until success or the attempt cap.

#### Scenario: Transient failure then retry
- **GIVEN** an email row left `pending` after a transient SMTP failure
- **WHEN** the retry worker processes it
- **THEN** the email is sent and the row transitions to `sent`

#### Scenario: Repeated failures
- **GIVEN** an email whose sends keep failing
- **WHEN** the attempt cap is reached
- **THEN** the row transitions to `failed` and a metric/alert is emitted

### Requirement: Idempotent enqueue without silent loss

The system SHALL only treat a duplicate `idempotency_key` as a no-op when the original row is `sent`.

#### Scenario: Duplicate key after failed first send
- **GIVEN** a first send failed leaving the row `pending`
- **WHEN** the same idempotency key is enqueued again
- **THEN** the email is not dropped and is eventually delivered

### Requirement: Digest email content

The system SHALL render weekly-digest emails from the digest's actual content.

#### Scenario: Digest email rendered
- **GIVEN** a weekly digest with top ideas, top questions, and sentiment
- **WHEN** the digest email is rendered
- **THEN** the email contains those items and a concrete sentiment value, never placeholder text

### Requirement: Unsubscribe on digest mail

The system SHALL provide a `List-Unsubscribe` mechanism and a working unsubscribe path for digest emails.

#### Scenario: Recipient unsubscribes
- **GIVEN** a digest email with an unsubscribe link
- **WHEN** the recipient follows the link
- **THEN** digest opt-in is disabled for that user without further emails

### Requirement: SMTP command deadlines

The system SHALL bound every SMTP command with a deadline so a stalled relay cannot hang a send indefinitely.

#### Scenario: Relay stalls mid-command
- **GIVEN** an SMTP relay that stops responding after connect
- **WHEN** a send is attempted
- **THEN** the send fails within the configured timeout rather than hanging
