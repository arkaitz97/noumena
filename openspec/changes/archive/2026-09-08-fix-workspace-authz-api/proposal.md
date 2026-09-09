## Why

Multi-tenancy and role enforcement are spec'd but never wired, and several API-layer defects surfaced in the audit:

1. **Workspace roles unenforced** — `internal/api/workspace_middleware.go:17,47,73` (`WorkspaceMiddleware`, `RequireWorkspaceRole`, `WithWorkspaceID`) are defined but **never mounted** in `cmd/api/main.go`. Admin/analyst/viewer roles are dead; any member can connect/remove channels, star comments, and export regardless of role. Only workspace-member CRUD calls `requireAdmin` (`workspace_handlers.go:386`).
2. **Workspace isolation + shared-channel model unimplemented** — `user_channel_tracking.workspace_id` (`schema.sql:156`) is never written; no channel-grant table exists; feed/insights are scoped by user only, so there is no cross-workspace isolation and no way to share a colleague's channel inside a workspace.
3. **GET /workspaces/{id} broken** — `cmd/api/main.go:420` maps the single-workspace route to `ListWorkspaces`, which ignores `{id}` and returns the full list.
4. **Unscoped comment star** — `internal/api/comment_handlers.go:65` + `internal/db/comments.go:37` and `comments_search.go:44` star comments by ID without checking the user can see them (write-only today, but route-order dependent and a real access hole).
5. **Insights `days` unbounded** — `internal/api/insights_handlers.go:331,358,397` only check `>0`; huge values overflow `time.Duration`.
6. **Channel-limit race** — `channel_handlers.go:174,528` read-then-connect check lets parallel connects exceed plan cap.
7. **Backfill dead code** — `cmd/api/main.go:239` `BackfillEnqueuer` never assigned; `UpsertPlatformChannel` (`channel_mgmt.go:42-59`) returns always-false `inserted` on real SQLite (`RETURNING id, changes()`), so backfill enqueue never fires in production (mock-only test coverage).
8. **Duplicate routes** — `/comments/feed` and `/comments/{id}/star` registered twice; chi last-wins makes the legacy handlers dead-but-shadowed and ordering-dependent.

## What Changes

- Mount workspace middleware and enforce roles on channel/comment/insights/export routes: `viewer` = read-only, `analyst` = read + export, `admin` = write/management. Server-side, not just UI.
- Implement workspace scoping: populate `tracking.workspace_id` at connect; add a channel-grant table (or reuse tracking) so workspace members see shared channels; scope feed/insights queries by workspace membership.
- Fix `GET /workspaces/{id}` to return a single workspace.
- Gate all star operations with an access-control join.
- Clamp `days` to `{7,30,90}`.
- Enforce plan channel cap atomically inside the tracking INSERT transaction.
- Wire a real `BackfillEnqueuer`; fix `UpsertPlatformChannel` inserted-detection (`RETURNING (xmax=0)` or select-before-insert) so backfill enqueues correctly.
- Remove duplicate route registrations.

## Capabilities

### New Capabilities

- `channel-sharing`: Workspace members can be granted access to a colleague's channel within the workspace, and shared-channel comments/digests respect grants.

### Modified Capabilities

- `multi-seat-workspaces`: Role-based access control is enforced server-side on all data routes; channel access is workspace-scoped.

## Impact

- `cmd/api/main.go`, `internal/api/workspace_middleware.go`, `internal/api/channel_handlers.go`, `internal/api/comment_handlers.go`, `internal/api/insights_handlers.go`, `internal/db/channel_mgmt.go`, `internal/db/comments.go`, `internal/db/comments_search.go`, `internal/db/comments_feed.go`, `internal/db/workspace.go`, `internal/db/workspace_models.go`
- Schema: new grant table + `workspace_id` population (migration)
- Frontend: role-aware UI already exists in part (`useWorkspaceRole`) — needs wiring (see `fix-frontend-plumbing`)
- Tests: add coverage for role enforcement, workspace scoping, star access control, channel-cap atomicity
