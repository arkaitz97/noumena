# Noumena Fix-Roadmap — Applying the Audit Change Set

**Status:** DRAFT | **Owner:** audit (2026-09-05)
**Scope:** 12 openspec changes in `openspec/changes/fix-*` + `implement-missing-product-features`, produced from the full code/security/UX audit.
**Validation:** all changes pass `openspec validate --changes`.

Apply each change with `/opsx-apply`. One change at a time; run its `tasks.md` to completion (incl. tests) before starting the next. Each phase below has a hard exit gate — do not proceed until it passes.

---

## Phase 0 — Pre-flight (before any apply)

**Goal:** safe working state + required secrets in place.

- [ ] Snapshot: copy `noumena.db` (+ `-journal`, `-wal`) to a dated backup. (If `scripts/backup-db.sh` from change 11 exists, use it.)
- [ ] Set `JWT_SECRET` (≥32 random bytes) and `ENCRYPTION_KEY` (32 bytes hex) in **all** environments now — changes 1 and 2 make them mandatory and will refuse to boot without them.
- [ ] Optional early hygiene from change 11 (do not require full 11): `gofmt -w`, extend `.gitignore`, normalize file modes, commit. Reduces noise for every later diff.
- [ ] Confirm current baseline: `go build ./...`, `go vet ./...`, `go test ./...` green; `npm run build` green.
- **Exit gate:** clean tree, working backup, secrets set in dev/staging/prod.

---

## Phase 1 — Critical security (ship-blockers)

**Changes:** `fix-auth-critical-security` → `fix-crypto-http-hardening`

**Why this order:**
- Change 1 is the remote-exploit critical (account takeover, forgeable JWT secret, deterministic invites) — nothing else matters until it's closed.
- Change 2 (hardening) touches the same auth/config surface; doing it second avoids touching `config.go`/`encryption.go` twice. Its ENCRYPTION_KEY-mandatory change is why Phase 0 must pre-set secrets.

**Exit gate:**
- Unauthenticated Google-link rejected; verified-token link works.
- Boot without `JWT_SECRET`/`ENCRYPTION_KEY` fails fast with clear errors.
- Spoofed `X-Forwarded-For` does not reset a rate bucket; authed users get per-user limits.
- No `?token=` in logs or OTel spans.

**Risk:** none known beyond requiring secrets in all envs (covered in Phase 0).

---

## Phase 2 — Storage correctness (unlocks everything data)

**Change:** `fix-sqlite-driver-migrations`

**Why this order:** This is the prerequisite for phases 3 and 6-features. It fixes the DSN pragmas (WAL/FK/busy_timeout — currently off in prod), introduces versioned ALTER migrations, repairs the `last_error`/`export_url` column drift, and fixes the `trialing` CHECK. Until this lands, deletion/export workers error and cascades are inert.

**Exit gate:**
- On a **copy** of the live DB: migrations apply once, idempotent on re-run; `journal_mode=wal`, `foreign_keys=1`, `busy_timeout≥5000`.
- Deletion-job claim and export-job completion succeed against real SQLite (they previously errored).
- `trialing` status insert succeeds.

**Risk:** migration runs on startup — do not roll forward before Phase 0 backup; verify on a copy first. Medium change to DB layer: run `go test ./internal/db/... ./internal/dbtest/...` explicitly.

---

## Phase 3 — Data lifecycle recovery (GDPR + export)

**Changes:** `fix-data-lifecycle-gdpr` → `fix-export-pipeline`

**Why this order:** Both depend on Phase 2 (columns + FK-on). Deletion (4) first because it fixes the 500-on-delete and GDPR erasure gaps; export (5) shares the lifecycle/worker surface and the same column fix, so it lands cleanly after. Reapers for stuck export jobs and non-blocking export are queued behind the worker recovery work in Phase 4.

**Exit gate (staging, real DB):**
- Account deletion: request succeeds → job completes → no user-owned rows remain in any table (check with FK on AND off).
- Refresh token revoked at provider; flaky revoke no longer stalls the worker.
- Workspace delete NULLs member `active_workspace_id`.
- Export: ZIP + URL + signed download works; crash mid-export recovers on restart; no orphan files; concurrent writes not blocked.

**Risk:** deletion/export are destructive — run against staging/users you control first. This is the first phase where production user data is actually erased.

---

## Phase 4 — Delivery reliability (email + workers)

**Changes:** `fix-email-delivery` → `fix-workers-recovery-shutdown`

**Why this order:**
- Change 6 fixes silent email loss (retry never sent, idempotency race, SMTP hangs, empty digest email) — a production-correctness bug independent of data layer.
- Change 8 builds on it: atomic email claim, stuck-job reapers, digest catch-up/deferred retry, shutdown ordering, panic safety. Apply 6 first so 8's claim changes build on a working send path.
- Both touch `retry_worker.go`/`pipeline.go`/`digest_worker.go` — sequential application avoids double-editing.

**Exit gate:**
- Transient SMTP failure → retry actually delivers; duplicate key never drops a pending email.
- Digest email renders real content; unsubscribe works.
- Kill process mid-ingestion → restart recovers channel; no duplicate LLM spend; deferred digest re-dispatched; missed Monday digest caught up.
- SIGTERM drains workers → HTTP → telemetry, no DB race, no panic kills process.

**Risk:** 6 is safe; 8's reaper + shutdown reorder could surface latent race — test with forced kills. Watch for regressions in digest delivery.

---

## Phase 5 — Multi-tenancy + frontend

**Changes:** `fix-workspace-authz-api` → `fix-frontend-plumbing` → `fix-frontend-ux-a11y`

**Why this order:**
- 7 is the backend foundation (roles enforced, workspace scoping, shared-channel grants, atomic channel cap, backfill wiring). It needs Phase 2's migration mechanism for the grant table + `workspace_id` backfill.
- 9 (frontend) depends on 7: role fetch, members flow, and role-gated export are only meaningful once the backend enforces roles. 9 also brings its own backend bits (`PATCH /user/me`, reauthorize).
- 10 (UX/a11y) touches the same views/components as 9 (DashboardLayout, AccountView, stores) — do it last in the phase to avoid merge churn, then follow with the visual/a11y polish.

**Exit gate:**
- Viewer blocked from write ops (403); analyst can export; admin manages members; shared channel visible to members only; backfill enqueued on first connect, not on reconnect.
- Email change verifies; reconnect works without duplicate channel; export result + link surfaced; 429 banner fires; logout/`next` redirect correct.
- Mobile nav functional; dark mode readable; axe clean on dashboard/settings; pagination stable under star/unstar.

**Risk:** role enforcement can break existing UX for users with no workspace — Phase 0 onboarding assigns a default workspace; watch for 403s in staging.

---

## Phase 6 — Product features

**Change:** `implement-missing-product-features` (apply in sub-batches)

**Why last:** it is the largest and builds on earlier phases — sentiment needs Phase 2 (column migration) + Phase 4 (digest/LLM stability), seats need Phase 5 (workspace roles), trials need the `trialing` CHECK fix from Phase 2. Applying on a stable, hardened base minimizes variables when debugging new features.

**Recommended sub-batch order (each independently shippable):**
1. Password change + token-expiry banner (small, auth-adjacent, no new schema)
2. Usage history (needs `billing_usage` migration)
3. Trials (needs Phase 2 CHECK fix; activates reminder cron)
4. Sentiment scoring (needs column migration + LLM prompt change + backfill — watch LLM cost on backfill)
5. Seat enforcement (needs Phase 5 roles)
6. Overage invoicing (Stripe; test in test mode first)
7. LLM provider failover (riskiest — new clients + secrets; last)

**Exit gate:** each sub-batch green on its own; sentiment trend renders; seat cap blocks; trial countdown shows; overage invoice appears exactly once in Stripe test mode.

**Risk:** LLM prompt changes (sentiment) alter classification output — A/B on staging with a sample of comments before full backfill.

---

## Phase 7 — Docs, CI, ops finalize

**Change:** `fix-docs-ci-ops`

**Why last:** the doc-drift half (Postgres→SQLite, runbooks, `.env.example` parity, `docs/api/*`) should reflect the *final* code, so it lands after features settle. The CI/hygiene half (vet/gofmt/frontend/audit gates, backup script, Dockerfile healthcheck/non-root) is valuable earlier — pull those tasks forward to Phase 0 or anytime after Phase 1 if you want the gates protecting later phases.

**Exit gate:**
- Docs match code (SQLite, real config keys, real sort params); runbooks exist; DEFERRED_TASKS claims corrected.
- CI runs vet + full tests + gofmt + frontend + audits and fails on regression.
- Image boots with healthcheck, non-root, writable export dir.
- `backup-db.sh` produces a restorable file (test restore on a copy).

---

## Execution matrix (parallelizable where marked)

| Phase | Changes | Can parallelize with | Hard dependency |
|---|---|---|---|
| 0 | pre-flight | — | — |
| 1 | 1, 2 | — | secrets from Phase 0 |
| 2 | 3 | — | Phase 0 backup |
| 3 | 4, 5 | — | Phase 2 |
| 4 | 6, 8 | 6 ∥ 7 (different surfaces) | 6→8 ordered; 8∦7 both touch db/workers |
| 5 | 7, 9, 10 | 7 ∥ 6 | 7→9→10 ordered |
| 6 | 12 | 12c(seats) after 7; 12a after 2 | Phase 2, Phase 5 |
| 7 | 11 | CI half early (Phase 0) | docs half at end |

Shortest sensible critical path: **0 → 1 → 3 → 4 → 5 → 6b.** (2 can trail; 6/7 parallelize.)

## Deploy notes
- Every phase is additive until Phase 3; after Phase 2 all deploys must run migrations (they run at boot). Keep the pre-migration backup.
- Changes 1/2 change required env — update Coolify/`Dockerfile` env before deploying those.
- Rollback: for DB-layer changes, restore the Phase 0 backup; for pure API/worker changes, revert commit + redeploy (no data impact).
- Re-run `openspec validate --changes` and archive each change (`/opsx-archive`) as its exit gate passes.
