## ADDED Requirements

### Requirement: Missed digest catch-up

The system SHALL generate a weekly digest even when the scheduled trigger time was missed.

#### Scenario: Downtime across the scheduled time
- **GIVEN** application downtime spanning Monday 08:00 UTC
- **WHEN** the application restarts
- **THEN** the missed digest is generated from the available data

### Requirement: Deferred digest re-dispatch

The system SHALL retry digests marked `deferred` due to LLM failure instead of dropping them.

#### Scenario: Deferred digest
- **GIVEN** a digest marked `deferred` after LLM failures
- **WHEN** its retry schedule comes due
- **THEN** it is dispatched again, eventually generating the digest or reaching the retry cap
