## ADDED Requirements

### Requirement: Jev price card and hybrid cost accounting
The system SHALL include a price card for TypeSafe Jev (`jev-latest`) at $0.042 per million input tokens with output tokens billed at $0, and SHALL price a hybrid classification batch as the sum of its Jev leg and its generative extraction leg.

#### Scenario: Jev model priced from its card
- **GIVEN** token usage reported by the `typesafe` provider for model `jev-latest`
- **WHEN** cost is derived
- **THEN** the system SHALL apply the Jev card (`InputPerM=42_000`, `OutputPerM=0` micro-USD) rather than the provider default

#### Scenario: Unknown Jev model falls back to the typesafe default
- **GIVEN** a `typesafe` response whose model is not present in the price-card table
- **WHEN** cost is derived
- **THEN** the system SHALL use the `typesafe` provider default card

#### Scenario: Hybrid batch cost sums both legs
- **GIVEN** a hybrid batch that consumed input/output tokens on the Jev judge and on the generative extractor
- **WHEN** the batch cost is recorded
- **THEN** the system SHALL sum both legs' derived costs and attribute the total to the quota-decrementing user and the batch's channel

#### Scenario: Price version stays immutable
- **GIVEN** the Jev card is added in a new release
- **WHEN** historical cost rows are read
- **THEN** they SHALL retain the price version they were written with, and only batches after the change SHALL use the new version
