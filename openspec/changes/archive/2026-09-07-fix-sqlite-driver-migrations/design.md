## Context

`modernc.org/sqlite` is the pure-Go driver. Its DSN supports a fixed set of parameters (`_pragma`, `_time_format`, `_timezone`, `_txlock`, `_inttotime`, `_texttotime`); everything else is ignored silently — which is why `_journal_mode=WAL&_fk=true` produced no effect. The boot script (`schema.sql`) is CREATE-IF-NOT-EXISTS, so it can never alter existing tables. Column drift is a symptom of that missing mechanism.

## Goals / Non-Goals

**Goals:**
- Production DB runs with WAL, FK ON, busy_timeout 5000ms (verified at boot).
- Schema changes delivered as ordered, idempotent, versioned migrations — including ALTERs.
- Fix the deletion/export column drift so those workers stop erroring.
- Backfill FTS5 so search covers all comments.

**Non-Goals:**
- Not moving off SQLite.
- Not supporting multi-writer replication (single-writer WAL stays).
- Not re-architecting the harness beyond applying the real schema.

## Decisions

### 1. Driver-supported DSN

`dsn := "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"`. Keep `_txlock` default. `_loc=UTC` dropped (all writes already use explicit `strftime(...,'Z')`). Add a boot-time assert that queries `PRAGMA journal_mode`, `PRAGMA foreign_keys`, `PRAGMA busy_timeout` and logs/errors on mismatch.

### 2. Versioned migrations

New table:
```sql
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
```
`ApplyMigrations` runs an ordered slice of `{version int, stmts []string}` in a transaction per migration, inserts the version row, and skips applied versions. Migration 1 = existing `schema.sql` baseline (re-created as CREATE IF NOT EXISTS). Subsequent migrations are plain `ALTER TABLE`/`CREATE INDEX`/`INSERT` steps. Never edit an applied migration.

### 3. Column realignment

Migration 2 renames: `deletion_jobs.last_error`→`error_message`, `data_export_jobs.storage_path`→`export_url`, `data_export_jobs.download_url`→`export_url` (SQLite: ADD new column, UPDATE copy, DROP old), drop `partial_export` if present. Then update Go code to the canonical names (`error_message`, `export_url`) so code and schema agree.

### 4. CHECK + FTS + indexes migration

Migration 3: rebuild `subscription_status` CHECK to include `'trialing'` (recreate table or use `ALTER TABLE ... DROP CONSTRAINT` — SQLite needs table-rebuild; acceptable, it's small). Migration 4: `INSERT INTO comments_fts(comments_fts) VALUES('rebuild')` once. Migration 5: add partial index `CREATE INDEX idx_comments_processed ON comments(processed_at) WHERE processed_at IS NULL`, plus `weekly_digests(status, created_at)` and `platform_channels(deletion_scheduled_at)`.

### 5. Harness parity

`internal/dbtest/harness.go` should open a temp DB and call the same `ApplyMigrations`, eliminating the duplicated (already-diverged) schema copy and the datetime format drift.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Table-rebuild for CHECK drop on large tables | `subscription_status` lives on `users` (small); run in migration tx with backup note |
| Existing live DB has drifted rows from broken workers | Migrations run after fix; column renames copy existing values so no data loss |
| Two processes opening DB mid-migration | Migrations run before workers start; WAL + busy_timeout reduces collision; single-writer deploy model |
| FTS rebuild locks table | Run as part of startup migration on small-to-medium datasets; document timing for large DBs |
