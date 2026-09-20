## ADDED Requirements

### Requirement: Campaign dashboard

The dashboard SHALL provide a campaigns view listing workspace campaigns and a campaign detail view showing the campaign's assets, the audience summary (volume, sentiment trend, category mix, questions, objections, topics, content ideas, toxicity share), the per-creator comparison, and an export control. The views SHALL be hidden when the campaign API is disabled.

#### Scenario: Campaign list

- **WHEN** a member opens the campaigns view
- **THEN** the view lists the workspace's campaigns with brand name, status, and window

#### Scenario: Create a campaign

- **WHEN** an admin creates a campaign and attaches assets
- **THEN** the campaign detail view shows the attached assets

#### Scenario: Summary renders observed signals

- **WHEN** a campaign's summary loads
- **THEN** the view shows comment volume, sentiment trend, category mix, top questions, objections, top topics, content ideas, and toxicity share

#### Scenario: Creator comparison renders

- **WHEN** the creator comparison loads
- **THEN** each creator row shows the same components, with low-sample rows labeled

#### Scenario: Unassociated comments disclosed

- **WHEN** the summary reports unassociated comments
- **THEN** the view displays that count so coverage is visible

#### Scenario: Export from the dashboard

- **WHEN** a member with at least the `analyst` role clicks export
- **THEN** the report file is downloaded
