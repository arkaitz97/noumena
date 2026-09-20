## Why

Workspace invitations cannot currently be accepted. The invite email links to `GET /invites/accept?token=...`, which is mounted on the anonymous router group with no `AuthMiddleware`, so `UserIDKey` is never populated and the handler always redirects to `/register?invite=...`. The SPA has no route for `/invites/accept` (its catch-all redirects to `/`), no code consumes `?invite=`, and there is no authenticated accept endpoint. Every invite therefore dead-ends, leaving workspace collaboration unusable despite the multi-seat plans already built.

The frontend uses an in-memory Bearer access token plus an HttpOnly refresh cookie scoped to `POST /auth/refresh`. A top-level browser navigation carries no identity, so the accept step must happen from the authenticated SPA over XHR rather than on the link GET.

## What Changes

- Change the invite email link to a public SPA deep link: `${AppBaseURL}/invite?token=...`.
- Add a public SPA route `/invite` that holds the pending token, hands off to login/signup via the existing `?next=` mechanism when unauthenticated, and completes acceptance once authenticated.
- Add an authenticated endpoint `POST /api/invites/accept` that accepts `{token}`, binds the invitation to the caller's account email, admits the member through the existing atomic seat-guarded insert, consumes the invite, sets the active workspace, and is idempotent for existing members.
- Keep `GET /invites/accept?token=...` as an unauthenticated redirect shim to the SPA deep link so previously sent emails keep working.
- Add an unauthenticated, rate-limited `GET /api/invites/preview?token=...` returning `{workspace_name, email, role}` so the SPA can show which workspace is being joined and prefill signup; the token remains the capability and the preview must not leak account existence.
- Define deterministic error semantics: wrong signed-in account → 403 (invite not consumed); expired/consumed token → 410; seat cap reached → 409; already a member → 200 idempotent.
- Add frontend service functions and store wiring for accept/preview, persisting the pending token in `sessionStorage` so the verify-email and OAuth round-trips do not lose it.

**BREAKING**: The invite email link target changes from `/invites/accept` to `/invite`; the old server route becomes a 302 shim, so existing emails remain functional.

## Capabilities

### New Capabilities
- `frontend-invite-acceptance`: SPA `/invite` route, pending-token persistence, unauthenticated handoff to login/signup, authenticated accept/preview calls, and user-facing success/error states.

### Modified Capabilities
- `multi-seat-workspaces`: Reworks the **Accept Invite** requirement from an unauthenticated GET flow to an email-bound, authenticated `POST /api/invites/accept`, and specifies the redirect shim, preview, idempotency, and error semantics.

## Impact

- **Frontend**: `frontend/src/router/index.ts` (new `/invite` route), new `frontend/src/views/auth/AcceptInviteView.vue`, `frontend/src/services/workspaces.ts` (`acceptInvite`, `previewInvite`), possibly `stores/workspaceRole.ts`/`stores/auth.ts` for post-accept active-workspace refresh.
- **Backend**: `cmd/api/main.go` (register `POST /api/invites/accept` in the authenticated `/api` group; register `GET /api/invites/preview`; repoint the anonymous `/invites/accept` route to the SPA redirect), `internal/api/workspace_handlers.go` (`AcceptInviteJSON`, `InvitePreview`, `AcceptInvite` shim), invite link construction.
- **DB/API**: reuses `WorkspaceInvite`, `GetWorkspaceInviteByToken`, `AddWorkspaceMemberAtomic` (seat-atomic), `ConsumeWorkspaceInvite`, `SetUserActiveWorkspace`; no schema change.
- **Security**: invite token remains single-use with a 7-day TTL; acceptance is authorized by authenticated account email matching the invited email; preview is rate-limited and returns no account-existence signal.
- **Tests**: backend handler tests for accept/preview/shim; frontend unit tests for the route guard/handoff and service; optional Playwright journey for the full invite flow.
