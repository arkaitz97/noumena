# Multi-Seat Workspaces Specification

## Purpose
Enables team collaboration through workspaces with role-based access control. Users can belong to multiple workspaces with different roles (Admin, Analyst, Viewer).

## Requirements

### Requirement: Default Workspace Creation
The system SHALL create a default personal workspace for each user upon registration.

#### Scenario: Personal Workspace on Registration
- **WHEN** a new user completes registration
- **THEN** the system SHALL create a workspace named "<email>'s Workspace" with the user as Admin

### Requirement: Workspace Member Roles
The system SHALL enforce three roles with specific permissions.

#### Scenario: Admin Permissions
- **WHEN** a user with Admin role accesses workspace resources
- **THEN** the system SHALL allow all operations: manage members, connect/remove channels, view/edit all data, manage billing

#### Scenario: Analyst Permissions
- **WHEN** a user with Analyst role accesses workspace resources
- **THEN** the system SHALL allow: view all data, export data, view insights. SHALL deny: star comments (admin-only), manage members, manage billing, connect/remove channels

#### Scenario: Viewer Permissions
- **WHEN** a user with Viewer role accesses workspace resources
- **THEN** the system SHALL allow: view feed, view insights, view digests. SHALL deny: star comments, export data, manage members, manage billing, connect/remove channels

### Requirement: Invite Team Members
The system SHALL allow Admins to invite users to a workspace via email.

#### Scenario: Send Invite
- **WHEN** an Admin calls POST /api/workspaces/{id}/invites with `{email, role}`
- **THEN** the system SHALL generate a cryptographically random invite token (7-day TTL), send an invite email with a signed link, and return 201 with invite_id

#### Scenario: Duplicate Invite
- **WHEN** an Admin invites an email that already has a pending invite
- **THEN** the system SHALL return 409 with "invite already pending for this email"

#### Scenario: Invite Rate Limit
- **WHEN** an Admin sends more than 10 invites within 24 hours
- **THEN** the system SHALL return 429 with "invite rate limit exceeded"

### Requirement: Accept Invite
The system SHALL allow invited users to join a workspace via the invite link.

#### Scenario: Accept Invite (Existing User)
- **WHEN** an existing user clicks the invite link (GET /invites/accept?token=...)
- **THEN** the system SHALL validate the token, create a workspace_members row with the specified role, consume the token, and redirect to /dashboard?workspace={id}

#### Scenario: Accept Invite (New User)
- **WHEN** a new user clicks the invite link and completes registration
- **THEN** the system SHALL create the user, add them to the workspace with the specified role, and redirect to /onboarding?workspace={id}

#### Scenario: Expired Invite
- **WHEN** a user clicks an invite link older than 7 days
- **THEN** the system SHALL return 410 with "invite expired" and offer to request a new invite

### Requirement: Workspace-Scoped Channel Access
The system SHALL scope channel visibility and access to the current workspace.

#### Scenario: List Workspace Channels
- **WHEN** a user calls GET /api/channels with an active workspace context
- **THEN** the system SHALL return only channels belonging to the current workspace

#### Scenario: Cross-Workspace Isolation
- **WHEN** a user attempts to access a channel from a different workspace
- **THEN** the system SHALL return 403 with "channel not accessible in this workspace"

#### Scenario: Shared Channel Grant
- **WHEN** an Analyst or Admin grants a channel to the workspace and a member requests its comments or insights
- **THEN** the system SHALL return the data

#### Scenario: Non-Member Request
- **WHEN** a user requests comments, insights, or stars a channel they neither track nor hold a grant for
- **THEN** the system SHALL deny the request

### Requirement: Role Enforcement Middleware
The system SHALL validate workspace role permissions on protected routes.

#### Scenario: Insufficient Role
- **WHEN** a Viewer attempts to call POST /api/channels/connect/youtube
- **THEN** the system SHALL return 403 with `{"code": "insufficient_role", "required": "admin"}`

#### Scenario: Valid Role
- **WHEN** an Analyst calls GET /api/comments/feed within their workspace
- **THEN** the system SHALL allow the request

### Requirement: Workspace Switching
The system SHALL allow users to switch between workspaces they belong to.

#### Scenario: List User Workspaces
- **WHEN** a user calls GET /api/workspaces/mine
- **THEN** the system SHALL return all workspaces the user belongs to with their role in each

#### Scenario: Switch Workspace
- **WHEN** a user sets their active workspace via PUT /api/workspaces/{id}/activate
- **THEN** the system SHALL validate membership, update the session context, and return 200 with workspace details

### Requirement: Remove Member
The system SHALL allow Admins to remove members from a workspace.

#### Scenario: Remove Member
- **WHEN** an Admin calls DELETE /api/workspaces/{id}/members/{user_id}
- **THEN** the system SHALL remove the workspace_members row, revoke the user's access to workspace channels, and return 204

#### Scenario: Self-Removal Prevention
- **WHEN** the last Admin attempts to remove themselves
- **THEN** the system SHALL return 400 with "cannot remove the last admin from a workspace"

### Requirement: Change Member Role
The system SHALL allow Admins to change a member's role.

#### Scenario: Change Role
- **WHEN** an Admin calls PATCH /api/workspaces/{id}/members/{user_id} with `{role: "analyst"}`
- **THEN** the system SHALL update the role, return 200, and the user's permissions SHALL update immediately

### Requirement: Server-Side Role Enforcement on Data Routes
The system SHALL enforce workspace roles server-side on all data routes: channel connect/remove, star/unstar, and member management require `admin`; export and insights require at least `analyst`; reads require any member (`viewer`+).

#### Scenario: Viewer Attempts a Write
- **GIVEN** a workspace member with the `viewer` role
- **WHEN** they call a channel-removal or channel-connect endpoint
- **THEN** the request is denied with 403

### Requirement: Single Workspace Fetch
The system SHALL return a single workspace for `GET /workspaces/{id}`.

#### Scenario: Fetch Own Workspace
- **GIVEN** a workspace the user belongs to
- **WHEN** `GET /workspaces/{id}` is called
- **THEN** exactly that workspace is returned

#### Scenario: Fetch Foreign Workspace
- **GIVEN** a workspace the user does not belong to
- **WHEN** `GET /workspaces/{id}` is called
- **THEN** a 404/403 is returned

### Requirement: Atomic Channel-Cap Enforcement
The system SHALL enforce plan channel limits atomically so concurrent connects cannot exceed the cap.

#### Scenario: Concurrent Connects at the Cap
- **GIVEN** a plan at its channel cap
- **WHEN** two connect requests arrive in parallel
- **THEN** at most one succeeds and the plan cap is not exceeded

### Requirement: Backfill Enqueued on First Track
The system SHALL enqueue a backfill ingestion job when a user first tracks a new platform channel.

#### Scenario: First Connect
- **GIVEN** a brand-new platform channel
- **WHEN** a user connects it
- **THEN** a backfill job is enqueued

#### Scenario: Reconnect
- **GIVEN** an already-tracked channel
- **WHEN** the user reconnects it
- **THEN** no duplicate backfill job is enqueued
