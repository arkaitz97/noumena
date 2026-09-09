## MODIFIED Requirements

### Requirement: Server-side role enforcement

The system SHALL enforce workspace roles server-side on all data routes, not just in the UI.

#### Scenario: Viewer attempts a write
- **GIVEN** a workspace member with the `viewer` role
- **WHEN** they call a channel-removal or channel-connect endpoint
- **THEN** the request is denied with 403

#### Scenario: Analyst exports
- **GIVEN** a workspace member with the `analyst` role
- **WHEN** they request an export or insights
- **THEN** the request succeeds

#### Scenario: Admin manages members
- **GIVEN** a workspace member with the `admin` role
- **WHEN** they invite or change a member role
- **THEN** the request succeeds

### Requirement: Workspace-scoped channel access

The system SHALL scope channel access to the user's workspace: channels are visible when the user tracks them or when a grant links the channel to one of the user's workspaces.

#### Scenario: Workspace member sees shared channel
- **GIVEN** a grant linking a channel to the user's workspace
- **WHEN** the member requests that channel's comments or insights
- **THEN** the data is returned

#### Scenario: Non-member request
- **GIVEN** a channel not tracked by the user and no grant to any of their workspaces
- **WHEN** the user requests its comments, insights, or stars it
- **THEN** the request is denied

### Requirement: Single workspace fetch

The system SHALL return a single workspace for `GET /workspaces/{id}`.

#### Scenario: Fetch own workspace
- **GIVEN** a workspace the user belongs to
- **WHEN** `GET /workspaces/{id}` is called
- **THEN** exactly that workspace is returned

#### Scenario: Fetch foreign workspace
- **GIVEN** a workspace the user does not belong to
- **WHEN** `GET /workspaces/{id}` is called
- **THEN** a 404/403 is returned

### Requirement: Atomic channel-cap enforcement

The system SHALL enforce plan channel limits atomically so concurrent connects cannot exceed the cap.

#### Scenario: Concurrent connects at the cap
- **GIVEN** a plan at its channel cap
- **WHEN** two connect requests arrive in parallel
- **THEN** at most one succeeds and the plan cap is not exceeded

### Requirement: Backfill enqueued on first track

The system SHALL enqueue a backfill ingestion job when a user first tracks a new platform channel.

#### Scenario: First connect
- **GIVEN** a brand-new platform channel
- **WHEN** a user connects it
- **THEN** a backfill job is enqueued

#### Scenario: Reconnect
- **GIVEN** an already-tracked channel
- **WHEN** the user reconnects it
- **THEN** no duplicate backfill job is enqueued
