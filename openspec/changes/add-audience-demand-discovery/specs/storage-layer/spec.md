## ADDED Requirements

### Requirement: Demand discovery storage tables

The schema SHALL include `demand_watches`, `demand_watch_terms`, and `channel_discovery_consent`, created by a versioned migration and mirrored in the baseline schema for fresh installs. `demand_watches` SHALL reference `workspaces(id)` with `ON DELETE CASCADE`. `demand_watch_terms` SHALL reference `demand_watches(id)` with `ON DELETE CASCADE` and SHALL enforce `UNIQUE(demand_watch_id, term)`. `channel_discovery_consent` SHALL reference `user_channel_tracking(id)` with `ON DELETE CASCADE` and SHALL enforce one consent row per tracking row.

#### Scenario: Migration creates the tables

- **WHEN** the versioned migration is applied to a database at the prior version
- **THEN** `demand_watches`, `demand_watch_terms`, and `channel_discovery_consent` exist with the declared columns, constraints, and indexes
- **AND** the migration is recorded in `schema_migrations`

#### Scenario: Fresh install matches migrated install

- **WHEN** a database is created from the baseline schema
- **THEN** it contains the same table definitions as a migrated database

#### Scenario: Workspace deletion cascades

- **WHEN** a `workspaces` row is deleted
- **THEN** its `demand_watches` and their `demand_watch_terms` are deleted

#### Scenario: Tracking deletion cascades consent

- **WHEN** a `user_channel_tracking` row is deleted (including via account deletion)
- **THEN** its `channel_discovery_consent` row is deleted

### Requirement: Opt-in consent index

The schema SHALL provide a partial index over `channel_discovery_consent(user_channel_tracking_id)` where `opted_in = 1` so the discovery query can resolve opted-in trackers without scanning non-consenting rows.

#### Scenario: Discovery query is index-assisted

- **WHEN** the discovery query resolves opted-in trackers
- **THEN** the query plan uses the partial consent index rather than a full scan
