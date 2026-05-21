## ADDED Requirements

### Requirement: Audit Trail for Deletion Events
The system SHALL log all deletion-related events to an audit trail for compliance and debugging.

#### Scenario: Deletion Request Logged
- **WHEN** a user requests account deletion
- **THEN** the system SHALL insert an audit_log row with action='deletion_requested', user_id, ip_address, and timestamp

#### Scenario: Deletion Cancelled Logged
- **WHEN** a pending deletion is cancelled
- **THEN** the system SHALL insert an audit_log row with action='deletion_cancelled', user_id, and timestamp

#### Scenario: Deletion Completed Logged
- **WHEN** the deletion worker completes a deletion job
- **THEN** the system SHALL insert an audit_log row with action='deletion_completed', user_id, channels_deleted count, and timestamp

### Requirement: Annual Plan Cancellation at Period End
The system SHALL handle annual plan cancellations by allowing access until the paid period expires.

#### Scenario: Annual Plan Deletion Request
- **WHEN** a user on an annual plan requests account deletion
- **THEN** the system SHALL cancel the Stripe subscription at period end (not immediately), set deletion_requested_at, and inform the user they retain access until the paid period expires

#### Scenario: Annual Plan Period Expires
- **WHEN** the annual plan period end date passes for a user with a pending deletion
- **THEN** the system SHALL proceed with the deletion worker pipeline

## MODIFIED Requirements

### Requirement: Account Deletion Request
The system SHALL allow users to request account deletion with email confirmation and a 30-day reversible window.

#### Scenario: Request Account Deletion
- **GIVEN** an authenticated user
- **WHEN** the user calls DELETE /api/user/account with email_confirmation matching their account email (case-insensitive)
- **THEN** the system SHALL set users.deletion_requested_at = NOW(), cancel the Stripe subscription if active (at period end for annual plans, immediately for monthly), insert an account_deletion_jobs row (idempotent), send a deletion_scheduled email with reversible_until timestamp (30 days from request), and return 202 with job_id, reversible_until, and status=scheduled

#### Scenario: Deletion Email Mismatch
- **GIVEN** an authenticated user
- **WHEN** the user submits an email_confirmation that does not match their account email
- **THEN** the system SHALL return 400

#### Scenario: Idempotent Deletion Request
- **GIVEN** a user who has already requested deletion (deletion_requested_at is set)
- **WHEN** the user calls DELETE /api/user/account again during the reversible window
- **THEN** the system SHALL return 202 with the existing reversible_until timestamp (no-op)

### Requirement: Cancel Pending Deletion
The system SHALL allow users to reverse a pending deletion within 30 days of request.

#### Scenario: Cancel Pending Deletion
- **GIVEN** a user with a deletion job in status='pending' and within 30 days of request
- **WHEN** the user calls POST /api/user/account/cancel-deletion
- **THEN** the system SHALL clear deletion_requested_at and return 200

#### Scenario: Cancel Non-Cancellable Deletion
- **GIVEN** a user whose deletion job is already claimed (status != 'pending') or past 30 days
- **WHEN** the user calls POST /api/user/account/cancel-deletion
- **THEN** the system SHALL return 409 with "no cancellable deletion found"
