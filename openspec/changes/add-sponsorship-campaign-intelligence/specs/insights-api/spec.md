## ADDED Requirements

### Requirement: Campaign read endpoint scope and access control

The insights API SHALL expose the campaign summary, creator comparison, and export routes as workspace-scoped read endpoints mounted alongside the existing insights routes, applying the same authentication, rate limiting, and workspace-role enforcement. Report export SHALL require at least the `analyst` role.

#### Scenario: Workspace scoping

- **WHEN** a member calls any `/api/campaigns/{id}/*` read route
- **THEN** the system resolves the workspace from the session and returns only data for campaigns in that workspace

#### Scenario: Viewer can read the summary

- **GIVEN** a workspace member with the `viewer` role
- **WHEN** they request a campaign summary in their workspace
- **THEN** the system allows the request

#### Scenario: Viewer cannot export

- **GIVEN** a workspace member with the `viewer` role
- **WHEN** they request a campaign report export
- **THEN** the system returns 403 with the standard insufficient-role body

#### Scenario: Rate limited like other insights routes

- **WHEN** a user exceeds the configured request rate on `/api/campaigns/*`
- **THEN** the system returns 429 using the existing limiter
