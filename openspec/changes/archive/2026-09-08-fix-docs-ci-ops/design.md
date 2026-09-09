## Context

The repo migrated to SQLite (archived `migrate-to-sqlite`) but only the storage-layer spec was updated; project-level docs still claim PostgreSQL. DEFERRED_TASKS was written before the migration and asserts runbooks exist that don't. CI runs a single integration command. Dockerfile lacks health/lifecycle basics.

## Goals / Non-Goals

**Goals:**
- Every doc accurately describes SQLite + current endpoints/config.
- CI catches vet/fmt/test/frontend/audit regressions.
- Image is production-sane (healthcheck, non-root, writable export dir).
- Operators have a real backup/restore runbook.

**Non-Goals:**
- Not rewriting product documentation wholesale.
- Not introducing a new CI system (the openspec `self-hosted-ci-cd` change covers Woodpecker separately).
- Not implementing the missing product features (separate change).

## Decisions

### 1. Doc corrections

Update to source-of-truth values:
- Stack tables: SQLite (modernc) instead of PostgreSQL; job queue wording → SQLite-backed `UPDATE…RETURNING` claims (already correct in code).
- `openspec/project.md` + `openspec/AGENTS.md`: replace pg-specific guidance (`FOR UPDATE SKIP LOCKED`, `pg_try_advisory_lock`, `testcontainers` → `dbtest` harness, SQLite pragmas).
- `docs/api/*`: fix key names and documented query params by diffing against `internal/api/*` handlers and `config.go`.
- `README.md` config section: generate a key list from `config.go` and assert `.env.example` parity (could add a tiny test).

### 2. Runbooks

Create `docs/ops/runbooks/{backup-restore,account-deletion,data-export,incident,onboarding}.md`. Update `DEFERRED_TASKS.md` to remove the false "resolved" claims (or mark pending). Either restore `SentimentChart.vue`/`SentimentSparkline.vue` or delete the claim (they are absent — delete the claim; sentiment feature is a separate change).

### 3. CI expansion

`integration.yml`: add jobs/steps:
- `go vet ./...`
- `go test ./...` (with `-race` where cheap)
- `gofmt -l .` gate (fail if non-empty)
- `make test-integration`
- frontend: `npm ci`, `npm run build`, `npm run test:unit` (vitest)
- `npm audit --omit=dev` + `govulncheck ./...` (or note as scheduled)
- invoke `scripts/test-frontend.sh` as the frontend entrypoint

### 4. Dockerfile

- `HEALTHCHECK` hitting a `/healthz` (add if absent in `main.go` — check; if not, add a trivial handler).
- `USER` non-root (e.g. `app` UID), ensure `EXPORT_DIR` (e.g. `/var/lib/noumena/exports`) is created with ownership in the image.
- Document required env (`ENCRYPTION_KEY`, `JWT_SECRET`, `DATABASE_PATH` mount).

### 5. Backup/restore

`scripts/backup-db.sh`: `sqlite3 "file:${DB}?_pragma=wal_checkpoint(TRUNCATE)"` + `sqlite3 .backup` to timestamped file, rotate N copies, optional offsite copy. Runbook covers restore (stop app, restore file, restart). Wire as optional systemd/cron timer (docs only).

### 6. Hygiene

- `find . -type f -not -path './.git/*' -exec chmod 644 {} +` then re-`chmod +x` only the actual scripts (`scripts/*.sh`, `tools/setup-machine.sh` etc.), commit the mode normalization.
- `gofmt -w` all Go files; add a `make fmt` target and CI gate.
- `.gitignore`: add `frontend/node_modules/`, `frontend/dist/`, `frontend/coverage/`, `.opencode/node_modules/`, `*.db-journal`, `exports/`.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| gofmt gate blocks unrelated PRs | Run `gofmt -w` in this change first; gate from then on |
| README/config parity test brittle | Test only presence of keys read by config.go in `.env.example`, not values |
| Backup script on live WAL DB | Use `sqlite3 .backup` (safe online backup) + checkpoint; document single-writer constraint |
| Non-root image breaks write paths | Verify EXPORT_DIR + SQLite path ownership in CI image build test |
