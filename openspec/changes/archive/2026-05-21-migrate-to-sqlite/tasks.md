## 1. Dependency and Configuration Changes

- [x] 1.1 Replace `github.com/jackc/pgx/v5` with `modernc.org/sqlite` and `github.com/google/uuid` in `go.mod`
- [x] 1.2 Remove `testcontainers-go/modules/postgres` and `testcontainers-go` dependencies from `go.mod`
- [x] 1.3 Update `.env.example` to replace `DATABASE_URL` with `DATABASE_PATH` (default `./noumena.db`)
- [x] 1.4 Add SQLite PRAGMA configuration (WAL mode, busy_timeout) to connection setup

## 2. Schema Rewrite

- [x] 2.1 Rewrite `internal/db/schema.sql` from PostgreSQL to SQLite dialect
- [x] 2.2 Convert all UUID columns to TEXT with application-side generation
- [x] 2.3 Convert JSONB columns to TEXT with JSON type affinity
- [x] 2.4 Convert `UUID[]` column (`llm_processing_jobs.comment_ids`) to TEXT storing JSON array
- [x] 2.5 Convert `TIMESTAMPTZ` columns to TEXT (ISO 8601 format)
- [x] 2.6 Convert `NUMERIC(4,2)` columns to REAL
- [x] 2.7 Replace `gen_random_uuid()` defaults (UUIDs generated in Go instead)
- [x] 2.8 Translate CHECK constraints to SQLite syntax
- [x] 2.9 Translate partial indexes to SQLite `WHERE` clause syntax
- [x] 2.10 Create FTS5 virtual table for comment full-text search with sync triggers
- [x] 2.11 Remove materialized view `mv_actionable_insights_agg` and `refresh_actionable_insights_view()` function
- [x] 2.12 Remove `ingestion_lock_id` column and advisory lock infrastructure
- [x] 2.13 Remove `xmax`-based insert detection from upsert queries
- [x] 2.14 Replace `FILTER (WHERE ...)` aggregates with `SUM(CASE WHEN ...)` patterns
- [x] 2.15 Ensure schema is idempotent (CREATE TABLE IF NOT EXISTS, safe re-runs)

## 3. Database Layer Rewrite

- [x] 3.1 Rewrite `internal/db/db.go` — replace `pgxpool` with `database/sql`, update connection logic, update `DBQuery` interface
- [x] 3.2 Rewrite `internal/db/models.go` — update Go struct types for TEXT UUIDs, REAL numerics, JSON string fields
- [x] 3.3 Rewrite `internal/db/comments.go` — update all comment CRUD queries for SQLite syntax
- [x] 3.4 Rewrite `internal/db/comments_search.go` — replace `to_tsvector` with FTS5 queries, update recursive CTE thread traversal
- [x] 3.5 Rewrite `internal/db/comments_feed_db.go` — update feed queries, replace `FILTER (WHERE)` with `CASE`, update insight aggregation
- [x] 3.6 Rewrite `internal/db/channels.go` — update channel listing and sync queries
- [x] 3.7 Rewrite `internal/db/channel_mgmt.go` — update OAuth states, platform channel upsert, tracking CRUD
- [x] 3.8 Rewrite `internal/db/tracking.go` — update ingestion token retrieval and priority promotion
- [x] 3.9 Rewrite `internal/db/ingestion.go` — replace `FOR UPDATE SKIP LOCKED` with atomic UPDATE-based claiming, remove advisory locks
- [x] 3.10 Rewrite `internal/db/digests.go` — update digest CRUD queries
- [x] 3.11 Rewrite `internal/db/llm_jobs.go` — update job lifecycle queries, handle JSON array for `comment_ids`
- [x] 3.12 Rewrite `internal/db/billing.go` — update Stripe idempotency, subscription updates, quota queries
- [x] 3.13 Rewrite `internal/db/lifecycle.go` — update account deletion, GDPR export, chunked deletion, replace CTEs
- [x] 3.14 Rewrite `internal/db/exports.go` — update export job queue with atomic claiming
- [x] 3.15 Rewrite `internal/db/email.go` — update email send log and retry claiming
- [x] 3.16 Rewrite `internal/db/workspace.go` — update workspace CRUD, membership, invites, audit log, rate limiting
- [x] 3.17 Rewrite `internal/db/workspace_models.go` — update struct types if needed
- [x] 3.18 Replace `make_interval()` calls with Go-side duration computation or SQLite date functions
- [x] 3.19 Replace `statement_timeout`/`lock_timeout` with Go `context.WithTimeout()`

## 4. Application Entry Point Updates

- [x] 4.1 Update `cmd/api/main.go` — replace `pgxpool` initialization with `database/sql` + SQLite driver
- [x] 4.2 Update `cmd/api/main.go` — read `DATABASE_PATH` instead of `DATABASE_URL` with default fallback
- [x] 4.3 Update any CLI tools (`backfill_stripe_subscription_id`, `cancel_deletion`, `seed_db`) to use new connection pattern

## 5. Test Harness Rewrite

- [x] 5.1 Rewrite `internal/dbtest/harness.go` — replace testcontainers-postgres with in-memory SQLite databases
- [x] 5.2 Update test harness to apply `schema.sql` at startup for each test database
- [x] 5.3 Ensure test isolation with separate in-memory databases per test
- [x] 5.4 Remove `docker-compose.test.yml`
- [x] 5.5 Update `Makefile` to remove postgres-related targets
- [ ] 5.6 Run `go test ./internal/db/...` and fix any failing tests

## 6. Documentation and Cleanup

- [ ] 6.1 Update `README.md` to reflect SQLite as the database
- [ ] 6.2 Update `docs/` to remove PostgreSQL setup instructions
- [ ] 6.3 Update `AGENTS.md` spec map guidance to reflect SQLite storage layer
- [ ] 6.4 Run `go mod tidy` to clean up unused dependencies
- [ ] 6.5 Run full test suite `go test ./...` and verify all tests pass
