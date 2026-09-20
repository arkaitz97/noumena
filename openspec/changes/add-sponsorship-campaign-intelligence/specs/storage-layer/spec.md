## ADDED Requirements

### Requirement: Campaign storage tables

The schema SHALL include `campaigns` and `campaign_assets`, created by a versioned migration and mirrored in the baseline schema. `campaigns` SHALL reference `workspaces(id)` with `ON DELETE CASCADE`. `campaign_assets` SHALL reference `campaigns(id)` and `content_assets(id)` with `ON DELETE CASCADE` and SHALL enforce `PRIMARY KEY(campaign_id, asset_id)`.

#### Scenario: Migration creates the tables

- **WHEN** the versioned migration is applied to a database at the prior version
- **THEN** `campaigns` and `campaign_assets` exist with the declared columns, constraints, and indexes
- **AND** the migration is recorded in `schema_migrations`

#### Scenario: Fresh install matches migrated install

- **WHEN** a database is created from the baseline schema
- **THEN** it contains the same campaign tables as a migrated database

#### Scenario: Workspace deletion cascades campaigns

- **WHEN** a `workspaces` row is deleted
- **THEN** its `campaigns` and their `campaign_assets` are deleted

#### Scenario: Deleting an asset detaches it

- **WHEN** a `content_assets` row is deleted
- **THEN** its `campaign_assets` rows are deleted and other assets in the campaign are unaffected

### Requirement: Comment to asset association

The schema SHALL add a nullable `comments.asset_id` referencing `content_assets(id)` with `ON DELETE SET NULL`, plus an index supporting campaign aggregate queries over `asset_id`. The migration SHALL opportunistically backfill `asset_id` for existing comments from stored assets where the video id is derivable.

#### Scenario: Column added

- **WHEN** the versioned migration is applied
- **THEN** `comments.asset_id` exists, is nullable, and is indexed

#### Scenario: Backfill associates derivable comments

- **GIVEN** existing comments whose channel and video id match a stored asset
- **WHEN** the migration runs
- **THEN** those comments have `asset_id` set

#### Scenario: Asset deletion preserves comments

- **WHEN** a `content_assets` row is deleted
- **THEN** its comments remain with `asset_id` set to NULL

#### Scenario: Re-run is idempotent

- **WHEN** the migration is re-applied
- **THEN** it completes without duplicating work or erroring
