# Observability

Noumena emits OpenTelemetry metrics, traces, and logs over OTLP/gRPC. The
instrumentation is gated by `OTEL_ENABLED` (default `false`) — when off,
the code path is a true no-op (zero allocations on the hot path, no
network connections, no log spam).

## Configuration

Four environment variables control the OTel stack:

| Variable | Default | Required when | Purpose |
|---|---|---|---|
| `OTEL_ENABLED` | `false` | — | Master switch. When `false`, the rest are ignored. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | (empty) | `OTEL_ENABLED=true` | OTLP/gRPC collector endpoint, e.g. `localhost:4317`. No URL scheme. |
| `OTEL_SERVICE_NAME` | `noumena-api` | `OTEL_ENABLED=true` | `service.name` resource attribute (sets the dashboard "service" facet). |
| `OTEL_RESOURCE_ATTRIBUTES` | (empty) | optional | Comma-separated `key=value` pairs added to every span/metric (e.g. `deployment.environment=staging,region=us-east-1`). |

Standard OpenTelemetry envs are also respected by the SDK directly. Of
note: `OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE` lets you
switch between cumulative (default) and delta temporality if your
backend prefers the latter (e.g. AWS CloudWatch, some Datadog
configurations).

### Rollback

If a metrics or tracing issue causes load on the API process, set
`OTEL_ENABLED=false` and restart. No code change is needed; the
binary's hot paths short-circuit on the global no-op MeterProvider /
TracerProvider before reaching any allocation, atomic, or syscall.

## Metric contract

All instruments live under one of these scopes (the value passed to
`otel.Meter("…")`):

- `noumena/api` — HTTP server middleware
- `noumena/ingestion` — comment-ingestion worker
- `noumena/llm` — classification client + failover
- `noumena/lifecycle` — deletion + export workers
- `noumena/billing` — Stripe webhook handler
- `noumena/email` — transactional pipeline + retry worker

### HTTP (`noumena/api`)

| Instrument | Type | Unit | Attributes |
|---|---|---|---|
| `noumena_http_requests_total` | Counter | `1` | `method`, `route`, `status` |
| `noumena_http_request_duration_seconds` | Histogram | `s` | `method`, `route`, `status` |

`route` is the chi route pattern (e.g. `/api/users/{id}`), not the raw
path — preserves cardinality.

### Ingestion (`noumena/ingestion`)

| Instrument | Type | Unit | Attributes |
|---|---|---|---|
| `noumena_comments_ingested_total` | Counter | `1` | `platform` |
| `noumena_ingestion_errors_total` | Counter | `1` | `platform`, `reason` |
| `noumena_ingestion_lock_contention_total` | Counter | `1` | `platform` |
| `noumena_fetch_duration_seconds` | Histogram | `s` | `platform`, `outcome` |
| `noumena_youtube_requests_total` | Counter | `1` | `platform`, `method`, `status` |
| `noumena_youtube_quota_units_total` | Counter | `1` | `platform`, `method` |
| `noumena_youtube_requests_suppressed_total` | Counter | `1` | `platform`, `reason` |
| `noumena_youtube_discovery_total` | Counter | `1` | `platform`, `via` |

`reason` enum: `rate_limited`, `token_invalid`, `unknown`,
`dead_letter`. `outcome` enum: `success`, `error`, `timeout`.

### LLM (`noumena/llm`)

| Instrument | Type | Unit | Attributes |
|---|---|---|---|
| `noumena_llm_requests_total` | Counter | `1` | `provider`, `model`, `outcome` |
| `noumena_llm_tokens_total` | Counter | `1` | `provider`, `model` |
| `noumena_llm_request_duration_seconds` | Histogram | `s` | `provider`, `model`, `outcome` |
| `noumena_llm_failover_switch_total` | Counter | `1` | `from_provider`, `to_provider` |

Custom histogram boundaries (seconds): `[0.05, 0.1, 0.25, 0.5, 1, 2.5,
5, 10, 30, 60, 120]` — wide enough for slow Anthropic responses and
short enough to surface OpenAI tail latency.

### Lifecycle (`noumena/lifecycle`)

| Instrument | Type | Unit | Attributes |
|---|---|---|---|
| `noumena_deletions_completed_total` | Counter | `1` | `outcome` |
| `noumena_deletion_chunk_rows` | Histogram | `1` | (none) |
| `noumena_exports_completed_total` | Counter | `1` | `outcome` |
| `noumena_export_bytes_total` | Counter | `By` | (none) |

`outcome` enum (deletion): `success`, `aborted_new_tracker`, `error`.
`outcome` enum (export): `success`, `error`.

### Billing (`noumena/billing`)

| Instrument | Type | Unit | Attributes |
|---|---|---|---|
| `noumena_stripe_webhook_total` | Counter | `1` | `event`, `outcome` |

`event` is the Stripe event type (e.g. `invoice.payment_succeeded`),
or empty when the body never parsed. `outcome` enum: `success`,
`signature_invalid`, `handler_error`, `ignored`, `idempotent_dedupe`.

### Email (`noumena/email`)

| Instrument | Type | Unit | Attributes |
|---|---|---|---|
| `noumena_emails_sent_total` | Counter | `1` | `template`, `outcome` |
| `noumena_email_retry_total` | Counter | `1` | `template` |

`outcome` enum: `success`, `error`, `idempotent_dedupe`.

## Closed-enum policy

The `reason`, `outcome`, and `event` attributes are closed enums
defined in `internal/telemetry/attrs.go`. Unknown values are coerced
to `error` (production) or panic (test builds via the `ValidOutcome`
helper). This keeps cardinality bounded: dashboards can pre-build
status panels without learning new label values from incoming
traffic.

## Tracing

Two scopes emit spans:

- `noumena-api` (mounted via `otelhttp.NewMiddleware`) — one server
  span per HTTP request with method/route/status attributes.
- `noumena/llm` — one span per `ClassifyBatch` call with
  provider/model/batch_size attributes; on error the span is marked
  `Error` with the underlying error message.

Span context propagates from inbound requests through the LLM call,
so a single request that classifies a batch shows up as a 2-level
trace.

## Operational notes

- **Cardinality**: `route` (HTTP), `template` (email), and `event`
  (billing) are unbounded in principle but bounded in practice by the
  source code's set of registered routes / templates / Stripe event
  types. There are no user-supplied attribute values on any
  instrument.
- **Rollout**: deploy with `OTEL_ENABLED=false` first, verify no
  startup errors or new log spam, then flip to `true` in staging and
  bake for 48 hours before promoting to production.
- **Shutdown**: `SIGTERM`/`SIGINT` triggers a bounded 5-second
  shutdown sequence that flushes buffered metric reads and trace
  spans before exit. If your collector is unreachable at shutdown,
  the buffer is dropped silently — there is no on-disk spool.
