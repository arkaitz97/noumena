# Onboarding

Getting a new operator/developer productive on Noumena.

## Prerequisites

- Go 1.26+, Node 20+ (frontend), `sqlite3` CLI (backups).
- A copy of `.env.example` → `.env`, filled in per
  [`../../setup-guide.md`](../../setup-guide.md).

## Verification

- `make test` passes (fast Go suite).
- `make test-integration` passes (DB-backed, in-memory SQLite — no Docker).
- `bash scripts/test-frontend.sh` passes (vitest, one-shot).

## First run

```sh
cp .env.example .env            # then fill secrets
go run ./cmd/api                # boots API on :8080, applies schema idempotently
cd frontend && npm ci && npm run dev
```

- Health: `curl localhost:8080/health`.
- The DB file lands at `DATABASE_PATH` (default `./noumena.db`).
- Secrets `JWT_SECRET` and `ENCRYPTION_KEY` are mandatory — the process
  refuses to start without them.

## Daily operations

- Backups: `bash scripts/backup-db.sh` (see
  [`backup-restore.md`](backup-restore.md)).
- Deploy: [`../../deploy-coolify.md`](../../deploy-coolify.md).
- Incident response: [`incident.md`](incident.md).
