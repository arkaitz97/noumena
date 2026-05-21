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
- **THEN** the system SHALL allow: view all data, export data, star comments, view insights. SHALL deny: manage members, manage billing, connect/remove channels

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
