## MODIFIED Requirements

### Requirement: Deletion request succeeds

The system SHALL accept an account-deletion request and enqueue an asynchronous deletion job.

#### Scenario: User requests deletion
- **GIVEN** an authenticated user
- **WHEN** they request account deletion
- **THEN** the endpoint succeeds, sets `deletion_requested_at`, and enqueues a pending deletion job

### Requirement: Complete data erasure

When a user is hard-deleted, the system SHALL remove every row owned by that user across all tables, regardless of foreign-key enforcement state.

#### Scenario: Hard delete completes
- **GIVEN** a deletion job reaching its final stage
- **WHEN** the user row is removed
- **THEN** no rows referencing that user remain in any user-owned table (tokens, oauth states, stars, usage logs, exports, memberships, rate-limit buckets, digests, tracking)

#### Scenario: Shared channel with another owner
- **GIVEN** a channel also tracked by another user
- **WHEN** one user is deleted
- **THEN** only that user's tracking/grants are removed and the shared channel data is preserved

### Requirement: OAuth refresh token revocation

The system SHALL revoke the user's OAuth refresh token in addition to the access token during deletion.

#### Scenario: Delete revokes provider tokens
- **GIVEN** a user with linked YouTube OAuth tokens
- **WHEN** the deletion job runs the revoke stage
- **THEN** both access and refresh tokens are revoked at the provider, and an already-invalid token is not treated as an error

### Requirement: Workspace deletion cleanup

The system SHALL clear member references when a workspace is deleted.

#### Scenario: Workspace deleted while active for members
- **GIVEN** members whose `active_workspace_id` points at the workspace
- **WHEN** the workspace is deleted
- **THEN** the members' `active_workspace_id` is NULLed and channel tracking is cleared, with no dangling references

### Requirement: Free-tier quota reset schedule

The system SHALL assign every free user a `quota_reset_at` so monthly resets always apply.

#### Scenario: New free user created
- **GIVEN** a new user on a free plan
- **WHEN** the account is created
- **THEN** `quota_reset_at` is set to approximately now + 30 days
