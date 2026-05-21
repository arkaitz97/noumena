## MODIFIED Requirements

### Requirement: Batch Comment Classification
The system SHALL process unprocessed comments in batches through an LLM provider, extracting category, topic, confidence, and actionable insights.

#### Scenario: Successful Batch Processing
- **GIVEN** unprocessed comments in the database
- **WHEN** the processor ticks (default every 10 seconds)
- **THEN** the system SHALL claim up to 50 comments per channel (FOR UPDATE SKIP LOCKED), send them to the LLM provider, apply confidence threshold (default 0.7), persist classification results (category, topic, confidence, actionable_insights), increment the primary token holder's quota usage, and mark the job completed

#### Scenario: Confidence Threshold Downgrade
- **GIVEN** an LLM result with confidence below 0.7
- **WHEN** the processor applies the confidence threshold
- **THEN** the system SHALL downgrade the category to "other"

#### Scenario: No Primary Tracker
- **GIVEN** a batch of comments for a channel with no active primary tracker
- **WHEN** the processor attempts to resolve the primary token holder
- **THEN** the system SHALL skip the batch without writing any results

#### Scenario: Paused User Batch Skip
- **GIVEN** a batch where the primary token holder's subscription_status is 'paused'
- **WHEN** the processor checks billing guard
- **THEN** the system SHALL skip the entire batch without processing

#### Scenario: Quota Exceeded Batch Skip
- **GIVEN** a batch where the primary token holder has used their full comment quota
- **WHEN** the processor checks billing guard
- **THEN** the system SHALL skip the entire batch without processing

## REMOVED Requirements

### Requirement: LLM Provider Failover
**Reason**: Not affected by this change; failover behavior remains unchanged but is documented separately for clarity.
**Migration**: No migration needed; failover logic in failover.go is untouched.

### Requirement: Multi-Provider Support
**Reason**: Not affected by this change; provider support remains unchanged.
**Migration**: No migration needed.

### Requirement: Overage Tracking
**Reason**: Not affected by this change; overage tracking remains unchanged.
**Migration**: No migration needed.

### Requirement: Graceful Shutdown
**Reason**: Not affected by this change; graceful shutdown behavior remains unchanged.
**Migration**: No migration needed.
