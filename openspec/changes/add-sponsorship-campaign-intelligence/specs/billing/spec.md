## ADDED Requirements

### Requirement: Campaign entitlements

The plan catalog SHALL expose a campaign entitlement per plan, and campaign creation SHALL be rejected with an upgrade error when the workspace has reached its entitlement. Campaign comment processing SHALL count against the workspace's existing comment quota; overage pricing SHALL remain comment-count based.

#### Scenario: Entitlement available

- **GIVEN** a workspace whose plan includes at least one campaign and whose campaign count is below the entitlement
- **WHEN** an admin creates a campaign
- **THEN** the system allows creation

#### Scenario: Entitlement reached

- **GIVEN** a workspace at its campaign entitlement
- **WHEN** an admin attempts to create another campaign
- **THEN** the system returns 402 with a message directing the user to upgrade

#### Scenario: Campaign comments count against quota

- **WHEN** comments associated with campaign assets are processed
- **THEN** they count against the workspace's comment quota exactly as other comments do, with no separate campaign meter

#### Scenario: Overage remains comment based

- **GIVEN** a billable plan with comment overage in a period
- **WHEN** the overage-invoicing job runs
- **THEN** the invoice is derived from comment counts, unchanged by the campaign entitlement
