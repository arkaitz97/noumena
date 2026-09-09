## Why

Documentation, CI, and operational hygiene lag reality after the SQLite migration and audit:

1. **Stale Postgres docs** — `PRD.md:385`, `openspec/project.md` (PostgreSQL 16/pgx, `FOR UPDATE SKIP LOCKED`, advisory locks), `openspec/AGENTS.md` ("pg_try_advisory_lock") still describe PostgreSQL; the app is SQLite (modernc). Anyone following these docs will configure the wrong stack.
2. **Missing runbooks** — `specs/DEFERRED_TASKS.md:72-80` claims runbooks 13.x/14.x are resolved, but `docs/ops/runbooks/` is empty; 15+ doc links dangle. It also claims `SentimentChart.vue`/`SentimentSparkline.vue` exist — they don't.
3. **Config docs drift** — README claims every config var is in `.env.example`, but `RATE_LIMIT_ENABLED`, `AUTH_MAX_PER_MIN`, `UNAUTH_MAX_PER_MIN`, `CLEANUP_TICK`, `COOKIE_SECURE` are read but not listed. `docs/api/billing.md:149` names `STRIPE_API_KEY` (actual: `STRIPE_SECRET_KEY`); `docs/api/insights.md:177` documents a `sort=most_toxic` value the API rejects.
4. **CI is minimal** — `.github/workflows/integration.yml` runs only `make test-integration`; no `go vet`, no `go test ./...`, no gofmt gate, no frontend build/tests, no dependency audit. `scripts/test-frontend.sh` (self-described CI entrypoint) is never invoked. `PRD.md:170` asks for npm/go audit.
5. **Dockerfile gaps** — no HEALTHCHECK, runs as root, and the default `EXPORT_DIR` is unwritable as container root (`EnsureExportDir` fails at boot).
6. **No backup story** — `PRD.md:178` calls for daily backup + WAL archiving; nothing exists; README references restoring "the last backup" with no procedure.
7. **Tree hygiene** — ~327 files got `chmod +x` (git mode drift, uncommitted); `gofmt -l` flags many Go files; `.gitignore` covers only `noumena.db`, `seed_db`, `.env` — frontend `node_modules`, `dist/`, `coverage/`, `.opencode/node_modules` are untracked-but-present.

## What Changes

- Correct Postgres-era docs to SQLite reality (PRD, openspec/project.md, openspec/AGENTS.md).
- Restore missing runbooks (deletion, export, backup, restore, incident basics) referenced by DEFERRED_TASKS; fix or annotate the SentimentChart claim.
- Complete `.env.example` with all config keys; fix `docs/api/*` inaccuracies (key names, sort values).
- Expand CI: vet, full test, gofmt gate, frontend build + vitest, `npm audit` + `go vulncheck`/`govulncheck`, invoke `scripts/test-frontend.sh`.
- Dockerfile: HEALTHCHECK, non-root user, writable `EXPORT_DIR`, document required env.
- Add SQLite backup procedure (daily `sqlite3 .backup` + WAL checkpoint/archive) with a script and runbook entry.
- Restore file modes (644/755 only where needed); run `gofmt -w`; extend `.gitignore`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None — docs/CI/ops only.

## Impact

- Docs: `PRD.md`, `README.md`, `docs/api/*`, `docs/ops/runbooks/*` (new), `specs/DEFERRED_TASKS.md`, `specs/ROADMAP.md`, `openspec/project.md`, `openspec/AGENTS.md`
- CI: `.github/workflows/integration.yml`, `Makefile`, `scripts/test-frontend.sh`
- Ops: `Dockerfile`, `.gitignore`, new `scripts/backup-db.sh`
- Hygiene: `gofmt -w` across Go tree, file-mode normalization
