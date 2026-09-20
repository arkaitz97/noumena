## 1. Backend API

- [x] 1.1 Add `AcceptInviteJSON` (`POST /api/invites/accept`, body `{token}`): hash the token, load the invite via `GetWorkspaceInviteByToken`, and return 410 for unknown/expired/consumed tokens.
- [x] 1.2 In `AcceptInviteJSON`, authorize by comparing the authenticated user's email to the invite email (case-insensitive); on mismatch return 403 without consuming the invite.
- [x] 1.3 In `AcceptInviteJSON`, handle the already-member case idempotently: return 200, set the workspace active, consume the invite, and do not insert a duplicate `workspace_members` row.
- [x] 1.4 In `AcceptInviteJSON`, admit the member with the resolved seat limit via the atomic `AddWorkspaceMemberAtomic`; map `ErrSeatLimit` to 409 and leave the invite unconsumed.
- [x] 1.5 In `AcceptInviteJSON`, on success consume the invite, set the active workspace, write the `workspace.invite_accepted` audit log, and return 200 with `{workspace_id, workspace_name, role}`.
- [x] 1.6 Add `InvitePreview` (`GET /api/invites/preview?token=`) returning `{workspace_name, email, role}` for valid invites and a generic 410 otherwise, with no account-existence signal.
- [x] 1.7 Resolve the workspace name for accept/preview (add a small query helper if `GetWorkspaceInviteByToken` does not already expose it); no schema change.
- [x] 1.8 Convert `AcceptInvite` (GET `/invites/accept`) into a 302 shim to `${AppBaseURL}/invite?token=<raw>` with no membership mutation.
- [x] 1.9 Change the invite email link target to `${AppBaseURL}/invite?token=<raw>`.
- [x] 1.10 Register `POST /api/invites/accept` in the authenticated `/api` group (inherits auth, `RequireVerifiedEmail`, rate limit, workspace context) and `GET /api/invites/preview` in the anonymous group with `AuthRateLimitMiddleware` in `cmd/api/main.go`.

## 2. Backend Tests

- [x] 2.1 Test existing-user acceptance: 200, `workspace_members` row created, invite consumed, active workspace set.
- [x] 2.2 Test wrong signed-in account: 403 and invite still unconsumed.
- [x] 2.3 Test expired, consumed, and unknown tokens: 410.
- [x] 2.4 Test seat cap reached: 409, invite unconsumed, and concurrent accepts admit exactly one at the cap (run with `-race`).
- [x] 2.5 Test idempotency: accepting twice yields 200 and a single membership row.
- [x] 2.6 Test preview: valid returns workspace name/email/role; invalid returns generic 410; endpoint is rate-limited.
- [x] 2.7 Test the legacy GET shim returns 302 to the SPA URL and performs no mutation.
- [x] 2.8 Test that an unverified session is rejected by `RequireVerifiedEmail` before acceptance.

## 3. Frontend Implementation

- [x] 3.1 Add `acceptInvite(token)` and `previewInvite(token)` to `frontend/src/services/workspaces.ts`.
- [x] 3.2 Add the public `/invite` route to `frontend/src/router/index.ts` with no `requiresAuth`/`requiresVerified`/`requiresOnboarded` meta.
- [x] 3.3 Create `frontend/src/views/auth/AcceptInviteView.vue`: read the token from the query, strip it from the URL, and persist it in `sessionStorage`.
- [x] 3.4 In `AcceptInviteView.vue`, when unauthenticated, route to login/signup with `next=/invite?token=<raw>` while preserving the pending token.
- [x] 3.5 In `AcceptInviteView.vue`, when authenticated, call `acceptInvite`, refresh the workspace list/active workspace, clear the pending token, and navigate to the workspace dashboard or onboarding.
- [x] 3.6 Handle `email_not_verified` by routing to verification while keeping the pending token for retry.
- [x] 3.7 Render distinct error states for 403 (wrong account + switch-account action), 409 (seat limit), and 410 (expired/consumed), clearing the pending token on terminal failures.
- [x] 3.8 Set `Referrer-Policy: no-referrer` for the invite route (meta tag or router hook).

## 4. Frontend Tests

- [x] 4.1 Test that `/invite` is publicly reachable and does not redirect to the catch-all.
- [x] 4.2 Test unauthenticated handoff: token stored in `sessionStorage` and navigation includes `next`.
- [x] 4.3 Test authenticated success: `acceptInvite` called, token cleared, navigation to the workspace.
- [x] 4.4 Test error states 403/409/410 render the expected messaging and token cleanup.
- [x] 4.5 Test that the pending token survives an email-verification round-trip.

## 5. Verification & Docs

- [x] 5.1 Run `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./...`; all pass.
- [x] 5.2 Run `npm test` in `frontend/`; all pass.
- [x] 5.3 Add a Playwright journey for accept-invite (existing user and new user) if the harness supports the auth round-trip.
- [x] 5.4 Update `frontend/MANUAL_TESTING_CHECKLIST.md` with the invite acceptance flow.
