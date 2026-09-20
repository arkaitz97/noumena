## ADDED Requirements

### Requirement: Cost display in billing views

The billing page SHALL show a month-to-date LLM cost figure in the plan card and a per-period cost column in the usage history table, using the fields from `GET /api/billing/status` and `GET /api/billing/usage`. When cost fields are absent (older backend), the UI SHALL render the existing view without the cost column.

#### Scenario: Usage table with cost column
- **WHEN** a user opens the billing page with cost fields present
- **THEN** the usage history table shows a cost column and the plan card shows month-to-date spend

#### Scenario: Backend without cost fields
- **WHEN** the usage response lacks cost fields
- **THEN** the table renders without the cost column and no error is shown