## MODIFIED Requirements

### Requirement: Data-export job recovery

The system SHALL recover data-export jobs that were interrupted, without orphaning files.

#### Scenario: Crash mid-export
- **GIVEN** a data-export job stuck in `exporting`
- **WHEN** the application restarts or the reaper tick runs
- **THEN** the job is reset to a retryable state and any partial ZIP for a non-completed job is removed

#### Scenario: Mark-completion failure
- **GIVEN** a ZIP written but its completion row update fails
- **WHEN** the failure is handled
- **THEN** the orphaned file is removed and the job returns to a retryable state

### Requirement: Exports do not block writers

The system SHALL stream exports without holding the main write path.

#### Scenario: Export while ingestion writes
- **GIVEN** an ongoing export
- **WHEN** ingestion or quota writes occur concurrently
- **THEN** the writes proceed without unbounded blocking
