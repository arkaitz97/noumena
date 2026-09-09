## ADDED Requirements

### Requirement: Job crash recovery

The system SHALL recover ingestion and LLM jobs left in an in-progress state by a crash.

#### Scenario: Crash mid-ingestion
- **GIVEN** an ingestion job stuck in `running`
- **WHEN** the reaper runs at startup or on its tick
- **THEN** the job is reset to `pending` and the channel resumes ingesting

#### Scenario: Stuck LLM processing
- **GIVEN** an LLM processing job stuck in `processing`
- **WHEN** the reaper runs
- **THEN** the job is reset and no duplicate LLM job is enqueued for the same comments

### Requirement: Expiry-aware token selection

The system SHALL prefer unexpired ingestion tokens when more than one is available.

#### Scenario: Primary token expired
- **GIVEN** a primary token past its expiry and an unexpired fallback
- **WHEN** an ingestion token is selected
- **THEN** the unexpired fallback is used
