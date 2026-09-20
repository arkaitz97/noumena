## ADDED Requirements

### Requirement: Campaign attribution storage tables

The schema SHALL include a `user_attribution` table keyed 1:1 by `user_id` and a `product_events` append-only table, both created by a versioned migration and mirrored in the baseline schema for fresh installs. Both tables SHALL declare `user_id` as a foreign key to `users(id)` with `ON DELETE CASCADE`. `user_attribution.sra_source` SHALL be constrained to the allowed SRA enum. `product_events` SHALL permit a null `user_id` to represent anonymous landing views. Indexes SHALL support querying events by user and by event name over time.

#### Scenario: Migration creates both tables

- **WHEN** the versioned migration is applied to a database at the prior version
- **THEN** `user_attribution` and `product_events` exist with the declared columns, constraints, and indexes
- **AND** the migration is recorded in `schema_migrations`

#### Scenario: Fresh install matches migrated install

- **WHEN** a database is created from the baseline schema
- **THEN** it contains the same `user_attribution` and `product_events` definitions as a migrated database

#### Scenario: Deleting a user cascades

- **WHEN** a `users` row is deleted
- **THEN** the associated `user_attribution` row and all `product_events` rows for that user are deleted by the foreign key cascade

#### Scenario: Anonymous events are permitted

- **WHEN** a `product_events` row is inserted with a null `user_id`
- **THEN** the insert succeeds

### Requirement: Campaign cohort view storage

The schema SHALL define a read-only `v_campaign_cohort` view in a versioned migration, derived from `users`, `user_channel_tracking`, `user_comment_stars`, `comment_exports`, `user_attribution`, and `product_events`, without adding denormalized state.

#### Scenario: View is created by migration

- **WHEN** the versioned migration is applied
- **THEN** `v_campaign_cohort` exists and is queryable

#### Scenario: View is re-runnable

- **WHEN** the migration that defines the view is re-applied to an existing schema
- **THEN** it completes without error and the view definition is unchanged
