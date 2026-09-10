## ADDED Requirements

### Requirement: YouTube Quota Telemetry

The ingestion meter SHALL expose YouTube request, quota-unit, suppression, and discovery-source counters so operators can explain quota consumption and detect regressions to expensive paths.

#### Scenario: Requests and units by method

- **WHEN** the client issues a YouTube API request
- **THEN** `noumena_youtube_requests_total{method,status}` SHALL increment by one and `noumena_youtube_quota_units_total{method}` SHALL increment by the centralized unit cost

#### Scenario: Suppressed requests counted

- **WHEN** the daily budget or a per-job ceiling refuses a request
- **THEN** `noumena_youtube_requests_suppressed_total{reason}` SHALL increment with a closed-enum reason (`daily_budget`, `job_ceiling`) and no network call SHALL occur

#### Scenario: Discovery source counted

- **WHEN** a job discovers videos
- **THEN** `noumena_youtube_discovery_total{via}` SHALL increment with `playlist` or `search_fallback`, and fallback use SHALL also be logged with the resolution error

#### Scenario: Quota decisions logged

- **WHEN** the worker defers a channel for quota, resumes a video from a checkpoint, or changes a channel's adaptive poll interval
- **THEN** a structured log line SHALL record the channel, decision, and counters involved
