## Context

`add-campaign-attribution-tracking` persists `user_attribution`, `product_events`, and the `v_campaign_cohort` view. This change consumes that data through an operator-facing dashboard and introduces the authorization primitive it requires.

The repository today has only workspace-scoped roles (`workspace_members.role`: admin/analyst/viewer) checked by `RequireWorkspaceRole` in `internal/api/workspace_middleware.go`. There is no global role. `RequireVerifiedEmail` in `internal/api/middleware.go` is the closest analogue for a global middleware and is used as the structural template. The API rate limiter and `audit_log` table already exist.

The dashboard is read-only, admin-only, and low-traffic; the campaign runs for 14 days with N of 50-100 users.

## Goals / Non-Goals

**Goals:**
- Add a global admin role with a middleware guard and safe bootstrap.
- Expose a read-only campaign summary, cohort listing, and CSV export.
- Render an in-app admin dashboard against the framework's decision matrix.
- Audit every admin access.

**Non-Goals:**
- No campaign metric rewriting; the view and queries come from the companion change.
- No Grafana/Prometheus/BI integration. Campaign metrics stay in SQLite; operational metrics stay in OTel.
- No admin write operations or user management (granting/revoking admin beyond the documented bootstrap is out of scope for the first iteration).
- No real-time streaming dashboard; queries are request-time reads.

## Decisions

### 1. Global admin as a `users.is_admin` column, not a new table or JWT claim
**Decision:** Migration v23 adds `users.is_admin BOOLEAN NOT NULL DEFAULT 0`. `RequireAdmin` loads the user and checks the column, with an `ADMIN_EMAILS` allowlist as a bootstrap/recovery path.

Rationale: an existing-users table is the natural home for a global flag; a dedicated `platform_admins` table adds a join for no benefit at this scale. Embedding admin in the JWT is rejected because it drifts from the DB and interacts with token-version invalidation. Alternatives considered: (a) workspace-admin-reuse — wrong scope, a workspace admin is not a platform admin; (b) env allowlist only — no durable per-user grant and every request compares emails, but it is retained as the bootstrap path so the first admin exists without a data migration.

### 2. Bootstrap via `ADMIN_EMAILS`, durable grants via the column
**Decision:** `RequireAdmin` authorizes when `users.is_admin = 1` OR the user's email is in the configured `ADMIN_EMAILS` allowlist. A small `tools/grant_admin` utility flips the column for durable grants.

Rationale: the very first admin cannot be created by a data migration (the email is deployment-specific). Env bootstrap is the standard cold-start pattern and is also the recovery path if the column is mis-set. The column then allows granting without redeploying config.

### 3. Request-time queries over `v_campaign_cohort`, no materialization
**Decision:** Summary, cohort, and CSV endpoints query the view at request time.

Rationale: N is tiny, the view is indexed through its underlying tables, and there is nothing to precompute at this scale. Materializing would add a worker and staleness for no gain.

### 4. Decision bucket computed server-side, targets encoded once
**Decision:** The summary endpoint returns the GREEN/YELLOW/RED bucket using the framework thresholds (`>= 40`, `15-39`, `< 15` ACC) and the diagnostic signals, so the frontend does not duplicate the rules.

Rationale: single source of truth for the decision logic; the frontend renders whatever bucket the server returns. Thresholds live as named constants for easy adjustment.

### 5. CSV generation reuses the existing export encoder
**Decision:** The CSV endpoint streams rows using the `internal/export` CSV encoder rather than ad-hoc `encoding/csv` calls.

Rationale: consistent escaping and streaming behavior with the product's other exports.

### 6. Frontend route and nav gated on a server-provided flag
**Decision:** `GET /auth/me` returns `is_admin`; the router adds a `requiresAdmin` guard and the navigation entry renders only when `is_admin` is true.

Rationale: prevents a non-admin from reaching a confusing 403 page, and keeps the nav clean. The server guard is the real boundary; the client guard is UX only.

### 7. Every admin access is audited
**Decision:** Each admin campaign endpoint writes an `audit_log` row with action `admin_campaign_view` and the requesting user id.

Rationale: the dashboard exposes creator handles and campaign attribution; access must be traceable. The `audit_log` table and its indexes already exist.

## Risks / Trade-offs

- **Exposure of creator PII and attribution** → Admin-only, server-guarded, audit-logged; cohort endpoint paginates. No anonymous `landing_view` data is user-identifiable.
- **`is_admin` misconfiguration** → The `ADMIN_EMAILS` allowlist is the recovery path; the flag is a single column that can be corrected directly.
- **Adds a global role surface that did not exist** → Scope is deliberately limited to read-only campaign endpoints; no general admin console is introduced.
- **Dashboard relies on a sibling change** → This change must be applied after `add-campaign-attribution-tracking`; the view must exist or summary queries fail. The endpoints return an explicit error if the view is absent.
- **Client-side guard is bypassable** → Accepted: the client guard is UX only; the authoritative check is `RequireAdmin` on the server.
- **Threshold drift between framework doc and code** → Thresholds are named constants with the framework's decision matrix in comments; a unit test asserts the bucket boundaries.

## Migration Plan

1. Apply after `add-campaign-attribution-tracking` (its migration v22 and view must exist).
2. Migration v23 adds `users.is_admin` with a default of 0; existing users are unaffected.
3. Deploy backend with `ADMIN_EMAILS` set to the operator's email; verify `/auth/me` returns `is_admin`.
4. Frontend route/nav activates for admin users only.
5. Rollback: revert the binary; the column and view are additive and harmless. Audit rows remain as historical record.

## Open Questions

- Confirm `ADMIN_EMAILS` env bootstrap is acceptable versus requiring a `tools/grant_admin` step before any admin exists. Default assumption: env bootstrap is enabled.
- Should the cohort CSV be generated server-side (current design) or exported client-side from the paginated cohort endpoint? Server-side is chosen for consistent encoding and large-N safety.
