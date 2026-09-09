## Context

Frontend has solid auth plumbing (in-memory access token, HttpOnly refresh, single-flight refresh). The gaps are wiring: role is fetched by a composable nobody calls; an email form posts to a nonexistent route; a reconnect button has no endpoint; export result has no UI; the rate-limit banner is set-but-never-read. Most need a small backend addition + frontend call.

## Goals / Non-Goals

**Goals:**
- Role gates functional (export, members).
- Email change works end-to-end.
- Reconnect works end-to-end.
- Export result discoverable.
- 429 UX functional; logout and re-login preserve state.

**Non-Goals:**
- Not building a full admin console (workspace member management only).
- Not changing the OAuth transport (popup/redirect flow stays).
- Not adding new export formats.

## Decisions

### 1. Role fetch at bootstrap

Call `fetchCurrentWorkspaceRole()` once after login/bootstrap in the auth store or `main.ts` bootstrap, and refresh after workspace switch. Store `role` in a pinia store. `canExport`/`isAdmin` derive from it. Because backend enforcement lands in `fix-workspace-authz-api`, the UI just mirrors the server decision; never use role for security.

### 2. Members admin flow

Add route `/settings/members` + `MembersView.vue` backed by existing workspace endpoints (`GET /workspaces/{id}/members`, invite, change role, remove). "Manage members" button (DashboardLayout.vue:187) navigates to it. Hide/disable behind `isAdmin`. No new backend needed beyond `fix-workspace-authz-api` enforcement.

### 3. PATCH /user/me (email change)

Backend: `PATCH /user/me` accepting `{email}`; validates format, checks uniqueness, sets `email_verified_at=NULL`, bumps `token_version` (forces re-auth, consistent with reset), sends a verification email (existing pipeline) with the new address. New email becomes effective on verification; keep the old email until verified to avoid account lockout. Frontend: wire AccountView form; show "check your inbox" state; disable until verified.

### 4. Reauthorize endpoint

Backend: `POST /channels/{channelId}/reauthorize` → starts the same Google OAuth flow (state cookie, PKCE) but on callback links the new tokens to the *existing* tracking row instead of creating a new channel. Frontend: "Reconnect" button calls it, opens the pre-opened popup, on success clears the expiry banner. Also revoke the old token.

### 5. Pre-opened popup for connect

In `channels.ts` connect flow, `const win = window.open('', 'oauth')` in the click handler, then fetch the URL and `win.location = url`. Matches PlanComparison.vue:62-65. Handles blocked-popup via fallback redirect.

### 6. Export result surface

Backend already has export status via `GET /api/user/export` (used by AccountView). Frontend: after requesting an export, poll the job status (reuse `usePolling`); when `completed`, show a download link + expiry time, and add a "Recent exports" list on the Account page. If `partial`, show the partial-file note.

### 7. 429 banner wiring

In `services/api.ts` interceptor, on 429 parse `Retry-After`, set a reactive `rateLimited` state (store or composable) consumed by `RateLimitBanner.vue`; remove the dead `window.__noumena_rate_limit` global.

### 8. Logout + re-login state

- Refresh-failure redirect: `router.push({ path: '/login', query: { next: route.fullPath } })`; LoginView reads `next` and navigates there post-login.
- Logout: on `POST /auth/logout` failure, keep the user authenticated and show an error (do not silently clear local state); on success clear store + reset. Optionally have the backend clear the refresh cookie regardless of auth state via a dedicated unauthenticated clear endpoint.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Email-change lockout if new email never verified | Keep old email until verification; show resend; clear instructions |
| Reauthorize creates duplicate channels | Callback must match by existing tracking row, not channel handle; server-side dedupe |
| Export polling cost | Stop polling on navigate away / after completion; respect `usePolling` visibility gate |
| Logout endpoint fails on flaky network | Surface error + retry; never fake a logged-out state |
