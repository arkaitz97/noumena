## 1. Role plumbing

- [x] 1.1 Call `fetchCurrentWorkspaceRole()` at bootstrap + after login + after workspace switch
- [x] 1.2 Store role in pinia store; derive canExport/isAdmin
- [x] 1.3 Export button enabled for analyst+; members button for admin
- [x] 1.4 Test: role resolves after login; UI reflects role

## 2. Members admin flow

- [x] 2.1 Add `/settings/members` route + `MembersView.vue`
- [x] 2.2 Wire invite / change-role / remove to workspace endpoints
- [x] 2.3 "Manage members" button navigates to it
- [x] 2.4 Test: non-admin cannot reach/act (UI + 403)

## 3. Email change (PATCH /user/me)

- [x] 3.1 Backend `PATCH /user/me` with email validation, uniqueness, token_version bump, verification email
- [x] 3.2 Keep old email until verified
- [x] 3.3 Frontend: wire AccountView form; "check your inbox" + resend state
- [x] 3.4 Test: change → verify → new email active; old email restored on failed verify

## 4. Channel reauthorize

- [x] 4.1 Backend `POST /channels/{id}/reauthorize` → OAuth flow linking to existing tracking row
- [x] 4.2 Revoke old token on success
- [x] 4.3 Frontend: "Reconnect" button uses pre-opened popup; clears expiry banner on success
- [x] 4.4 Test: reauthorize updates token on same channel; no duplicate channel

## 5. Connect popup pre-open

- [x] 5.1 Pre-open popup synchronously in click handler (PlanComparison pattern)
- [x] 5.2 Fetch URL then navigate popup; blocked-popup fallback
- [ ] 5.3 Test (e2e/manual): slow network still opens popup

## 6. Export result surface

- [x] 6.1 Poll export job status after request; stop on completion/navigation
- [x] 6.2 Show download link + expiry; partial-export note
- [x] 6.3 Add "Recent exports" list in Account page
- [x] 6.4 Test: request → status → completed → link visible

## 7. Rate-limit banner wiring

- [x] 7.1 429 branch in api.ts interceptor; parse Retry-After
- [x] 7.2 RateLimitBanner consumes reactive state; remove dead global
- [x] 7.3 Test: 429 shows banner with countdown

## 8. Logout + re-login state

- [x] 8.1 Preserve `next` through /login
- [x] 8.2 Logout only clears state after server confirms; error keeps session
- [x] 8.3 Test: failed logout keeps user logged in; success clears

## 9. Verify

- [x] 9.1 `npm run build` + vitest green
- [ ] 9.2 Manual: analyst exports; admin manages members; reconnect works; email change verifies
