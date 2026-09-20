## ADDED Requirements

### Requirement: Campaign management

The system SHALL let a workspace admin create, read, update, and delete a sponsorship campaign with a name, brand name, status (`draft`, `active`, `completed`), and a run window. Campaigns SHALL be workspace-scoped and SHALL NOT be readable or mutable by other workspaces.

#### Scenario: Create a campaign

- **WHEN** a workspace admin calls `POST /api/campaigns` with a name, brand name, and window
- **THEN** the system creates the campaign with status `draft` and returns 201

#### Scenario: Window validation

- **WHEN** a campaign is created or updated with an end date before its start date
- **THEN** the system returns 400 and persists nothing

#### Scenario: Foreign workspace access

- **WHEN** a member requests or mutates a campaign owned by another workspace
- **THEN** the system returns 403 and discloses nothing about the campaign

#### Scenario: Viewer cannot mutate

- **GIVEN** a workspace member with the `viewer` role
- **WHEN** they attempt to create or update a campaign
- **THEN** the system returns 403

### Requirement: Campaign asset linking

The system SHALL let a workspace attach and detach content assets to a campaign, and SHALL only allow attaching assets from channels the workspace is authorized for.

#### Scenario: Attach an authorized asset

- **WHEN** an admin attaches an asset whose channel the workspace can access
- **THEN** the campaign-asset link is created and the asset appears in the campaign

#### Scenario: Detach an asset

- **WHEN** an admin detaches an asset
- **THEN** the link is removed and the asset no longer contributes to campaign aggregates

#### Scenario: Reject a foreign asset

- **WHEN** an admin attaches an asset whose channel the workspace cannot access
- **THEN** the system returns 403 and creates no link

#### Scenario: Duplicate attach is idempotent

- **WHEN** an already-attached asset is attached again
- **THEN** the system does not create a duplicate link

### Requirement: Campaign audience aggregates

The system SHALL compute, over comments associated with the campaign's assets and within the campaign window, a summary containing comment volume, unique authors, sentiment distribution and a per-day sentiment trend, category mix, top questions, constructive-criticism objections, top topics, top content ideas, and the toxicity/spam share. The summary SHALL return the exact window used and the count of associated comments that could not be linked to an asset.

#### Scenario: Summary computed

- **WHEN** a member requests `GET /api/campaigns/{id}/summary`
- **THEN** the system returns the aggregate contract and the window used

#### Scenario: Objections are constructive criticism

- **WHEN** the summary is computed
- **THEN** the objections field contains comments in category `constructive_criticism`

#### Scenario: Toxicity share is an observed signal

- **WHEN** the summary is computed
- **THEN** the toxicity share is the proportion of toxic and spam comments, presented as an observed signal and not as a suitability verdict

#### Scenario: Unassociated comments are disclosed

- **GIVEN** some comments in the window cannot be linked to a campaign asset
- **WHEN** the summary is returned
- **THEN** it includes the count of unassociated comments

#### Scenario: Small sample is labeled

- **GIVEN** a campaign whose comment volume is below the configured minimum
- **WHEN** the summary is returned
- **THEN** it is flagged as a small sample so shares are not over-interpreted

### Requirement: Cross-creator comparison

The system SHALL return a per-creator (channel) comparison for a campaign, using identical component definitions across creators: engagement totals from assets, comment volume, sentiment composition, question count, objection count, and toxicity share. Rows below the configured minimum comment count SHALL be labeled rather than silently ranked.

#### Scenario: Compare creators

- **WHEN** a member requests `GET /api/campaigns/{id}/creators`
- **THEN** the system returns one row per creator with the same components for each

#### Scenario: Suppression label

- **GIVEN** a creator with fewer than the minimum comment count
- **WHEN** the comparison is returned
- **THEN** that row is labeled with its low sample size

### Requirement: Campaign report export

The system SHALL export a campaign report as CSV or JSON containing the campaign metadata, the aggregate summary, the per-asset rows, and the per-creator comparison.

#### Scenario: Export a campaign report

- **WHEN** a member calls `POST /api/campaigns/{id}/export` with a format
- **THEN** the system streams a file with the metadata, summary, per-asset, and per-creator sections and returns the download metadata

#### Scenario: Export respects workspace scope

- **WHEN** a report is generated
- **THEN** it contains only data from channels the workspace is authorized for

### Requirement: Campaign feature gate

The campaign API SHALL be gated by `CAMPAIGN_API_ENABLED`. When disabled, the routes SHALL return 503 and expose no campaign data.

#### Scenario: API disabled

- **WHEN** `CAMPAIGN_API_ENABLED=false` and any `/api/campaigns*` request is made
- **THEN** the system returns 503 with a `campaign_disabled` code
