## ADDED Requirements

### Requirement: Global administrator column

The schema SHALL include a non-null boolean administrator flag on the users table, defaulting to false, created by a versioned migration and mirrored in the baseline schema for fresh installs.

#### Scenario: Migration adds the administrator flag

- **WHEN** the versioned migration is applied to a database at the prior version
- **THEN** the users table has a non-null administrator column defaulting to false
- **AND** the migration is recorded in `schema_migrations`

#### Scenario: Existing users default to non-admin

- **WHEN** the migration runs against a database with existing users
- **THEN** all existing users have administrator access disabled
