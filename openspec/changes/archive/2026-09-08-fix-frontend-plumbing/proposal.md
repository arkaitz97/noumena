## Why

Frontend audit found several flows that are broken or dead because backend/plumbing was never wired:

1. **Role never fetched** — `frontend/src/composables/useWorkspaceRole.ts:8` `fetchCurrentWorkspaceRole()` is never called → role stays `viewer` → Export button is always inert (`InsightsFeed.vue:147`), "Manage members" always hidden, and there is no members route at all.
2. **Email change broken** — `frontend/src/services/account.ts:29` calls `PATCH /user/me`, but no such backend route exists (only `GET /user/me`). The email form in AccountView 404s.
3. **Token-expiry "Reconnect" is a dead button** — `frontend/src/components/ChannelCard.vue:56-62` tells users to "Reconnect to resume tracking" but no reconnect endpoint/flow exists; tracking silently stops.
4. **Connect popup can be blocked** — `frontend/src/stores/channels.ts:100-111` awaits the OAuth URL network call before `window.open`, violating the "must open synchronously" contract (PlanComparison pre-opens `about:blank` correctly).
5. **Export has no result surface** — async export only toasts "we'll prepare your file" (`feed.ts:187-189`); nothing tells the user where the file is or when it's ready.
6. **Rate-limit banner dead** — `DashboardLayout.vue:86` sets `window.__noumena_rate_limit` that nothing reads; 429s fall to generic toasts.
7. **Login redirect loses destination** — `services/api.ts:146` refresh-failure redirect drops the `next` query param.
8. **Logout can silently fail to log out** — `stores/auth.ts:108` resets local state even if `POST /auth/logout` fails, leaving the HttpOnly refresh cookie valid → user "logged out" re-authenticates on next load.

## What Changes

- Call `fetchCurrentWorkspaceRole()` at app bootstrap and after login; role gates actually work.
- Build the members admin flow: `/settings/members` route + view wired to existing workspace-member endpoints; "Manage members" button navigates there (backend role enforcement from `fix-workspace-authz-api`).
- Implement backend `PATCH /user/me` (email change with re-verification; optional with change 1 hardening) and wire the AccountView form.
- Add a per-channel reauthorize endpoint (same OAuth flow, link new token to existing channel) + wire the "Reconnect" button.
- Pre-open the connect popup synchronously before the URL fetch (same pattern as PlanComparison).
- Surface export progress/result: poll export job status, show link + expiry when ready, add an export-history entry point.
- Wire 429 handling: API interceptor fires the rate-limit banner with countdown/Retry-After.
- Preserve `next` through re-login; make logout server-confirmed (clear cookie or treat failure as not-logged-out with a retry).

## Capabilities

### New Capabilities

- `channel-reauthorize`: Users can re-link an expired/revoked YouTube token to an existing channel from the UI.

### Modified Capabilities

- `frontend-dashboard`: Role-based UI (export, members) is live; export results are surfaced; rate-limit and reconnect flows work; email change and re-login preserve state.

## Impact

- `frontend/src/composables/useWorkspaceRole.ts`, `frontend/src/main.ts` / bootstrap, `frontend/src/router/index.ts`, `frontend/src/views/settings/*`, `frontend/src/views/dashboard/*`, `frontend/src/components/ChannelCard.vue`, `frontend/src/components/RateLimitBanner.vue`, `frontend/src/stores/channels.ts`, `frontend/src/stores/feed.ts`, `frontend/src/services/api.ts`, `frontend/src/stores/auth.ts`, `frontend/src/services/account.ts`
- Backend: `internal/api/account_handlers.go` (`PATCH /user/me`), `internal/api/channel_handlers.go` (reauthorize), `internal/api/insights_handlers.go` or new export-status route
- Tests: frontend store/view specs for role fetch, export status, reconnect, logout
