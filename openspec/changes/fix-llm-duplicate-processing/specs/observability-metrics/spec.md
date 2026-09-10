# Delta: observability-metrics

## MODIFIED Requirements

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
