## 1. Schema and admin flag

- [ ] 1.1 Add `is_admin BOOLEAN NOT NULL DEFAULT 0` to the `users` DDL in `internal/db/schema.sql`
- [ ] 1.2 Add migration v23 in `internal/db/migrations.go` adding `users.is_admin` with an introspection guard
- [ ] 1.3 Extend `userColumns`, `scanUser`, and the `User` model in `internal/db/db.go` and `internal/db/models.go`
- [ ] 1.4 Add migration tests for fresh install and existing-user default

## 2. Admin authorization

- [ ] 2.1 Add `ADMIN_EMAILS` parsing to `internal/config/config.go` with tests
- [ ] 2.2 Implement `internal/api/admin_middleware.go` with `RequireAdmin` (flag OR allowlisted email, 401/403 paths)
- [ ] 2.3 Return `is_admin` from `GET /auth/me` in `internal/api/auth_handlers.go`
- [ ] 2.4 Add `tools/grant_admin` CLI to set the flag by email
- [ ] 2.5 Add middleware tests: flag admin, allowlisted admin, non-admin 403, unauthenticated 401

## 3. Campaign reporting queries

- [ ] 3.1 Implement `internal/db/campaign_report.go`: summary, per-attribution breakouts, paginated cohort, CSV row source over `v_campaign_cohort`
- [ ] 3.2 Encode the GREEN/YELLOW/RED thresholds as named constants with a unit test on the boundaries
- [ ] 3.3 Add DB tests for summary, empty cohort, date filter, and breakout grouping

## 4. Admin campaign API

- [ ] 4.1 Implement `internal/api/admin_campaign_handlers.go`: `GET /api/admin/campaign/summary`, `/cohort`, `/export.csv`
- [ ] 4.2 Stream CSV via the `internal/export` encoder
- [ ] 4.3 Write an `audit_log` entry (`admin_campaign_view`) on each successful admin read
- [ ] 4.4 Mount the nested `/api/admin` group with `RequireAdmin` in `cmd/api/main.go`
- [ ] 4.5 Add handler tests: admin success, non-admin 403, pagination, CSV content type, audit row written

## 5. Frontend admin surface

- [ ] 5.1 Extend the auth store/types to carry `is_admin` from the current-user response
- [ ] 5.2 Add the `requiresAdmin` router guard and the `/admin/campaign` route in `frontend/src/router/index.ts`
- [ ] 5.3 Create `frontend/src/services/adminCampaign.ts` and `frontend/src/stores/admin.ts`
- [ ] 5.4 Create `frontend/src/views/admin/CampaignDashboard.vue` (funnel, ACC vs target, TTFI, high-intent, decision bucket, breakouts, CSV button, small-sample caveat)
- [ ] 5.5 Add the conditional admin navigation entry in the app layout
- [ ] 5.6 Add Vitest tests for the guard, conditional nav, and dashboard rendering of the decision bucket

## 6. Verification

- [ ] 6.1 Run `make test` and `gofmt`/lint clean
- [ ] 6.2 Run frontend unit tests and lint
- [ ] 6.3 Manually verify the dashboard and CSV against seeded campaign data
- [ ] 6.4 Run `openspec validate add-campaign-metrics-dashboard --strict`
