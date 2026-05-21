# Project Worldview: Noumena

## Context
Noumena is a B2C/B2B SaaS platform that connects to content creators' YouTube channels, ingests comments via platform APIs, and uses LLMs to classify, filter, and synthesize comments into actionable growth insights. It removes comment noise and delivers structured intelligence: top questions, audience pain points, viral content ideas, collaboration hooks, and product validation signals.

## Stack

| Layer | Technology |
|-------|-----------|
| Backend API | Go 1.25, chi/v5 router |
| Database | PostgreSQL 16 (pgx/v5 driver) |
| Frontend | Vue 3 (Composition API) + Vite + TypeScript + Tailwind CSS v4 |
| State Management | Pinia + Vue Router |
| Payments | Stripe (Checkout + Billing + Webhooks, stripe-go v76) |
| LLM Processing | OpenAI GPT-4o (primary), Anthropic Claude 3.5 Sonnet (fallback), OpenCode (configurable) |
| Auth | Custom JWT (golang-jwt/v5) + Google OAuth 2.0 PKCE |
| Email | SMTP pipeline with idempotent retry (Resend legacy retained) |
| Job Queue | PostgreSQL-backed (FOR UPDATE SKIP LOCKED + advisory locks) |
| Observability | OpenTelemetry (OTLP gRPC exporter) |
| Testing | testcontainers-go (PostgreSQL), testify |

## Architecture

### Design Patterns
- **Shared Channel Model**: A single real-world platform channel maps to one `platform_channels` record, shared across multiple Noumena users. Each user has a `user_channel_tracking` row with per-user OAuth tokens and preferences. Comments are ingested once and shared.
- **Job Queue Pattern**: PostgreSQL-based queue using `FOR UPDATE SKIP LOCKED` for claim semantics and `pg_try_advisory_lock` for per-channel mutual exclusion. No external queue infrastructure (Redis, SQS).
- **Worker-Based Processing**: Ingestion, LLM processing, deletion, export, and email retry each run as independent goroutines driven by tick loops.
- **Feature Flags**: Runtime toggles via environment variables (`INGESTION_WORKER_ENABLED`, `LLM_PROCESSOR_ENABLED`, `INSIGHTS_API_ENABLED`, `BILLING_ENABLED`, `EMAIL_ENABLED`, `LIFECYCLE_WORKERS_ENABLED`, `ACCOUNT_LIFECYCLE_API_ENABLED`, `OTEL_ENABLED`).
- **Failover Pattern**: LLM provider failover uses a sliding error window (default: 5 errors in 60s) to switch from primary to fallback permanently for the process lifetime.

### Module Layout
```
internal/
  api/          — HTTP handlers (chi router), middleware, auth guards
  auth/         — JWT, password hashing, refresh tokens, encryption
  billing/      — Stripe client, plan catalog, quota enforcement, webhook dispatch
  config/       — Environment variable loader, process-wide defaults
  db/           — PostgreSQL query layer (PostgresQuery implements all DB interfaces)
  dbtest/       — Test container helpers
  email/        — SMTP pipeline, templates, retry worker, signing
  export/       — CSV/JSON export encoder, streaming
  ingestion/    — YouTube API client, scheduler, worker, advisory locks
  lifecycle/    — Deletion worker, export worker, cleanup, OAuth revocation
  llm/          — LLM clients (OpenAI, Anthropic, OpenCode), processor, failover, digest
  oauth/        — Google SSO (PKCE + JWKS), YouTube OAuth
  telemetry/    — OpenTelemetry attributes, counters, span helpers
frontend/       — Vue 3 SPA (dashboard, auth, onboarding)
tools/          — CLI utilities (cancel_deletion, backfill_stripe_subscription_id)
```

### Key Data Flow
1. **User connects YouTube channel** → OAuth exchange → upsert `platform_channels` + create `user_channel_tracking` → enqueue backfill job
2. **Scheduler** polls every 30s → finds channels due → enqueues ingestion jobs (`poll` or `backfill`)
3. **Ingestion Worker** claims jobs → acquires per-channel advisory lock → fetches videos + comment threads from YouTube API → batch-inserts comments (deduped by `platform_comment_id`)
4. **LLM Processor** ticks every 10s → claims unprocessed comments in batches of 50 → classifies via LLM → persists category/sentiment/topic → increments quota
5. **Insights API** serves filtered comment feeds, sentiment aggregates, toxicity shields, trends, star/unstar, export lifecycle
6. **Weekly Digest** cron runs Monday 08:00 UTC → generates per-user digest via LLM → stores + optionally emails
7. **Billing** processes Stripe webhooks → updates subscription state → enforces channel/comment quotas
8. **Lifecycle** handles account deletion (GDPR export → channel detach → chunked comment delete → hard delete) and data exports (ZIP bundle → signed URL)
