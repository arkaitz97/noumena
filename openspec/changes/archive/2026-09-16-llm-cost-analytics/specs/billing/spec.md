## ADDED Requirements

### Requirement: Cost in usage and status responses

`GET /api/billing/usage` SHALL include per-period `llm_cost_usd` alongside comment counts, and `GET /api/billing/status` SHALL include month-to-date `llm_cost_mtd_usd`. Cost figures SHALL be additive fields; existing fields SHALL be unchanged. Cost SHALL be shown for all plans including Free; overage invoicing logic SHALL remain comment-count based.

#### Scenario: Usage response with cost
- **WHEN** a user fetches billing usage
- **THEN** each period row includes `llm_cost_usd` derived from the cost rollup

#### Scenario: Status response with cost
- **WHEN** a user fetches billing status
- **THEN** the response includes `llm_cost_mtd_usd` for the current billing period