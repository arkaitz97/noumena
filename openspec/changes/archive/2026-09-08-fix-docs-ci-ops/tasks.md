## 1. Fix stale docs

- [x] 1.1 PRD.md stack/schema sections → SQLite reality
- [x] 1.2 openspec/project.md + AGENTS.md: replace pg-specific guidance
- [x] 1.3 docs/api/*: fix key names (STRIPE_SECRET_KEY) and sort params (newest/oldest only)
- [x] 1.4 .env.example: add RATE_LIMIT_ENABLED, AUTH_MAX_PER_MIN, UNAUTH_MAX_PER_MIN, CLEANUP_TICK, COOKIE_SECURE + new keys from crypto/rate-limit changes
- [x] 1.5 README config-parity claim verified by a test

## 2. Runbooks + DEFERRED_TASKS

- [x] 2.1 Create docs/ops/runbooks/{backup-restore,account-deletion,data-export,incident,onboarding}.md
- [x] 2.2 Fix DEFERRED_TASKS claims (runbooks, SentimentChart absent → annotate/remove)
- [x] 2.3 Verify 15+ dangling doc links resolve

## 3. CI expansion

- [x] 3.1 Add go vet + go test ./... + gofmt gate to integration.yml
- [x] 3.2 Add frontend job (npm ci, build, vitest) via test-frontend.sh
- [x] 3.3 Add npm audit + govulncheck step
- [x] 3.4 Makefile fmt/test targets

## 4. Dockerfile

- [x] 4.1 Add /healthz handler (if absent) + HEALTHCHECK
- [x] 4.2 Non-root USER; EXPORT_DIR owned/writable
- [x] 4.3 Document required env in Dockerfile/README

## 5. Backup story

- [x] 5.1 scripts/backup-db.sh (checkpoint + .backup + rotation)
- [x] 5.2 Backup/restore runbook
- [x] 5.3 Optional timer/cron docs

## 6. Tree hygiene

- [x] 6.1 Normalize file modes (644; +x only for scripts) and commit
- [x] 6.2 gofmt -w all Go files; verify gofmt -l empty
- [x] 6.3 Extend .gitignore (frontend node_modules/dist/coverage, .opencode/node_modules, *.db-journal, exports/)

## 7. Verify

- [ ] 7.1 CI green on a branch run
- [x] 7.2 docker build + container boot: healthcheck passes, export dir writable
- [x] 7.3 Backup script produces restorable file; restore tested on a throwaway copy