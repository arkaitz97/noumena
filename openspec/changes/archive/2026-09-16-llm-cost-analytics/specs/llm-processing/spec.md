## ADDED Requirements

### Requirement: Batch cost persistence

The LLM processing subsystem SHALL persist token usage and derived cost for every processed batch and digest, following the cost-attribution rules in the `llm-cost-analytics` capability.

#### Scenario: Classification batch with tokens
- **WHEN** a batch is classified successfully
- **THEN** the system records input/output tokens and cost for the batch in the same transaction that persists classification results

#### Scenario: Digest generation
- **WHEN** a weekly digest is generated for a user
- **THEN** the system records the digest's token usage and cost against the user's billing period