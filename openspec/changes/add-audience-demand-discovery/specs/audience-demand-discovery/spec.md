## ADDED Requirements

### Requirement: Demand watch definition

The system SHALL let a workspace create, read, update, pause, and delete a named demand watch that describes a product, category, or competitor by a set of terms and an optional default lookback window. A watch SHALL be workspace-scoped and SHALL only be readable or mutable by members of its workspace.

#### Scenario: Create a watch

- **WHEN** a workspace member calls `POST /api/demand/watches` with a name, at least one term, and an optional lookback
- **THEN** the system creates the watch with `status = active`, stores its terms, and returns 201 with the watch id and terms

#### Scenario: Reject a watch without terms

- **WHEN** the request carries an empty term list or a term longer than the configured maximum
- **THEN** the system returns 400 and creates no watch

#### Scenario: Foreign workspace access

- **WHEN** a member requests or mutates a watch owned by another workspace
- **THEN** the system returns 403 and discloses nothing about the watch

#### Scenario: Pause and resume a watch

- **WHEN** a member sets a watch's status to `paused`
- **THEN** discovery for that watch stops returning results until it is set back to `active`

### Requirement: Creator-opt-in discovery scope

The system SHALL return discovery results only for channels that have at least one active tracking row with discovery consent `opted_in = true`. The system SHALL NOT return comments or aggregates from any channel without such consent, regardless of workspace grants or channel ownership.

#### Scenario: Opted-in channel appears

- **GIVEN** a channel with an active tracking row whose creator opted in
- **WHEN** a demand watch matches that channel's comments
- **THEN** the channel appears in the ranked shortlist

#### Scenario: Non-opted-in channel is excluded

- **GIVEN** a channel whose comments match the watch terms but whose creator has not opted in
- **WHEN** the shortlist is requested
- **THEN** the channel does not appear and none of its comments are returned

#### Scenario: Revoking consent removes the channel

- **WHEN** the last opted-in tracking row for a channel sets consent to false
- **THEN** subsequent discovery requests exclude that channel and its prior evidence is no longer served

### Requirement: Ranked creator shortlist

The system SHALL return a ranked list of opted-in creators for a watch, ranked by a composite of transparent components: demand intensity, engagement quality, and brand safety. Each result SHALL include the component values, the matching high-signal comment count, and the watch window used.

#### Scenario: Ranked results with components

- **WHEN** a member calls `GET /api/demand/watches/{id}/creators` with a `days` window
- **THEN** the system returns creators ordered by the composite score, each with demand intensity, engagement quality, and brand safety components

#### Scenario: High-signal weighting

- **GIVEN** two channels with the same number of matching comments, one dominated by `question` and `content_idea` comments and the other by `praise`
- **WHEN** the shortlist is computed
- **THEN** the channel dominated by questions and content ideas ranks higher on demand intensity

#### Scenario: Coverage indicator

- **WHEN** the shortlist is returned
- **THEN** the response includes an indicator of how many opted-in channels were searched, so the caller can judge coverage

#### Scenario: Empty corpus

- **GIVEN** no opted-in channel matches the watch terms
- **WHEN** the shortlist is requested
- **THEN** the system returns 200 with an empty list and the coverage indicator, not an error

### Requirement: Evidence view

The system SHALL return the individual matching comments that justify a creator's rank, including category, sentiment, topic, and posted-at, for opted-in channels only. The system SHALL deduplicate near-identical comment bodies so repeated boilerplate does not inflate evidence.

#### Scenario: Evidence for a creator

- **WHEN** a member calls `GET /api/demand/watches/{id}/evidence?channel_id=...`
- **THEN** the system returns the matching comments for that channel within the window, newest first, up to a limit

#### Scenario: Evidence for a non-opted-in channel

- **WHEN** evidence is requested for a channel without consent
- **THEN** the system returns 404 and no comment bodies

#### Scenario: Near-duplicate collapse

- **GIVEN** a channel whose matching results include many near-identical comments
- **WHEN** evidence is returned
- **THEN** near-identical bodies are collapsed so the evidence set reflects distinct signals

### Requirement: Shortlist export

The system SHALL allow a workspace to export a watch's ranked shortlist as CSV or JSON, containing the same columns as the on-screen result and respecting the opt-in scope.

#### Scenario: Export a shortlist

- **WHEN** a member calls `POST /api/demand/watches/{id}/export` with a format
- **THEN** the system streams a file with the ranked creators and their components, and returns the download metadata

#### Scenario: Export excludes non-opted-in channels

- **WHEN** a shortlist export is generated
- **THEN** it contains no row derived from a non-opted-in channel

### Requirement: Demand discovery feature gate

The demand-discovery API SHALL be gated by `DEMAND_DISCOVERY_API_ENABLED`. When disabled, the routes SHALL return 503 and SHALL expose no watch or discovery data.

#### Scenario: API disabled

- **WHEN** `DEMAND_DISCOVERY_API_ENABLED=false` and any `/api/demand/*` request is made
- **THEN** the system returns 503 with a `demand_discovery_disabled` code
