## ADDED Requirements

### Requirement: SQLite backup and restore

The system SHALL provide a documented, scripted backup and restore procedure for the SQLite database.

#### Scenario: Daily backup
- **GIVEN** a running application
- **WHEN** the backup script runs
- **THEN** a consistent backup file is produced (checkpointed) and older backups are rotated

#### Scenario: Restore
- **GIVEN** a valid backup file
- **WHEN** an operator follows the restore runbook
- **THEN** the application returns to the backed-up state

### Requirement: CI quality gates

The system SHALL run vet, full tests, formatting, frontend build and tests, and dependency audits in CI.

#### Scenario: Unformatted code pushed
- **GIVEN** a pull request with Go files failing `gofmt`
- **WHEN** CI runs
- **THEN** the gate fails

#### Scenario: Frontend regression
- **GIVEN** a pull request that breaks a frontend test
- **WHEN** CI runs the frontend job
- **THEN** the job fails

### Requirement: Production container health

The system SHALL run the API container with a healthcheck, a non-root user, and a writable export directory.

#### Scenario: Container starts
- **GIVEN** the API image
- **WHEN** the container starts
- **THEN** the health endpoint is reachable, the process runs as a non-root user, and the export directory is writable
