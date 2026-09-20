## ADDED Requirements

### Requirement: Asset persistence during video discovery

The ingestion worker SHALL persist a content asset for each video it discovers while walking a channel's uploads playlist, upserting idempotently by `(platform_channel_id, platform_asset_id)` and recording the video id used to associate comments. Persisting assets SHALL NOT change the existing comment deduplication or quota semantics.

#### Scenario: Asset persisted on first walk

- **WHEN** the worker discovers a video not yet stored for the channel
- **THEN** it inserts the asset with metadata in the same run, without an extra API call beyond the playlist walk already performed

#### Scenario: Re-walk is idempotent

- **WHEN** the worker discovers a video already stored for the channel
- **THEN** the upsert updates metadata without creating a duplicate and without failing

#### Scenario: Comment association video id captured

- **WHEN** the worker ingests comments for a video
- **THEN** the video id needed to associate those comments with the asset is available to the system

### Requirement: Asset metric refresh pass

The ingestion subsystem SHALL run a metrics-refresh pass that updates asset engagement counts using batched statistics requests routed through the existing quota-budget admission, and SHALL defer the pass when the budget is exhausted or a `quotaExceeded` stop is armed.

#### Scenario: Refresh routed through budget admission

- **WHEN** the metrics-refresh pass issues a statistics request
- **THEN** the request is metered against the daily quota budget and suppressed if the budget is spent

#### Scenario: Refresh defers on hard stop

- **GIVEN** a server `quotaExceeded` stop is armed for the day
- **WHEN** the refresh pass runs
- **THEN** it makes no requests and resumes on the next day

### Requirement: Sponsored-signal detection in ingestion

The ingestion subsystem SHALL evaluate an asset's title and description for configured sponsored-content markers when the asset is persisted or its metadata changes, and SHALL store the matched markers and an `is_sponsored` flag on the asset.

#### Scenario: Detection on persist

- **WHEN** an asset is inserted or its title/description is updated
- **THEN** sponsored markers are evaluated and stored

#### Scenario: Detection is deterministic

- **WHEN** the same metadata is evaluated twice
- **THEN** the same `is_sponsored` value and marker set result
