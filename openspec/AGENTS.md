# AGENTS.md — OpenSpec Agent Instructions for Noumena

## Repository Overview
Noumena is a Go backend + Vue 3 frontend SaaS platform for AI-powered comment intelligence. Storage is SQLite via `modernc.org/sqlite` (pure-Go driver; `internal/db/schema.sql`), job queues are SQLite-backed (no Redis/SQS), router is chi, modules are feature-flagged. Workers (ingestion, LLM, digest, email, lifecycle, export) run as goroutines inside the single `api` process.

## Active Change Set (audit fixes)
A 2026-09-05 audit produced 12 openspec changes under `openspec/changes/` (`fix-*`, `implement-missing-product-features`), each with `proposal.md`/`design.md`/`tasks.md` + spec deltas. **Apply order and phases: `openspec/change-roadmap.md`.** Before editing a domain, check whether an open change already covers the area — if so, update/implement that change instead of editing the spec or code directly. Known-broken areas (deletion/export workers, email retry, workspace role enforcement) are deliberately left unfixed until their changes are applied; do not treat current behavior as intended.

## Spec Map
When working on a change, identify which domain spec(s) it affects:

| Domain | Spec File | Key Packages |
|--------|-----------|-------------|
| Authentication | `specs/auth/spec.md` | `internal/api/auth_handlers.go`, `internal/auth/`, `internal/oauth/`, `internal/api/middleware.go` |
| Channel Management | `specs/channel-management/spec.md` | `internal/api/channel_handlers.go`, `internal/oauth/youtube.go` |
| Comment Ingestion | `specs/ingestion/spec.md` | `internal/ingestion/` (worker.go, scheduler.go, lock.go, youtube.go) |
| LLM Processing | `specs/llm-processing/spec.md` | `internal/llm/` (processor.go, opencode.go, digest_worker.go, digest_llm.go, digest_aggregates.go) |
| Insights API | `specs/insights-api/spec.md` | `internal/api/insights_handlers.go`, `internal/api/comment_handlers.go`, `internal/export/` |
| Billing | `specs/billing/spec.md` | `internal/api/billing_handlers.go`, `internal/billing/` (plans.go, quota.go, client.go) |
| Data Lifecycle | `specs/data-lifecycle/spec.md` | `internal/api/account_handlers.go`, `internal/lifecycle/` (deletion_worker.go, export_worker.go) |
| Email Pipeline | `specs/email-pipeline/spec.md` | `internal/email/` (pipeline.go, retry_worker.go, smtp.go, templates.go) |
| Weekly Digest | `specs/weekly-digest/spec.md` | `internal/api/digest_handlers.go`, `internal/llm/digest_*.go` |
| Frontend Dashboard | `specs/frontend-dashboard/spec.md` | `frontend/` (Vue 3 SPA) |
| Actionable Insights Aggregation | `specs/actionable-insights-aggregation/spec.md` | `internal/api/insights_handlers.go`, `internal/db/comments_feed_db.go` |
| Actionable Insights Extraction | `specs/actionable-insights-extraction/spec.md` | `internal/llm/prompts.go`, `internal/llm/processor.go` |

## Domain-Specific Guidance

### Auth
- JWT access tokens: 15min TTL, HS256 signed. Refresh tokens: 30-day TTL, SHA-256 hashed in DB, stored as HttpOnly cookies.
- Token versioning: `users.token_version` is incremented on logout-all and password-reset. Access tokens embed the version; middleware validates it.
- Password policy: >=8 chars, >=1 uppercase, >=1 digit. Bcrypt cost >=12 (configurable via BCRYPT_COST).
- Google SSO uses PKCE (S256) and verifies ID tokens against Google's JWKS. Two flows: redirect-based and popup-based JSON.
- Email verification tokens and password reset tokens use the same `GenerateRefreshToken()` primitive (32-byte random, hex-encoded) but are stored separately with distinct TTLs.

### Channel Management
- Shared channel model: `platform_channels` (one per real channel) + `user_channel_tracking` (one per user-channel pair).
- Token priority: 0 = primary ingestion token, higher = fallback. Computed as MAX+1 on new tracker; race condition handled with single retry.
- Backfill days: only 30, 60, 90, 180 are valid. Default 90.
- Plan limits enforced AFTER CSRF validation but BEFORE burning Google auth code.

### Ingestion
- Job queue: SQLite-backed. Claim via `UPDATE … WHERE status='pending' … RETURNING` (the archived PostgreSQL `FOR UPDATE SKIP LOCKED` wording is obsolete).
- Per-channel mutual exclusion (`AcquireChannelLock`, `internal/ingestion/lock.go`) is a documented **no-op** — SQLite serializes writes. Concurrency safety relies on claim atomicity + `ON CONFLICT DO NOTHING` on comment insert; a real advisory lock is deferred (see `fix-workers-recovery-shutdown`).
- Retry budget: 5 attempts. Dead-lettered jobs park the channel for 1 hour.
- Token refresh: worker always forces a refresh via oauth2.TokenSource before API calls.
- Only YouTube is implemented; non-YouTube platforms are marked completed without API calls.

### LLM Processing
- Batch size: 50 comments per channel per tick. Tick interval: 10 seconds.
- Confidence threshold: 0.7 default. Below threshold → category downgraded to "other".
- OpenCode is the sole LLM provider for both classification and digest generation.
- LLM calls happen outside DB transactions. Persist results in a separate Tx2.
- Overage tracking: only for Creator and Studio plans. Free is below-quota by definition; Agency uses custom pricing.
- `*opencodeClient` implements both `LLMClient` (ClassifyBatch) and digest generation (GenerateDigestRaw).
- Retry logic: 3 retries with 1s/2s/4s backoff on 429/5xx, honoring Retry-After header.

### Insights API
- Feature-gated by `INSIGHTS_API_ENABLED`. Returns 503 when off.
- Feed queries run in parallel (feed + count) via goroutines. Timeout degradation: count=null, has_more from len(rows)==limit.
- Export: sync if under threshold (50K), async otherwise. Files stored on disk with signed URL download (no auth middleware on download route).
- All access control enforced at SQL layer via mandatory access-control joins.

### Billing
- Plan catalog in `internal/billing/plans.go` is the single source of truth. Never hard-code limits elsewhere.
- Webhook processing: idempotent via `processed_stripe_events` table. Each event dispatched in a transaction (when pool available).
- Quota cron: runs every 24h at 08:00 UTC. Resets free-tier quotas, expires past-due grace (72h default).
- Quota middleware: returns 402 when comments_used >= quota. Applied to ingestion-related routes.

### Data Lifecycle
- Deletion is NOT a hard delete on request. Sets `deletion_requested_at`, enqueues async job. Reversible within `DELETION_REVERSIBLE_HOURS` (default 48h) while status='pending'.
- Deletion stages: export (best-effort, 12h timeout) → per-channel detach (OAuth revoke, chunked comment delete) → final wipe (digests, stars, auth tokens, hard delete user).
- **Known-broken (audit):** the deletion endpoint + worker and the export worker currently error on real SQLite (Postgres-ism `NOW()`/`$1` + `last_error`/`storage_path` column drift; DSN pragmas WAL/FK/busy_timeout are not applied). Do not debug as-is — the fixes land via `fix-sqlite-driver-migrations` → `fix-data-lifecycle-gdpr` → `fix-export-pipeline`.
- Failed deletion jobs are NOT auto-retried. They stay in 'failed' state for human triage.
- Export ZIP cap: 500MB. Partial exports include a README.md and are considered successful (not failure).
- Signed URLs: HMAC-based signature with EXPORT_SIGNED_URL_SECRET. Download route has NO auth middleware.

### Email Pipeline
- `EMAIL_ENABLED=false` is a documented kill-switch. Send is a logged no-op, no rows written.
- Idempotency: two-layer — pre-insert SELECT + unique constraint on idempotency_key.
- Retry worker: 3-attempt cap, 60-second tick. **Known-broken (audit):** the retry path never calls `Sender.Send` (only bumps `attempt_count`), so transient SMTP failures lose mail silently — fixed by `fix-email-delivery`.
- Email address redaction in logs: first char + "***" + domain.

### Weekly Digest
- Cron: Monday 08:00 UTC. Minimum 10 comments required.
- LLM failures: 3 retries over 2 hours → deferred status.
- Paused channels excluded from digest computation.
- Email opt-in controlled via PATCH /api/user/preferences.

## Conventions
- All handlers use `writeJSON` / `writeError` from `auth_handlers.go`.
- DB interfaces are narrow (e.g., `WorkerDB`, `SchedulerDB`, `RefreshTokenStore`). `*db.SQLiteQuery` (db.go:243) satisfies the `DBQuery` interface; some stale comments still say `PostgresQuery` — see `fix-docs-ci-ops`.
- Feature flags use environment variables. Default values in `config.Default`.
- Secrets: `JWT_SECRET` (≥32 bytes) and `ENCRYPTION_KEY` are mandatory — do not add or rely on hardcoded fallbacks (planned enforcement in `fix-auth-critical-security` + `fix-crypto-http-hardening`).
- Migrations: currently CREATE-IF-NOT-EXISTS only (`ApplyMigrations`). Versioned `ALTER TABLE` migrations (with `schema_migrations` ledger + SQLite `_pragma` DSN: WAL/FK/busy_timeout) land via `fix-sqlite-driver-migrations` — follow that design once merged; until then, schema changes must be idempotent CREATE IF NOT EXISTS.
- Tests use the `internal/dbtest` harness (in-memory SQLite, real `ApplyMigrations` path) plus MockQuery-style interfaces for unit tests. No testcontainers/PostgreSQL.
- Workspace role enforcement and shared-channel grants are **not yet wired** (middleware unmounted, `tracking.workspace_id` never written) — pending `fix-workspace-authz-api`. Do not assume data routes are role-scoped.
- OpenTelemetry instrumentation is zero-cost when `OTEL_ENABLED=false` (no-op providers).

## When Adding a New Domain
1. Create `openspec/specs/{domain}/spec.md` following the RFC 2119 + GIVEN/WHEN/THEN format.
2. Add the domain to the Spec Map table above.
3. Add domain-specific guidance to the Domain-Specific Guidance section.
4. Update `openspec/project.md` if the architecture or stack changes.
