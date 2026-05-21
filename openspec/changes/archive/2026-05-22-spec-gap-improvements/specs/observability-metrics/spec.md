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
The system SHALL emit metrics for LLM classification operations.

#### Scenario: LLM Request Metrics
- **WHEN** an LLM classification request completes
- **THEN** the system SHALL record `noumena_llm_requests_total{provider, model, status}` and `noumena_llm_request_duration_seconds{provider, model}` and `noumena_llm_tokens_total{provider, direction}` (direction=prompt|completion)

#### Scenario: Failover Event
- **WHEN** the system switches from primary to fallback provider
- **THEN** the system SHALL increment `noumena_llm_failover_total{from_provider, to_provider}` and emit a structured log event

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
