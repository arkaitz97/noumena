# 0005. Use SQLite with one writer as the system of record

## Status

Accepted. 2026-11-24.

## Context and Problem Statement

The system needs a database for users, channels, comments, LLM jobs, billing
state, audit entries, and lifecycle jobs. The expected volume is one creator's
or a small agency's channels: tens of thousands to low millions of comments,
one deployment, one operator. Read traffic is a dashboard; write traffic is
batch ingestion and classification workers.

The original product notes assumed PostgreSQL. The implementation uses
SQLite (`modernc.org/sqlite`). The decision must be explicit because it sets a
hard scaling ceiling and changes how tests run.

## Decision Drivers

- One process and one host are acceptable for the target user.
- Operating a database server adds a failure mode and a deployment step that
  the project does not need.
- Tests must run without Docker or external services.
- Transactions, foreign keys, unique constraints, full-text search, and
  partial indexes are required.
- The code must be portable to PostgreSQL later without a rewrite of the
  domain layer.

## Considered Options

1. **PostgreSQL with a connection pool and migrations.**
2. **SQLite in WAL mode with a single connection pool and embedded
   migrations.**
3. **An embedded key-value store (BoltDB or Badger).**

## Decision Outcome

Chosen option 2.

`InitDB` opens SQLite with `journal_mode(WAL)`, `busy_timeout(5000)`, and
`foreign_keys(1)`, then asserts the pragmas actually took effect. The pool is
capped at one connection because SQLite serializes writes
(`internal/db/db.go`). A second read-only handle is opened with
`query_only(1)` for long export streams so they do not hold the main
connection.

Migrations are embedded and versioned (`internal/db/migrations.go`). Version 1
is `schema.sql`; later versions repair or extend it inside a transaction and
record the version in `schema_migrations`. The test harness
(`internal/dbtest/harness.go`) applies the same migration set to an in-memory
database, so tests run with `go test ./...` and no Docker.

SQLite-specific SQL is confined to `internal/db`: `?` placeholders,
`ON CONFLICT`, `json_each`, FTS5. The handler and worker layers depend on
interfaces (`DBQuery`, `WorkerDB`), which keeps a PostgreSQL port plausible.

### Consequences

#### Good

- `make test` needs no services. All 88 Go test files run in one command.
- Backup is a file copy plus the WAL checkpoint procedure in
  `docs/ops/runbooks/backup-restore.md`.
- Foreign keys, partial unique indexes, and FTS5 are available and used.

#### Bad

- One writer. A long write transaction blocks every other write, including
  request-path writes. The export path was split onto a separate read-only
  handle to reduce this risk, but ingestion bursts still queue.
- No horizontal scale. A second API process would race on job claims and on
  the file. Running two replicas is unsupported.
- Some SQL is not portable: `ON CONFLICT`, `json_each`, FTS5, and `rowid`.
  A PostgreSQL migration touches `internal/db` broadly.
- The code contains stale comments that mention Postgres advisory locks and
  `FOR UPDATE SKIP LOCKED`. Those refer to the original design; the SQLite
  lock helper is a no-op (`internal/ingestion/lock.go`).
- The export `query_only` handle is a mitigation, not isolation. A large
  export still competes for the same WAL and disk.

#### Neutral

- WAL means readers do not block writers, which is why the dashboard remains
  responsive during ingestion.
- `busy_timeout(5000)` turns lock contention into latency, not immediate
  errors.
- File-level backup requires the WAL checkpoint procedure; copying only the
  main file can lose recent commits.

## Confirmation

- `go test ./internal/db/ -run 'Test_ApplyMigrations_Idempotent'`
- `go test ./internal/dbtest/...`
- The boot-time pragma assertion fails the process if WAL, foreign keys, or
  busy timeout did not apply.
- CI runs `go test ./...` with no database service.

## More Information

- `internal/db/db.go`
- `internal/db/migrations.go`
- `internal/db/schema.sql`
- `internal/dbtest/harness.go`
- `docs/ops/runbooks/incident.md` (single-writer rule)
- `docs/ops/runbooks/backup-restore.md`
