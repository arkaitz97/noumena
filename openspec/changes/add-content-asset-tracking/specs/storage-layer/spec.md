## ADDED Requirements

### Requirement: Content asset storage table

The schema SHALL include a `content_assets` table created by a versioned migration and mirrored in the baseline schema for fresh installs. `content_assets.platform_channel_id` SHALL reference `platform_channels(id)` with `ON DELETE CASCADE`, and the table SHALL enforce `UNIQUE(platform_channel_id, platform_asset_id)`. The table SHALL store engagement counts, `is_sponsored`, `sponsored_signals` as JSON text, and `metrics_refreshed_at`.

#### Scenario: Migration creates the table

- **WHEN** the versioned migration is applied to a database at the prior version
- **THEN** `content_assets` exists with the declared columns, constraints, and indexes
- **AND** the migration is recorded in `schema_migrations`

#### Scenario: Fresh install matches migrated install

- **WHEN** a database is created from the baseline schema
- **THEN** it contains the same `content_assets` definition as a migrated database

#### Scenario: Duplicate asset rejected

- **WHEN** two rows are inserted with the same `(platform_channel_id, platform_asset_id)`
- **THEN** the second insert is rejected by the unique constraint

#### Scenario: Deleting a channel cascades assets

- **WHEN** a `platform_channels` row is deleted
- **THEN** its `content_assets` rows are deleted

### Requirement: Content asset refresh index

The schema SHALL provide an index supporting the metrics-refresh scan over `metrics_refreshed_at` and a partial index over sponsored assets.

#### Scenario: Refresh scan is index-assisted

- **WHEN** the refresh pass selects assets due for a metric update
- **THEN** the query plan uses the `metrics_refreshed_at` index rather than a full scan
