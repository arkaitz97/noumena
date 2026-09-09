## ADDED Requirements

### Requirement: Graceful shutdown drain

The system SHALL stop background workers before flushing telemetry and closing the database.

#### Scenario: Shutdown signal received
- **GIVEN** a running process with active workers
- **WHEN** a shutdown signal is received
- **THEN** workers drain within a bounded window, then the HTTP server stops, telemetry flushes, and the database closes — in that order
