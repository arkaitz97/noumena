## Context

Noumena is a Go-based social media comment analytics platform using PostgreSQL 16 via `pgx/v5` with connection pooling. The schema is a single embedded SQL file (641 lines) applied idempotently at startup. The application uses 22 tables, 1 materialized view, and extensive PostgreSQL-specific features: JSONB, GIN indexes, UUID arrays, advisory locks, `FOR UPDATE SKIP LOCKED`, recursive CTEs, full-text search, partial indexes, CHECK constraints, `xmax` system column, and `gen_random_uuid()`.

All queries are raw SQL — no ORM. The test harness uses testcontainers-go to spin up ephemeral PostgreSQL instances.

## Goals / Non-Goals

**Goals:**
- Eliminate external PostgreSQL dependency; single-binary deployment with embedded SQLite
- Maintain identical application behavior and API contracts
- Preserve all existing functionality: job queues, full-text search, JSON storage, analytics queries
- Keep tests fast and self-contained (no Docker required)
- Use pure-Go SQLite driver (no CGO) for cross-compilation simplicity

**Non-Goals:**
- No performance optimization beyond functional parity
- No schema redesign or normalization changes
- No changes to application logic, API routes, or user-facing behavior
- No multi-writer / WAL mode clustering support

## Decisions

### 1. SQLite driver: `modernc.org/sqlite` (pure-Go)

**Rationale:** Avoids CGO, enables cross-compilation, no system-level SQLite dependency. `mattn/go-sqlite3` is faster but requires CGO and a C compiler.

**Alternatives considered:**
- `mattn/go-sqlite3`: Faster, battle-tested, but CGO dependency complicates builds
- `ncruces/go-sqlite3`: WASM-focused, less mature

### 2. UUIDs: stored as TEXT (canonical hex string)

PostgreSQL uses `UUID` type with `gen_random_uuid()`. SQLite has no native UUID.

**Rationale:** Store UUIDs as `TEXT` in canonical form (`xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`). Generate UUIDs in Go using `github.com/google/uuid` before insert. Replace `gen_random_uuid()` defaults with Go-side generation.

**Alternatives considered:**
- BLOB (16 bytes): More compact but harder to debug and requires conversion in every query
- TEXT without dashes: Slightly smaller but loses standard UUID format

### 3. JSONB → JSON (stored as TEXT)

SQLite has no JSONB binary format. Use `JSON` type affinity (stored as TEXT) with SQLite's `json()` and `json_extract()` functions for queries.

**GIN index replacement:** SQLite cannot index JSONB with GIN. Replace with:
- For `actionable_insights` queries: use `json_extract()` in WHERE clauses with a regular index on extracted columns, or accept full table scan for analytics queries (acceptable given current data volume)
- Create generated columns for frequently queried JSONB paths and index those

### 4. UUID arrays → JSON arrays

`llm_processing_jobs.comment_ids UUID[]` becomes `comment_ids TEXT` storing a JSON array of UUID strings. Use `json_each()` for iteration.

### 5. Advisory locks → application-level locking with SQLite `GET_LOCK` alternative

PostgreSQL advisory locks (`pg_try_advisory_lock`) are used for per-channel ingestion serialization.

**Replacement:** Use SQLite's `BEGIN IMMEDIATE` transaction + a lock table row with `UPDATE ... WHERE` pattern. Or use `SELECT ... FOR UPDATE` equivalent via `BEGIN IMMEDIATE` which serializes writes. Since SQLite serializes all writes by default, explicit advisory locks are unnecessary — the database itself provides the serialization.

### 6. `FOR UPDATE SKIP LOCKED` → application-level job claiming

PostgreSQL uses `SELECT ... FOR UPDATE SKIP LOCKED` for concurrent job queue claiming.

**Replacement:** Use atomic UPDATE-based claiming:
```sql
UPDATE ingestion_jobs
SET status = 'processing', claimed_at = CURRENT_TIMESTAMP
WHERE id = (
  SELECT id FROM ingestion_jobs
  WHERE status = 'pending' AND scheduled_at <= CURRENT_TIMESTAMP
  ORDER BY scheduled_at ASC
  LIMIT 1
)
RETURNING *;
```
SQLite supports `RETURNING` since 3.35.0. This is atomic and safe for concurrent workers.

### 7. Full-text search → SQLite FTS5

Replace `to_tsvector()/plainto_tsquery()` with SQLite FTS5 virtual table. Create an FTS5 table that mirrors the `comments` table's `body` column, kept in sync via triggers.

### 8. Materialized view → regular query or application-side caching

`mv_actionable_insights_agg` with `REFRESH MATERIALIZED VIEW CONCURRENTLY` has no direct SQLite equivalent.

**Replacement:** Convert to a regular query. If performance is insufficient, cache results in a regular table refreshed by a background goroutine.

### 9. `xmax` system column → `changes()` function

PostgreSQL uses `(xmax = 0)` to detect INSERT vs UPDATE in upserts.

**Replacement:** Use SQLite's `changes()` function or the `INSERT ... ON CONFLICT DO UPDATE` with a flag column approach. Alternatively, check `SELECT changes()` after the upsert — returns 1 for INSERT, 1 for UPDATE (ambiguous). Better: use a `INSERT ... ON CONFLICT DO UPDATE` and compare `rowid` before/after, or simply don't need this distinction if the application can handle both cases uniformly.

### 10. CHECK constraints → SQLite CHECK constraints

SQLite supports CHECK constraints natively. Direct translation.

### 11. Partial indexes → SQLite partial indexes

SQLite supports `WHERE` clause on indexes since 3.8.0. Direct translation.

### 12. `make_interval()` → application-side duration calculation

PostgreSQL's `make_interval()` for time arithmetic.

**Replacement:** Compute durations in Go and pass as parameters, or use SQLite's date functions (`datetime()`, `julianday()`).

### 13. `TIMESTAMPTZ` → TEXT with ISO 8601

SQLite stores timestamps as TEXT (ISO 8601), REAL (Julian day), or INTEGER (Unix epoch).

**Replacement:** Use TEXT in ISO 8601 format (`2024-01-15T10:30:00Z`). Go's `time.Time` marshals to this format naturally. Use SQLite's `datetime()` functions for comparisons.

### 14. `NUMERIC(4,2)` → REAL

SQLite has no fixed-precision decimal. Use `REAL` for `confidence` and `sentiment_score`.

### 15. Recursive CTEs → SQLite recursive CTEs

SQLite supports recursive CTEs. Direct translation.

### 16. CTEs → SQLite CTEs

SQLite supports CTEs. Direct translation.

### 17. `ON CONFLICT` / Upsert → SQLite `ON CONFLICT`

SQLite supports `ON CONFLICT` clause. Direct translation.

### 18. `FILTER (WHERE ...)` → CASE in aggregates

SQLite does not support `FILTER (WHERE ...)` on aggregates.

**Replacement:** Use `SUM(CASE WHEN condition THEN 1 ELSE 0 END)` pattern.

### 19. Test harness → in-memory SQLite

Replace testcontainers-postgres with `:memory:` SQLite databases. Each test gets its own in-memory database. Schema applied at test startup. No Docker required.

### 20. `statement_timeout` / `lock_timeout` → Go-side context timeouts

SQLite has no statement timeout. Apply timeouts via Go `context.WithTimeout()` on database operations.

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| **Concurrency**: SQLite allows only one writer at a time. Multiple ingestion/LLM workers will serialize. | Current workload is single-instance. If multi-writer needed later, use `wal` mode or revert to PostgreSQL. |
| **JSON query performance**: No GIN index equivalent for JSONB path queries. | Current data volume is manageable. Add generated columns + indexes if needed. |
| **Full-text search quality**: FTS5 ranking differs from PostgreSQL ts_rank. | Acceptable for current use. Tuning FTS5 parameters if needed. |
| **`RETURNING` support**: Requires SQLite 3.35.0+. | modernc.org/sqlite bundles a recent version. Verify version at startup. |
| **Large database file**: Single file can grow large. | SQLite handles multi-GB databases well. Add `VACUUM` maintenance if needed. |
| **No `pg_try_advisory_lock`**: Ingestion serialization relies on SQLite write serialization. | Works for single-process. Would need redesign for multi-process. |
| **Lost PostgreSQL features**: Window functions, `FILTER`, `xmax`, `make_interval()` all require workarounds. | All have documented alternatives (see Decisions section). |

## Migration Plan

1. Rewrite `schema.sql` to SQLite dialect
2. Replace `pgxpool` with `database/sql` + `modernc.org/sqlite`
3. Update all query files in `internal/db/`
4. Rewrite test harness
5. Update `go.mod` dependencies
6. Run full test suite
7. Update documentation and deployment scripts

**Rollback:** Keep PostgreSQL schema and queries in a git branch. Revert the change commit if issues arise. No data migration needed since this is a development-time change (production data would need export/import).

## Open Questions

1. **Data migration for existing deployments**: If any production instances exist, need a PostgreSQL → SQLite dump/import tool. (Assuming greenfield for now.)
2. **SQLite version pinning**: Should we verify minimum SQLite version at startup? (Recommended: check for 3.35.0+ for `RETURNING` support.)
3. **WAL mode**: Should we enable `PRAGMA journal_mode=WAL` for better concurrent read performance? (Recommended: yes.)
4. **Busy timeout**: What `PRAGMA busy_timeout` value? (Recommended: 5000ms to handle write serialization contention.)
