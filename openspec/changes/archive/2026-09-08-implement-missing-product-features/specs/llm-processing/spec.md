## ADDED Requirements

### Requirement: LLM provider failover

The system SHALL support a configurable primary and fallback LLM provider, switching to the fallback after an error window.

#### Scenario: Primary provider fails
- **GIVEN** a configured primary LLM provider
- **WHEN** the primary produces N errors within a sliding window (default 5 in 60s)
- **THEN** the system switches to the fallback provider for the process lifetime

#### Scenario: No fallback configured
- **GIVEN** only a primary provider is configured
- **WHEN** the primary fails
- **THEN** the system continues to retry the primary and does not crash
