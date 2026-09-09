## Context

Current data model: `users` → `workspaces` (membership via `workspace_members`), `user_channel_tracking` (user↔platform_channel, with `workspace_id` column that exists but is never written), comments shared per channel. The API only ever scopes by `uct.user_id`. Workspace middleware exists but is unmounted. This change wires the multi-seat model the specs already describe.

## Goals / Non-Goals

**Goals:**
- Server-side role enforcement on all data routes (viewer/analyst/admin).
- Channels belong to a workspace; workspace members can access channels shared within it.
- Feed/insights/stars/export all scoped through the same access path.
- Backfill pipeline actually runs.

**Non-Goals:**
- Not building workspace-to-workspace public sharing (cross-tenant).
- Not implementing seat billing here (billing change separate).
- Not rewriting the UI (frontend wiring is a separate change).

## Decisions

### 1. Role enforcement model

Roles: `admin` (manage workspace, members, channels), `analyst` (read + export + insights), `viewer` (read only). Enforce via a middleware that resolves `workspace_id` from the request (path param / `X-Workspace-Id` header / JWT default workspace) then checks membership+role. Mount at the `/api` group for workspace-scoped routes. Actions map:
- channel connect/remove, star/unstar moderation, member management, billing → require `admin`
- export/insights/trends → require at least `analyst`
- read feed/comments/digest → any member (`viewer`+)

Fallback policy: if no workspace is resolvable, use the user's `active_workspace_id`; if the user has no membership anywhere, deny.

### 2. Channel ownership in a workspace

At connect time, set `tracking.workspace_id = active_workspace_id` (migration backfills existing trackings to the owner's active workspace). Access rule: a user may read a channel's comments/digests/insights if they track it **or** a grant row links them to a channel in one of their workspaces.

### 3. Grant table (shared channels)

New table `channel_grants(id, workspace_id, channel_id, granted_by, created_at)` with UNIQUE(workspace_id, channel_id). A grant lets every workspace member (min role viewer) see that channel's comments/insights via feed query that joins `channel_grants` OR `user_channel_tracking`. UI for granting lives in the frontend change; backend exposes `POST /workspaces/{id}/channels/{channelId}/grant` + revoke, gated to admin/analyst.

### 4. Single access path for feed/star/export

Refactor `BuildFeedQuery` (`comments_feed.go`) to accept an access predicate: `(uct.user_id = ? AND uct.channel_id = c.id) OR (cg.workspace_id = ? AND cg.channel_id = c.id)`. Reuse the same predicate for star access checks and export filtering. This removes the unscoped star writes and the duplicate legacy handlers (delete legacy `CommentHandler.Feed/Star`; keep scoped insights versions).

### 5. Atomic channel cap enforcement

Move the plan-limit check into the same transaction that inserts the tracking row (INSERT ... SELECT ... WHERE (SELECT count) < plan_cap). On violation, roll back and return 402/409. This removes the read-then-connect race.

### 6. Backfill wiring

- Fix `UpsertPlatformChannel` inserted detection: use `RETURNING id, (xmax = 0)` (true only on insert) or SELECT-before-INSERT; return the flag correctly.
- Assign `BackfillEnqueuer` in `main.go` (real enqueuer that inserts a backfill `ingestion_jobs` row).
- Keep job-queue dedupe so reconnects don't double-backfill.

### 7. days clamp

Shared `ValidRanges = {7,30,90}`; reject or clamp otherwise, consistent with Trends.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Role enforcement breaks existing users who are members of zero workspaces | Onboarding assigns default workspace; fallback deny with clear 403 |
| Grant table grows / permission checks get complex | Single predicate helper; index on (workspace_id, channel_id) |
| Backfill enqueue flood on reconnect | Job-queue dedupe (pending/running) retained |
| Breaking existing API shape for feed/star | Keep response shapes; only access semantics tighten |
