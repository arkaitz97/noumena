## Why

The companion change `add-campaign-attribution-tracking` produces the data needed to evaluate the 14-day guerrilla campaign: attribution rows, product events, and a `v_campaign_cohort` view. That data is only useful if it can be read as a funnel and compared against the campaign's decision matrix (GREEN / YELLOW / RED at day 14). Today there is no operator-facing surface for this: no global admin role exists, there is no reporting endpoint, and nothing renders ACC, TTFI, funnel drop-off, or per-angle breakouts. Without it, the campaign cannot be judged or steered mid-flight.

## What Changes

- Introduce a global admin authorization primitive (the existing `workspace_members.role` is per-workspace, not global): a `users.is_admin` flag, a `RequireAdmin` middleware, an `ADMIN_EMAILS` bootstrap allowlist for operational recovery, and audit logging of admin access.
- Add read-only admin campaign endpoints: a summary/funnel with the decision bucket, a paginated cohort listing, and a CSV export.
- Add an in-app admin dashboard route (guarded by the new role) rendering the funnel, ACC against target, TTFI, high-intent rate, and per-UTM/angle breakouts, with a CSV download.
- Expose the admin flag to the client on the current-user endpoint so the dashboard route and navigation entry can be gated.

This change is read-only over the data produced by `add-campaign-attribution-tracking`; it writes nothing except audit-log entries.

## Capabilities

### New Capabilities
- `admin-authorization`: Global platform administrator role, admin middleware, bootstrap allowlist, and audit logging of admin access.
- `campaign-metrics-dashboard`: Read-only campaign metric computation (funnel, ACC, TTFI, high-intent, per-attribution breakouts, decision bucket), cohort listing, CSV export, and the in-app admin dashboard surface.

### Modified Capabilities
- `storage-layer`: Adds the `users.is_admin` column and its migration.
- `frontend-dashboard`: Adds the admin route guard, the admin dashboard route, and the conditional admin navigation entry.

## Impact

- **Database / migrations**: `internal/db/schema.sql`, `internal/db/migrations.go`, `internal/db/db.go` — migration v23 adds `users.is_admin`; the user column list and scanner are extended.
- **Backend**:
  - `internal/api/admin_middleware.go` — new `RequireAdmin`.
  - `internal/api/admin_campaign_handlers.go` — summary, cohort, and CSV endpoints.
  - `internal/api/auth_handlers.go` — `GET /auth/me` returns `is_admin`.
  - `internal/config/config.go` — `ADMIN_EMAILS` parsing.
  - `cmd/api/main.go` — nested `/api/admin` route group.
  - `internal/db/campaign_report.go` — summary and cohort queries over `v_campaign_cohort`.
- **Frontend**:
  - `frontend/src/router/index.ts` — `requiresAdmin` guard and `/admin/campaign` route.
  - `frontend/src/views/admin/CampaignDashboard.vue`, `frontend/src/services/adminCampaign.ts`, `frontend/src/stores/admin.ts` — dashboard surface.
  - `frontend/src/stores/auth.ts` / layout navigation — conditional admin entry.
- **Security**: admin endpoints expose creator handles and campaign attribution; access is restricted to admins and every access is written to `audit_log`.
- **Dependency**: requires the tables and view from `add-campaign-attribution-tracking`; this change should be applied after it.
