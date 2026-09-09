## 1. Fix DSN pragmas

- [x] 1.1 Change DSN to `?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)`
- [x] 1.2 Remove `_loc=UTC` (and other unsupported keys)
- [x] 1.3 Add boot-time pragma assertions (journal_mode, foreign_keys, busy_timeout)
- [x] 1.4 Test: open temp DB via InitDB, assert all three pragmas

## 2. Versioned migration framework

- [x] 2.1 Create `schema_migrations(version, applied_at)` table
- [x] 2.2 Add `internal/db/migrations.go` with ordered migration runner (tx per migration, version ledger)
- [x] 2.3 Convert existing `schema.sql` boot into migration v1 (CREATE IF NOT EXISTS baseline)
- [x] 2.4 Make `ApplyMigrations` run pending migrations only, idempotently
- [x] 2.5 Test: apply twice → no-op second time; version rows recorded

## 3. Fix column drift

- [x] 3.1 Migration: `deletion_jobs.last_error` → `error_message`
- [x] 3.2 Migration: `data_export_jobs.storage_path`/`download_url` → `export_url`; drop `partial_export`
- [x] 3.3 Update `internal/db/lifecycle.go` column constants to `error_message`/`export_url`
- [x] 3.4 Update `internal/lifecycle/deletion_worker.go`, `export_worker.go` queries
- [x] 3.5 Test: real-SQLite deletion job claim + export job completion succeed

## 4. CHECK constraint, FTS rebuild, indexes

- [x] 4.1 Migration: `subscription_status` CHECK includes `'trialing'` (table rebuild for users)
- [x] 4.2 Migration: FTS5 `rebuild` for `comments_fts`
- [x] 4.3 Migration: partial index on `comments(processed_at) WHERE processed_at IS NULL`
- [x] 4.4 Migration: indexes on `weekly_digests(status, created_at)` and `platform_channels(deletion_scheduled_at)`
- [x] 4.5 Test: `trialing` status insert succeeds; FTS finds pre-migration comment

## 5. Harness parity

- [x] 5.1 `internal/dbtest/harness.go` uses real `ApplyMigrations` instead of duplicated schema
- [x] 5.2 Remove divergent schema copy and datetime workaround
- [x] 5.3 Full `go test ./...` green

## 6. Verify

- [x] 6.1 Boot app against existing `noumena.db`, confirm migrations apply and pragmas set
- [x] 6.2 Reboot again → no re-apply
- [x] 6.3 Live sanity: schedule deletion, confirm worker progresses past claim
