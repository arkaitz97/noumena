# Actionable Insights Aggregation Specification

## Purpose

Defines how actionable insights are aggregated, queried, and surfaced to creators across channels and time windows.

## Requirements

### Requirement: Insights Listing Endpoint
The system SHALL provide an endpoint to list actionable insights with filtering and pagination.

#### Scenario: List All Insights
- **GIVEN** an authenticated user with processed comments containing actionable_insights
- **WHEN** the user calls GET /api/insights/actionable-insights with no filters
- **THEN** the system SHALL return up to 20 insights sorted by confidence DESC, with insight type, description, source comment text, channel name, and timestamp

#### Scenario: Filter by Insight Type
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/insights/actionable-insights?insight_type=video_idea
- **THEN** the system SHALL return only insights of type "video_idea"

#### Scenario: Filter by Multiple Insight Types
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/insights/actionable-insights?insight_type=video_idea,editing_tip
- **THEN** the system SHALL return insights of either type

#### Scenario: Filter by Channel
- **GIVEN** an authenticated user tracking multiple channels
- **WHEN** the user calls GET /api/insights/actionable-insights?channel_id={id}
- **THEN** the system SHALL return insights scoped to that channel. If the user does not track the channel, return 403.

#### Scenario: Filter by Time Window
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/insights/actionable-insights?days=7
- **THEN** the system SHALL return insights from comments processed within the last 7 days

### Requirement: Top Insights Aggregation
The system SHALL provide aggregated top insights grouped by type.

#### Scenario: Get Top Insights by Type
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/insights/actionable-insights/top?days=30
- **THEN** the system SHALL return the top 10 most frequent insight types with count, and for each type the top 3 most confident insight descriptions

#### Scenario: Top Insights for Specific Channel
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/insights/actionable-insights/top?channel_id={id}&days=30
- **THEN** the system SHALL return top insights scoped to that channel

### Requirement: Insights Frequency Distribution
The system SHALL provide the distribution of insight types over a time window.

#### Scenario: Get Insight Type Distribution
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/insights/actionable-insights/distribution?days=30
- **THEN** the system SHALL return a count of each insight type found in the window, sorted by count DESC

### Requirement: Materialized View Refresh
The system SHALL maintain a materialized view for insight aggregation queries.

#### Scenario: Scheduled Refresh
- **GIVEN** the job scheduler is running
- **WHEN** 15 minutes have elapsed since the last refresh
- **THEN** the system SHALL refresh the `mv_actionable_insights_agg` materialized view

#### Scenario: Manual Refresh
- **GIVEN** an authenticated admin user
- **WHEN** the user calls POST /api/insights/actionable-insights/refresh
- **THEN** the system SHALL trigger an immediate refresh of the materialized view and return 200

### Requirement: Insights Feature Gate
The system SHALL gate all actionable insights endpoints behind the existing INSIGHTS_API_ENABLED feature flag.

#### Scenario: API Disabled
- **GIVEN** INSIGHTS_API_ENABLED=false
- **WHEN** any request hits an actionable insights route
- **THEN** the system SHALL return 503 with "insights_api_disabled"
