## Why

The SQLite migration (archived `migrate-to-sqlite`) updated the storage layer but left Postgres-isms and a broken DSN, silently disabling correctness guarantees in production (verified against the live `noumena.db`):

1. **DSN pragmas ignored** — `internal/db/db.go:37` uses `_journal_mode=WAL&_busy_timeout=5000&_fk=true&_loc=UTC`. The `modernc.org/sqlite` driver only honors `_pragma`, `_time_format`, `_timezone`, `_txlock`, `_inttotime`, `_texttotime`; unknown keys are silently dropped. Live DB reports `journal_mode=delete`, `foreign_keys=0`. **No WAL, no FK enforcement, no busy_timeout in production** — cascades are inert, concurrent writers get immediate SQLITE_BUSY.
2. **No migration mechanism** — `ApplyMigrations` (`db.go:22-30`) is CREATE-IF-NOT-EXISTS only; zero `ALTER TABLE` anywhere. Columns added after first creation never materialize on existing DBs (already bit: `last_error`/`storage_path` drift in the same commit that shipped them).
3. **Column drift** — code reads/writes `last_error`, `storage_path`, `download_url`, `partial_export` (`internal/db/lifecycle.go`), but `schema.sql` declares `error_message`, `export_url`. Deletion worker and export worker error at runtime (reproduced).
4. **CHECK constraint excludes valid status** — `schema.sql:29` `subscription_status` CHECK omits `'trialing'`, but code writes it (`billing_handlers.go:751`, `billing.go:273`).
5. **FTS5 never backfilled** — `comments_fts` added post-hoc; pre-existing comments are not indexed (no rebuild) → search silently misses older comments.
6. **Missing indexes** — `comments.processed_at` (LLM hot path), `weekly_digests(status, created_at)`, `platform_channels.deletion_scheduled_at`.

## What Changes

- Fix DSN to driver-supported pragmas: `?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)`. Assert pragmas on boot.
- Add a versioned migration framework: `schema_migrations` table + ordered migration steps (including `ALTER TABLE` for column renames/additions), run idempotently at startup. Convert the boot CREATE-IF-NOT-EXISTS script into migration v1.
- Add migration steps for: `last_error`→`error_message`, `storage_path`/`download_url`→`export_url`, `partial_export` removal, `subscription_status` CHECK incl. `'trialing'`, FTS5 rebuild, new indexes.
- Update code references (`lifecycle.go`, `export_worker.go`, `deletion_worker.go`) to the correct column names.
- Align the `dbtest` harness to apply the embedded schema via the migration runner instead of a divergent copy.

## Capabilities

### New Capabilities

- `schema-migrations`: Versioned, idempotent SQLite migrations with ALTER support and a `schema_migrations` ledger.

### Modified Capabilities

- `storage-layer`: SQLite is opened with WAL journaling, foreign keys ON, and a busy timeout; schema changes are applied through versioned migrations.

## Impact

- `internal/db/db.go`, `internal/db/schema.sql`, new `internal/db/migrations.go`
- `internal/db/lifecycle.go`, `internal/db/exports.go`, `internal/lifecycle/deletion_worker.go`, `internal/lifecycle/export_worker.go`, `internal/api/billing_handlers.go`
- `internal/dbtest/harness.go` (drop duplicated schema copy)
- New tests asserting boot-time pragmas and migration idempotency.
