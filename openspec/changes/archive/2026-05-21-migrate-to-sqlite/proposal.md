## Why

PostgreSQL adds operational complexity (separate service, connection pooling, container orchestration) that is unnecessary for this single-instance analytics application. SQLite eliminates the external database dependency, simplifies deployment, reduces resource usage, and removes the need for Docker Compose in development/testing while maintaining sufficient performance for the current workload.

## What Changes

- Replace `pgx/v5` driver with `modernc.org/sqlite` (pure-Go, no CGO)
- Rewrite `internal/db/schema.sql` from PostgreSQL dialect to SQLite dialect
- Replace all PostgreSQL-specific SQL features with SQLite equivalents
- Update Go database layer to use `database/sql` with SQLite driver
- Replace test harness from testcontainers-postgres to in-memory SQLite databases
- Remove Docker Compose test infrastructure
- Update connection configuration from `DATABASE_URL` to `DATABASE_PATH`

## Capabilities

### New Capabilities

(none — this is an implementation-only change, no new user-facing capabilities)

### Modified Capabilities

(none — all existing capability requirements remain unchanged. Only the storage layer implementation changes.)

## Impact

- **`internal/db/`**: Complete rewrite of all query files — schema, db.go, models, comments, channels, ingestion, digests, llm_jobs, billing, lifecycle, exports, email, workspace, auth
- **`internal/dbtest/harness.go`**: Replace testcontainers-based PostgreSQL with in-memory SQLite
- **`docker-compose.test.yml`**: Removed
- **`cmd/api/main.go`**: Updated database initialization
- **`go.mod`**: Replace `pgx/v5` with `modernc.org/sqlite`, remove `testcontainers-go/modules/postgres`
- **`.env.example`**: Replace `DATABASE_URL` with `DATABASE_PATH`
- **`Makefile`**: Remove postgres-related targets
- **`docs/`**: Update any deployment or setup documentation referencing PostgreSQL
