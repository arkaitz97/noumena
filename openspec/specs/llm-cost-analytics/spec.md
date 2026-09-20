# LLM Cost Analytics Specification

## Purpose
Tracks LLM token usage and derived cost per batch, attributed to users, channels, and billing periods using versioned price-card snapshots, and rolls costs up to daily billing-period totals for reporting and display.

## Requirements

### Requirement: Per-batch token accounting

The LLM processor SHALL record input and output token usage for every classification and digest call, attributed to the user whose quota is decremented, the channel, and the current billing period. The price card (provider + model + per-M-token rates) used for conversion SHALL be snapshotted per batch so historical costs are immutable.

#### Scenario: Batch processed
- **WHEN** a classification batch completes
- **THEN** the system persists token usage and derived cost (micro-cents) for the user, channel, and period

#### Scenario: Price card changes
- **WHEN** provider prices change in a later release
- **THEN** batches processed after the change use the new price and earlier rows keep the old snapshot

### Requirement: Cost rollup

The system SHALL roll up per-batch costs into the user's billing period (daily granularity) and SHALL include digest generation costs in the same rollup.

#### Scenario: Multiple batches in one period
- **WHEN** several batches and a digest run complete in one billing period
- **THEN** the period rollup sums all classification and digest costs for the user
