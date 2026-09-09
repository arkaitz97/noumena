## MODIFIED Requirements

### Requirement: SQLite connection pragmas

The system SHALL open SQLite with WAL journaling, foreign keys enabled, and a 5-second busy timeout, and SHALL verify these at startup.

#### Scenario: Application starts
- **GIVEN** a valid database path
- **WHEN** the application opens the database
- **THEN** `journal_mode` is `wal`, `foreign_keys` is `1`, and `busy_timeout` is at least 5000ms
- **AND** a startup mismatch is reported (error or fatal log)

### Requirement: Versioned schema migrations

The system SHALL apply schema changes through an ordered, versioned, idempotent migration runner that records applied versions.

#### Scenario: Fresh database
- **GIVEN** an empty database
- **WHEN** the application starts
- **THEN** all migrations apply in order and the `schema_migrations` ledger records each version

#### Scenario: Existing database on an older version
- **GIVEN** a database missing columns or constraints added by later migrations
- **WHEN** the application starts
- **THEN** only pending migrations apply, including ALTER TABLE steps, and existing data is preserved

#### Scenario: Re-run on up-to-date database
- **GIVEN** a database at the latest version
- **WHEN** the application starts
- **THEN** no migrations re-run and the database is unchanged

### Requirement: Deletion and export job column names

The system SHALL use `error_message` for job failure text and `export_url` for data-export download URLs, consistently in both schema and application code.

#### Scenario: Deletion job completes with failure
- **GIVEN** a deletion job that fails
- **WHEN** the worker records the error
- **THEN** the error text is stored in `error_message` and the job reads back successfully

#### Scenario: Data export completes
- **GIVEN** a data export job finishing
- **WHEN** the export URL is persisted and the worker reads the job back
- **THEN** the URL is read from `export_url` without a missing-column error

### Requirement: Subscription status values

The system SHALL accept `trialing` as a valid `subscription_status` value.

#### Scenario: Webhook sets trialing status
- **GIVEN** a Stripe event carrying a `trialing` status
- **WHEN** the webhook handler applies the update
- **THEN** the write succeeds without a CHECK-constraint violation

### Requirement: FTS5 index covers all comments

The system SHALL ensure the FTS index is rebuilt after a migration so pre-existing comments are searchable.

#### Scenario: Search after migration
- **GIVEN** comments ingested before the FTS table was created
- **WHEN** a user searches for text in those comments
- **THEN** matching pre-existing comments are returned
