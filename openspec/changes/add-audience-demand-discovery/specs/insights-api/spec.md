## ADDED Requirements

### Requirement: Demand discovery endpoint scope and access control

The insights API SHALL expose the demand-watch and discovery routes as workspace-scoped, read-only-by-default resources mounted alongside the existing insights routes, and SHALL apply the same authentication, rate limiting, and workspace-role enforcement as other insights endpoints. Mutating watch routes SHALL require at least the `analyst` workspace role.

#### Scenario: Workspace scoping

- **WHEN** a user calls any `/api/demand/*` route
- **THEN** the system resolves the workspace from the session and returns only resources belonging to that workspace

#### Scenario: Viewer cannot mutate watches

- **GIVEN** a workspace member with the `viewer` role
- **WHEN** they attempt to create, update, or delete a demand watch
- **THEN** the system returns 403 with the standard insufficient-role body

#### Scenario: Analyst can read discovery

- **GIVEN** a workspace member with the `analyst` role
- **WHEN** they request the creator shortlist for a watch in their workspace
- **THEN** the system allows the request

#### Scenario: Rate limited like other insights routes

- **WHEN** a user exceeds the configured request rate on `/api/demand/*`
- **THEN** the system returns 429 using the existing limiter
