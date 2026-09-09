## ADDED Requirements

### Requirement: Sentiment scoring

The system SHALL assign a closed-set sentiment (negative/neutral/positive) to processed comments and expose an aggregate endpoint.

#### Scenario: Comment processed by LLM
- **GIVEN** a comment processed by the classification pipeline
- **WHEN** classification completes
- **THEN** the comment's sentiment is persisted with the comment

#### Scenario: Sentiment trend requested
- **GIVEN** a channel the user can access
- **WHEN** `GET /api/insights/sentiment` is called with a valid `days` value
- **THEN** per-day sentiment aggregates are returned for the requested window
