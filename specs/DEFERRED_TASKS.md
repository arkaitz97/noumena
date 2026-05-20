# Deferred & Blocked Tasks

This document captures every unchecked task across the 10 OpenSpec changes generated from the SPECS folder, along with the reason each was not completed during implementation.

**Generated:** 2026-05-02
**Total unchecked tasks:** ~98 across 10 changes
**Build/test status at time of writing:** `go build ./...`, `go vet ./...`, `go test ./...` all green; frontend `npm run build` green.

---

## Summary by Change

| Change | Unchecked | Status |
|---|---|---|
| `core-auth` | 1 | ✅ 73/74 |
| `channel-management` | 19 | ✅ 59/78 |
| `comment-ingestion` | 21 | ✅ 61/82 |
| `llm-processing` | 4 | ✅ 54/58 |
| `insights-api` | 30 | ✅ 84/114 |
| `weekly-digest` | 5 | ✅ 52/57 |
| `billing` | 0 | ✅ Complete |
| `data-lifecycle` | 21 | ✅ 109/115 |
| `frontend-auth` | 3 | ✅ 58/61 |
| `frontend-dashboard` | 11 | ✅ 63/74 |

## Dominant Blocker Themes

1. **Live Postgres / DB harness required** — most integration tests across `comment-ingestion`, `llm-processing`, `insights-api`, `weekly-digest`, `data-lifecycle`, and `channel-management`. The repo has `docker-compose.test.yml` but no fixture-driven test harness wired into CI; existing `db_test.go` skips when `TEST_DATABASE_URL` is unset.
2. **Staging / production environment required** — rollout & smoke-test steps in `llm-processing`, `insights-api`, `weekly-digest`, `frontend-auth`, `frontend-dashboard`.
3. **Ops/runbook documentation deferred** — `insights-api §14`, `comment-ingestion §12`, `weekly-digest §8.5`, `channel-management §13`. No central runbook location agreed.
4. **Backend feature gap for popup SSO** — RESOLVED by `auth-google-popup-endpoints`: backend now exposes `GET /auth/google/auth-url`, `POST /auth/google/callback`, and `GET /auth/me`. Frontend migration from redirect fallback to true popup remains as a follow-up (tracked under `frontend-auth §3.3 / §9.3`).
5. **Missing frontend test runner** — `frontend/package.json` has only `dev`/`build`/`preview`; no Vitest/Jest configured.
6. ~~**Components inlined into views** — `frontend-dashboard §4.1–4.10`~~ **Resolved** by `frontend-component-extraction`: 10 reusable SFCs (`CommentCard`, `FilterBar`, `SentimentChart`, `SentimentSparkline`, `ToxicityShield`, `WeeklyDigestCard`, `PlanComparison`, `UsageBar`, `ChannelCard`, `ConfirmModal`) live at `frontend/src/components/*.vue` and are consumed by every dashboard/settings view. Each component has a colocated `__tests__/<Component>.spec.ts` covering prop typing, default render, and emit assertions; the suite executes under Vitest.

---

## core-auth — 1 unchecked

| # | Task | Reason |
|---|------|--------|
| 11.13 | Integration tests for Google SSO callback (state mismatch, bad ID token, new user, returning user, link-existing-account confirm/decline) | **BLOCKED** — requires real Google JWKS or a JWKS test harness with mock RSA keypair + injectable issuer. State-mismatch and bad-ID-token error paths are unit-testable in a follow-up. |

---

## channel-management — 19 unchecked

### §11 Encryption (2 tasks, no inline reason)
| # | Task | Reason |
|---|------|--------|
| 11.2 | Unit tests verifying persisted `access_token`/`refresh_token` columns are ciphertext and decrypt back to plaintext | Skipped during apply — encryption uses existing `auth.Encrypt`/`auth.Decrypt`. Requires DB or fake-pool harness; treated as DB-test work. |
| 11.3 | Audit log statements to ensure tokens are never logged | Manual audit task; deferred. |

### §12 Testing (13 tasks)
All require the dockerised Postgres test harness or browser-driven OAuth flow. Pure-Go correctness for OAuth client / DB methods is exercised via existing unit tests on the YouTube client + scheduler.

| # | Task |
|---|------|
| 12.1 | Unit tests for `BuildAuthURL` (asserts `access_type=offline`, `prompt=consent`, scope, state) |
| 12.2 | Unit tests for `oauth_states` insert/consume (success, expired, wrong user, replay) |
| 12.3 | Unit tests for `UpsertPlatformChannel` (insert path, conflict path, returns correct `inserted` flag) |
| 12.4 | Unit tests for `GetIngestionToken` and `PromoteNextIngestionToken` (multi-tracker promotion, primary pause/remove, concurrent promotion) |
| 12.5 | Integration tests for `GET /api/channels/connect/youtube` (200 / 401 / 403) |
| 12.6 | Integration tests for `POST /api/channels/exchange/youtube` (success new + reused, 400 invalid_state / invalid_code / no_refresh_token, 409 already-tracked) |
| 12.7 | Integration tests for `GET /api/channels` (correct fields, user-scoped, empty list) |
| 12.8 | Integration tests for pause / resume / delete (shared-data preservation, schedule-deletion on last tracker) |
| 12.9 | Integration test for `PATCH /api/channels/{id}/backfill-window` (200 valid, 400 invalid, no re-backfill) |
| 12.10 | Integration test for `GET /api/channels/{id}/status` (`is_primary_token_holder`, `comment_count`) |
| 12.11 | Integration test for daily expiry job (notify-once, reconnect clears flag) |
| 12.12 | Integration test for purge job (purge on zero active, abort if tracker reappears in grace window) |
| 12.13 | Concurrency test: two trackers race `token_priority = 0` insert (partial unique index rejects one, app retries) |

### §13 Documentation (4 tasks) — ✅ Resolved by `ops-docs-and-runbooks`
Documented under `docs/` (see `docs/README.md`).

| # | Task |
|---|------|
| 13.1 | Document `/api/channels/*` endpoints in API reference — ✅ `docs/api/channels.md` |
| 13.2 | Document YouTube OAuth client setup in ops runbook — ✅ `docs/ops/runbooks/youtube-oauth-setup.md` |
| 13.3 | Document channel-deletion grace window for support staff — ✅ `docs/ops/runbooks/channel-grace-window.md` |
| 13.4 | Document `backfill_days` semantics (no re-backfill on change) for end-user help — ✅ `docs/help/backfill-window.md` |

---

## comment-ingestion — 21 unchecked

### Live-DB blockers (12 tasks)
| # | Task | Reason |
|---|------|--------|
| 1.8 | Run migration locally against a populated dev DB | No live dev DB; migration is idempotent, runs at startup via `ApplyMigrations`. |
| 2.12 | Unit tests for each new DB method using a test Postgres | No fixture harness; covered indirectly via 6.14 / 11.x integration logic. |
| 3.4 | Unit test: two goroutines acquire same channel's advisory lock | Requires live Postgres advisory-lock semantics. |
| 3.5 | Unit test: closing connection releases the lock automatically | Requires live Postgres. |
| 7.3 | E2E OAuth-connect → backfill-enqueued test | Requires live Postgres + browser OAuth; covered logically by 5.5 + 7.1. |
| 11.1 | Walk every `comment-ingestion-worker` scenario | PARTIAL — happy/rate-limit/token-invalid/dead-letter covered; multi-channel concurrency, real advisory-lock, backfill resume blocked. |
| 11.2 | Walk every `ingestion-scheduling` scenario | PARTIAL — classification + idempotency covered; live-load scheduling blocked. |
| 11.3 | Walk every `youtube-comment-fetching` scenario | DONE via `youtube_client_test.go` (left unchecked because not run in staging). |
| 11.4 | Walk every `comment-deduplication` scenario | PARTIAL — SQL in place; concurrent-insert race blocked. |
| 11.5 | Walk every `ingestion-retry-and-backoff` scenario | PARTIAL — routing + parking covered; live wall-clock verification blocked. |
| 11.6 | Walk every `comment-backfill` scenario | PARTIAL — implemented; full multi-tick backfill resume blocked. |
| 11.7 | Multi-process advisory-lock + dedup-leak test | Requires two backend processes against same live Postgres. |

### Worker scope (1 task)
| # | Task | Reason |
|---|------|--------|
| 5.4 | Honor `INGESTION_MAX_CONCURRENCY` via worker-side semaphore | Moved into Section 6 (worker dispatch loop). |

### Docs / metrics (2 tasks)
| # | Task | Reason |
|---|------|--------|
| 10.2 | Document defaults in `.env.example` and README | `.env.example` portion owned by `backend-schema-and-env-fixes`; README/docs side ✅ resolved by `ops-docs-and-runbooks` (`docs/ops/runbooks/ingestion-deployment.md`, `docs/README.md`). |
| 10.5 | Metrics counters | ✅ Resolved by `observability` capability (otel-instrumentation change): ingestion meter `noumena/ingestion` records `noumena_comments_ingested_total`, `noumena_ingestion_errors_total`, `noumena_ingestion_lock_contention_total`, `noumena_fetch_duration_seconds`. |

### Deployment (6 tasks)
All require production/staging environment and ops handoff:

| # | Task |
|---|------|
| 12.1 | Deploy with `INGESTION_WORKER_ENABLED=false` |
| 12.2 | Enable worker on one instance |
| 12.3 | Scale worker to all instances |
| 12.4 | Run initial-backfill enqueue script |
| 12.5 | Configure dead-letter alerting destination in production |
| 12.6 | Add ops runbook entry — ✅ Resolved by `ops-docs-and-runbooks` (`docs/ops/runbooks/ingestion-deployment.md`) |

---

## llm-processing — 4 unchecked

All deferred to ops/staging:

| # | Task | Reason |
|---|------|--------|
| 9.3 | Apply migration in staging; deploy worker with `LLM_PROCESSOR_ENABLED=false` | Requires staging environment access. |
| 9.4 | Smoke test in staging | Requires real `OPENAI_API_KEY` + populated `comments` table. |
| 9.5 | Failover dry-run in staging | Requires real `ANTHROPIC_API_KEY`. |
| 9.6 | Production rollout | Production rollout step; deferred to ops. |

---

## insights-api — 30 unchecked

### Live-DB / harness required (10 tasks)
| # | Task |
|---|------|
| 1.8 | Migration verification with `\d+ comment_exports` and `\di comments` |
| 4.18 | Unit tests for every DB method using real Postgres |
| 5.8 | Handler tests for feed |
| 6.5 | Tests for star/unstar handlers |
| 7.7 | Tests for insights aggregates |
| 8.8 | Tests for export lifecycle |
| 9.4 | Tests for latest digest handler |
| 11.10 | Worker tests (claim concurrency, retry, file write, cleanup, startup recovery) |
| 13.1–13.10 | E2E suite: cross-tenant 403, full filter combos, star flow, sync 1K export, async 60K export, expiry 410, sentiment percentages, toxicity-shield denominator, foreign `channel_id` 403, perf at 1M rows |

### Documentation (6 tasks) — ✅ 14.1, 14.3, 14.5 resolved by `ops-docs-and-runbooks`
| # | Task |
|---|------|
| 14.1 | Runbook: `EXPORT_DIR` disk usage monitoring — ✅ `docs/ops/runbooks/export-disk-monitoring.md` |
| 14.2 | Runbook: query timeout behavior |
| 14.3 | Document 50K sync/async threshold — ✅ `docs/api/comments.md` |
| 14.4 | Document high-signal default categories + two-layer toxic opt-in |
| 14.5 | Document CSV column schema — ✅ `docs/help/csv-export-schema.md` (and `docs/api/comments.md`) |
| 14.6 | Add metrics emission TODO list — ✅ Resolved by `observability` capability (otel-instrumentation change): HTTP middleware records `noumena_http_requests_total` + `noumena_http_request_duration_seconds`; LLM meter records request/token/duration/failover; lifecycle + billing + email subsystems each have their own scope. |

### Rollout (6 tasks, environment-driven)
| # | Task |
|---|------|
| 15.1 | Deploy with flags off; verify migrations applied |
| 15.2 | Enable `INSIGHTS_API_ENABLED=true` in staging; smoke tests |
| 15.3 | Enable `EXPORT_WORKER_ENABLED=true` in staging |
| 15.4 | `EXPLAIN ANALYZE` representative production-shape feed queries |
| 15.5 | Enable both flags in production; monitor 48h |
| 15.6 | Confirm no regression; archive change |

> Gating wiring already in place: routes return 503 when `INSIGHTS_API_ENABLED=false`, export worker only starts when `EXPORT_WORKER_ENABLED=true`, migration application is idempotent.

---

## weekly-digest — 5 unchecked

| # | Task | Reason |
|---|------|--------|
| 2.11 | Unit tests covering each query method against test DB | No fixture-driven test DB harness; query methods exercised indirectly via worker + handler tests. |
| 8.2 | Manual E2E staging test (>10 processed comments) | Requires staging + OpenAI key. |
| 8.3 | Manual staging test (<10 comments → skipped) | Logic covered by `TestRunUserDigest_SkipBelowThreshold`. |
| 8.4 | Manual staging test (LLM always fails → deferred) | Logic covered by retry-exhaustion unit test. |
| 8.5 | Update internal README / runbook section — ✅ Resolved by `ops-docs-and-runbooks` (`docs/ops/runbooks/weekly-digest-delivery.md`) |

---

## billing — 0 unchecked ✅

All 64 tasks complete. **One operational caveat documented inline in `specs/07-billing.md`:** live Stripe test-mode E2E (real Checkout flow → signed `checkout.session.completed` against the live webhook receiver) is not in CI and must be run manually during release validation.

---

## data-lifecycle — 21 unchecked

All require live Postgres because the SUT relies on real `FOR UPDATE` locks, `RETURNING` clauses, transactional retries, and multi-table cascades that the in-memory `MockQuery` cannot faithfully reproduce.

### Email pipeline (1 task)
| # | Task |
|---|------|
| 14.3 | Unit tests for `email.Send` idempotency under concurrent inserts (needs real unique-violation, not a mock map) |

### Deletion DB methods (1 task)
| # | Task |
|---|------|
| 14.5 | Unit tests for `GetActiveTrackerCountForUpdate`, `DeleteCommentsByChannelChunked` convergence + abort-on-new-tracker |

### Deletion-worker integration (8 scenarios)
| # | Scenario |
|---|----------|
| 14.6.1 | User with one channel and no other trackers |
| 14.6.2 | User with one channel and 2 other trackers |
| 14.6.3 | Primary token holder triggers `PromoteNextIngestionToken` before revocation |
| 14.6.4 | OAuth revocation failure does not block local deletion |
| 14.6.5 | Worker honors `DELETION_REVERSIBLE_HOURS` grace window |
| 14.6.6 | New tracker arriving mid-purge aborts shared deletion |
| 14.6.7 | Worker restart after partial chunked-delete resumes correctly |
| 14.6.8 | 1M-comment chunked deletion completes in expected number of chunks |

### Export worker (1 task)
| # | Task |
|---|------|
| 14.7 | Integration tests for export worker (ZIP contents, signed URL, DB writes) |

### Account / lifecycle handlers (5 tasks)
| # | Task |
|---|------|
| 14.8.2 | `GET /api/user/export` enqueue + concurrent existing |
| 14.8.3 | `GET /api/user/export/{job_id}/status` cross-user 404 |
| 14.8.4 | `GET /api/user/export/{job_id}/download` 403 expired/invalid sig, 200 ZIP stream |
| 14.8.5 | `GET /api/user/me` profile + PATCH email verification, 409 collision, 429 rate limit |
| 14.8.6 | Auth middleware returns 403 `account_scheduled_for_deletion` after deletion request |

### Trigger-wiring integration tests (5 tasks)
| # | Task |
|---|------|
| 14.9.1 | Login lockout email fires exactly once per lock event |
| 14.9.2 | `invoice.payment_failed` emits `payment_failed`; redelivery does not duplicate |
| 14.9.3 | Grace-expiry job emits `payment_grace_final`; recovery before expiry emits nothing |
| 14.9.4 | Trial reminder fires once per `trial_ends_at` cycle |
| 14.9.5 | Weekly digest delivery honors `digest_email_opt_in`; idempotent on second run |

> Note: Handler-level smoke for 14.8.1 (`DELETE /api/user/account` 401 + email mismatch) is implemented in `internal/api/account_handlers_test.go`. The cancel_deletion CLI smoke (14.10) is in `tools/cancel_deletion/main_test.go`.

> ~~**Additional caveat:** Stripe subscription cancellation in `DeleteAccount` requires a `users.stripe_subscription_id` column (currently only `stripe_customer_id`). Documented inline; non-fatal.~~ **Resolved** by `backend-schema-and-env-fixes` — the column was added, webhooks now persist it, and `DeleteAccount` calls `billing.CancelSubscription` directly.

---

## frontend-auth — 3 unchecked

> §5.6 is now executable: `frontend-vitest-runner` lands a Vitest harness and the refresh-coalescing test at `frontend/src/services/__tests__/api.refresh.test.ts`. The §5.6 checkbox in `openspec/changes/frontend-auth/tasks.md` will be flipped to `[x]` in a follow-up — it is intentionally **not** flipped as part of `frontend-vitest-runner`.

| # | Task | Reason |
|---|------|--------|
| 5.6 | Integration test verifying concurrent 401s share one refresh call | **BLOCKED** — no test runner wired into `frontend/package.json` (only `dev`/`build`/`preview`); manual test plan provided inline. |
| 12.1 | Manually walk testing checklist against staging | **BLOCKED** — no staging environment; cannot exercise live Google OAuth or SMTP-delivered verification emails. |
| 12.3 | Confirm `npm run dev` E2E flow: register → verify → onboarding → dashboard | **BLOCKED** — requires running backend + SMTP. Code paths align with backend handlers reviewed. |

### Backend gaps documented inline on checked tasks
- **3.3** — Backend popup endpoints `GET /auth/google/auth-url` and `POST /auth/google/callback` are now implemented (`auth-google-popup-endpoints` change). Frontend popup migration of `loginWithGoogleRedirect` → true popup is the remaining work.
- **9.3** — Same as 3.3: backend is unblocked; frontend popup migration is the remaining follow-up.
- **`/auth/me`** — Now provided by the dedicated `GET /auth/me` endpoint (`auth-google-popup-endpoints` change), independent of `ACCOUNT_LIFECYCLE_API_ENABLED` and the verified-email middleware. Frontend can switch off the `/api/user/me` fallback for auth-store hydration.

---

## frontend-dashboard — 11 unchecked

### §4 Reusable components (10 tasks, no extracted files)

These were **inlined directly into views** rather than extracted into separate `.vue` component files. Functionality works (build green, main chunk 19.47 kB gzip), but reusability and testability suffer. Future cleanup task: extract these to `frontend/src/components/` for proper reuse.

| # | Component | Used in |
|---|-----------|---------|
| 4.1 | `CommentCard.vue` | `InsightsFeed.vue`, `SavedCommentsView.vue` |
| 4.2 | `FilterBar.vue` | `InsightsFeed.vue`, `SavedCommentsView.vue` |
| 4.3 | `SentimentChart.vue` | `InsightsFeed.vue` |
| 4.4 | `SentimentSparkline.vue` | `InsightsFeed.vue` (sidebar) |
| 4.5 | `ToxicityShield.vue` | `InsightsFeed.vue` |
| 4.6 | `WeeklyDigestCard.vue` | `InsightsFeed.vue` (sidebar) |
| 4.7 | `PlanComparison.vue` | `BillingView.vue` |
| 4.8 | `UsageBar.vue` | `BillingView.vue` |
| 4.9 | `ChannelCard.vue` | `ChannelsView.vue` |
| 4.10 | `ConfirmModal.vue` | `ChannelsView.vue`, `AccountView.vue` |

### §10 Polish
| # | Task | Reason |
|---|------|--------|
| 10.8 | Manually walk testing checklist in `specs/10-frontend-dashboard.md` against staging | **BLOCKED** — requires live staging with backend deployed, real Stripe in test mode, and a YouTube test channel. All happy-path wiring is implemented and unit-verifiable. |

---

## Recommended Follow-up Workstreams

Group the deferred work into roughly four follow-up PRs:

### PR 1 — DB integration test harness (unblocks ~70 tasks)
- Wire `testcontainers-go` (or `docker-compose.test.yml` already present) into Go test suite.
- Add fixture loader and cleanup helpers.
- Implement DB-backed tests under: `channel-management §12`, `comment-ingestion §2.12 / 3.4 / 3.5 / 7.3 / 11.x`, `llm-processing §2.6` (already checked but noted), `insights-api §4.18 / 5.8 / 6.5 / 7.7 / 8.8 / 9.4 / 11.10 / 13.x`, `weekly-digest §2.11`, `data-lifecycle §14.3 / 14.5 / 14.6.x / 14.7 / 14.8.x / 14.9.x`.

### PR 2 — Frontend test runner (unblocks 1 task + enables future tests)
- Add Vitest to `frontend/package.json`.
- Implement `frontend-auth §5.6` refresh-coalescing test.
- Add component tests as components are extracted.

### PR 3 — Component extraction (unblocks frontend-dashboard §4.1–4.10) — RESOLVED
- ~~Extract inlined markup from `InsightsFeed.vue`, `SavedCommentsView.vue`, `ChannelsView.vue`, `BillingView.vue`, `AccountView.vue` into 10 reusable SFCs under `frontend/src/components/`.~~ Resolved by `frontend-component-extraction`: all 10 components extracted, views consume them exclusively, colocated component tests in place.

### PR 4 — Backend SSO popup endpoints (unblocks `frontend-auth §3.3 / §9.3`)
- Add `GET /auth/google/auth-url` returning `{ auth_url, state }`.
- Add `POST /auth/google/callback` accepting `{ code, state }`.
- Migrate frontend from redirect fallback to true popup flow.

### PR 5 — Documentation & runbooks
- Stand up an `internal/docs/runbooks/` or `docs/ops/` directory.
- Author runbooks for: insights-api `EXPORT_DIR` monitoring, comment-ingestion deployment, weekly-digest delivery, channel-management OAuth setup, data-lifecycle deletion/export support flows.
- Author API reference for `/api/channels/*`, `/api/insights/*`, `/api/comments/*`, `/api/billing/*`, `/api/user/*`.

### PR 6 — Staging/production rollout
- Apply all migrations in staging (idempotent; runs at startup).
- Enable feature flags one-by-one with 24–48h soak windows: `LLM_PROCESSOR_ENABLED`, `INSIGHTS_API_ENABLED`, `EXPORT_WORKER_ENABLED`, `INGESTION_WORKER_ENABLED`, `DIGEST_WORKER_ENABLED`, `LIFECYCLE_WORKERS_ENABLED`, `EMAIL_ENABLED`, `BILLING_ENABLED`.
- Run `EXPLAIN ANALYZE` on production-shape queries.
- Manual E2E walkthroughs (`frontend-auth §12`, `frontend-dashboard §10.8`, `weekly-digest §8.2–8.4`, `billing` Stripe test-mode flow, `data-lifecycle` deletion + GDPR export).

---

## Operational Caveats Worth Re-stating

1. **`STRICT_CONFIG=1`** must be set in production. Without it, `JWT_SECRET` falls back to an insecure dev value and other required env vars use weak defaults.
2. ~~**`users.stripe_subscription_id` column** does not exist; full Stripe subscription cancellation in `DeleteAccount` is best-effort via `stripe_customer_id`. Add the column + migration as a small follow-up.~~ **Resolved** by `backend-schema-and-env-fixes`: column added (with partial unique index), webhooks persist it, and `DeleteAccount` cancels the subscription via the Stripe SDK directly.
3. **Geo lookup for lockout email region** is empty; adding a GeoIP DB is out-of-scope.
4. ~~**No `.env.example`** in repo — all defaults live in `internal/config/config.go`.~~ **Resolved** by `backend-schema-and-env-fixes`: `.env.example` now lists every variable read by `config.Load`, grouped by feature area with REQUIRED-vs-optional annotations.
5. **No `README.md`** at repo root before this work — added during `data-lifecycle` apply with env table, rollback semantics, and runbooks.
6. **Top-level `specs/07-billing.md`** carries the billing "Operations" section (env vars, Stripe dashboard setup, BILLING_ENABLED rollout/rollback, past_due runbook) since no central docs location exists.
