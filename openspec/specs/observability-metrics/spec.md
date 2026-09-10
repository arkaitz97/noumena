# Observability & Metrics Specification

## Purpose
Provides OpenTelemetry instrumentation across all subsystems for monitoring, alerting, and debugging. Zero performance impact when disabled via feature flag.

## Requirements

### Requirement: Feature-Gated Initialization
The system SHALL initialize OpenTelemetry only when OTEL_ENABLED=true.

#### Scenario: OTel Disabled
- **WHEN** the application starts with OTEL_ENABLED=false
- **THEN** the system SHALL use no-op providers for tracing and metrics, adding zero overhead

#### Scenario: OTel Enabled
- **WHEN** the application starts with OTEL_ENABLED=true and OTEL_EXPORTER_OTLP_ENDPOINT is set
- **THEN** the system SHALL initialize the OTel SDK, configure the OTLP exporter, and register all instrumentations

### Requirement: HTTP Middleware Instrumentation
The system SHALL record metrics for all HTTP requests.

#### Scenario: Request Metrics
- **WHEN** any HTTP request is processed
- **THEN** the system SHALL increment `noumena_http_requests_total{method, route, status}` and record `noumena_http_request_duration_seconds{method, route}` histogram

#### Scenario: Slow Request Logging
- **WHEN** a request takes longer than SLOW_REQUEST_THRESHOLD (default 1000ms)
- **THEN** the system SHALL log a structured warning with method, route, duration, and user_id (if authenticated)

### Requirement: Ingestion Metrics
The system SHALL emit metrics for the comment ingestion pipeline.

#### Scenario: Ingestion Counters
- **WHEN** comments are successfully ingested
- **THEN** the system SHALL increment `noumena_ingestion_comments_ingested_total{platform, channel_id}`

#### Scenario: Ingestion Errors
- **WHEN** an ingestion job fails
- **THEN** the system SHALL increment `noumena_ingestion_errors_total{error_type, channel_id}` where error_type is one of: api_error, rate_limit, token_expired, dead_letter

#### Scenario: Lock Contention
- **WHEN** a worker fails to acquire an advisory lock
- **THEN** the system SHALL increment `noumena_ingestion_lock_contention_total{channel_id}`

### Requirement: LLM Processing Metrics
The system SHALL emit metrics for LLM classification operations, including per-attempt counters that make duplicate-provider-call loops detectable.

#### Scenario: LLM Request Metrics
- **WHEN** an LLM classification request completes
- **THEN** the system SHALL record `noumena_llm_requests_total{provider, model, status}` and `noumena_llm_request_duration_seconds{provider, model}` and `noumena_llm_tokens_total{provider, direction}` (direction=prompt|completion)

#### Scenario: Failover Event
- **WHEN** the system switches from primary to fallback provider
- **THEN** the system SHALL increment `noumena_llm_failover_total{from_provider, to_provider}` and emit a structured log event

#### Scenario: Provider Call Attempts Counter
- **WHEN** any HTTP attempt against a provider begins (including in-retry attempts within one classification call)
- **THEN** the system SHALL increment `noumena_llm_provider_calls_total{provider, retryable}` (retryable=true for transport/429/5xx-triggered attempts), so the ratio of provider calls to distinct classification jobs is observable

#### Scenario: Job Outcome Counter
- **WHEN** a classification job reaches a terminal state for the attempt
- **THEN** the system SHALL increment `noumena_llm_job_attempts_total{provider, outcome}` with outcome in `completed|failed_transient|failed_terminal|parked`

#### Scenario: Parked Comments Gauge
- **WHEN** the processor samples queue depth on a tick
- **THEN** the system SHALL record `noumena_llm_parked_comments` — the count of unprocessed comments excluded from claiming because they reached the attempt cap

### Requirement: Billing Metrics
The system SHALL emit metrics for billing operations.

#### Scenario: Quota Usage
- **WHEN** a user's quota usage is checked
- **THEN** the system SHALL record `noumena_billing_quota_usage{plan, user_id}` as a gauge (0.0-1.0)

#### Scenario: Webhook Processing
- **WHEN** a Stripe webhook is processed
- **THEN** the system SHALL increment `noumena_billing_webhooks_total{event_type, status}` and record `noumena_billing_webhook_duration_seconds{event_type}`

### Requirement: Lifecycle Metrics
The system SHALL emit metrics for data lifecycle operations.

#### Scenario: Deletion Progress
- **WHEN** a deletion job progresses through stages
- **THEN** the system SHALL record `noumena_lifecycle_deletion_stage{stage}` gauge and increment `noumena_lifecycle_deletions_total{status}` on completion

#### Scenario: Export Metrics
- **WHEN** a data export completes
- **THEN** the system SHALL record `noumena_lifecycle_export_size_bytes{user_id}` and increment `noumena_lifecycle_exports_total{status}`

### Requirement: Health Check Endpoints
The system SHALL provide health and readiness endpoints.

#### Scenario: Health Check
- **WHEN** a request hits GET /health
- **THEN** the system SHALL return 200 with `{"status": "ok", "uptime_seconds": <n>}`

#### Scenario: Readiness Check
- **WHEN** a request hits GET /ready
- **THEN** the system SHALL verify database connectivity and return 200 with `{"status": "ready"}` or 503 with `{"status": "not_ready", "reason": "database_unavailable"}`

### Requirement: Structured Logging
The system SHALL use structured logging with sensitive field redaction.

#### Scenario: Log Format
- **WHEN** any log entry is written
- **THEN** the system SHALL output JSON with timestamp, level, message, and contextual fields (request_id, user_id if available)

#### Scenario: Email Redaction
- **WHEN** a log entry contains an email address
- **THEN** the system SHALL redact it to first_char + "***" + domain format

### Requirement: Metrics Export
The system SHALL expose metrics for scraping when OTel is enabled.

#### Scenario: Prometheus Endpoint
- **WHEN** a request hits GET /metrics and OTEL_ENABLED=true
- **THEN** the system SHALL return Prometheus-format metrics for all registered instruments

### Requirement: Graceful Shutdown Drain
The system SHALL stop background workers before flushing telemetry and closing the database.

#### Scenario: Shutdown Signal Received
- **GIVEN** a running process with active workers
- **WHEN** a shutdown signal is received
- **THEN** the system SHALL drain workers within a bounded window, then stop the HTTP server, flush telemetry, and close the database — in that order

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
