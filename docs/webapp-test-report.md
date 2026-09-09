# WebApp Exploratory & Regression Test Report — Noumena

Date: 2026-09-08
Target: frontend (Vue 3 + Vite) @ http://localhost:5173, backend API @ http://localhost:8080
Harness: puppeteer-core (headless Chromium) against live dev servers. Vitest suite (`npm test`) exists but was not executed — browser-level run only per task scope.

## 1. Summary

| Path type | Total | Pass | Fail | Blocked | Pass ratio |
|---|---|---|---|---|---|
| Happy | 11 | 10 | 0 | 1 | 90.9% |
| Unhappy | 21 | 17 | 4 | 0 | 81.0% |
| **Total** | **32** | **27** | **4** | **1** | **84.4%** |

Blocked item excluded from pass ratio: `/auth/verify-email` token acceptance — env `EMAIL_VERIFICATION_REQUIRED=false` issues no verification token (MockEmail never called); endpoint correctly rejects empty token with 410. Flow not exercisable in this environment. Requires `EMAIL_VERIFICATION_REQUIRED=true` + reachable email sink to unblock.

## 2. Failures (Unhappy paths)

### R-1 — Login with invalid credentials shows wrong error
- Path: Login / invalid credentials
- Steps: fill email `nobody-*@example.com`, password `WrongPass1`, submit.
- Observed: inline error `missing refresh token`, stays on /login. Expected: `Invalid email or password.`
- Cause: backend answers 401 on bad creds; axios response interceptor (`services/api.ts`) treats every 401 as token-expiry, triggers single-flight refresh, refresh fails (`missing refresh token`), that error propagates to the view instead of the original 401 body. Backend message is swallowed.
- Impact: users get misleading internal error; attacker-informative leak of auth architecture.
- Fix direction: interceptor should only refresh on 401 when the request was bearer-authenticated (had `Authorization` header) or when error body indicates expiry; otherwise pass through original error.

### R-2 — Authed hard reload races bootstrap → lands on /login
- Path: AuthLifecycle / hard-reload `/login` and `/dashboard` with valid session.
- Observed: server log shows `POST /auth/refresh 200`, `GET /api/user/me 200`, `GET /api/channels 200`, `GET /api/workspaces/active/role 200` (session fully recovered) — yet page lands on `/login` (respectively `/login?next=/dashboard`).
- Cause: `main.ts` installs router (`app.use(router)`) before `await auth.bootstrap()`. vue-router starts initial navigation immediately on install, so the `beforeEach` guard evaluates while `isAuthenticated` is still false and redirects to `/login`. Bootstrap completes afterward; nothing re-navigates. The `isBootstrapped` flag exists in the auth store (comment: "so the router guard can defensively await it") but the guard never reads it.
- Impact: intermittent session loss on every hard reload of authed pages — depends on which of route resolution vs bootstrap round-trip wins.
- Fix direction: guard should `await` bootstrap completion (or a promise exposed by the store) before evaluating auth state; or call `bootstrap()` before `app.use(router)`.

### R-3 — Unknown route renders silent blank page
- Path: Landing / GET `/definitely-not-a-page`.
- Observed: HTTP 200, empty `#app`, no console error, no redirect, no 404 UI. No catch-all route in `router/index.ts`.
- Impact: dead links and stale bookmarks show blank screens; no SEO/404 affordance.
- Fix direction: add catch-all route → NotFound view or redirect `/`.

### R-4 — (Environment artifact, not app bug) stale `api` binary
- Committed/running `./api` (May 28 build) lacked `GET /api/workspaces/active/role` (source has it, `cmd/api/main.go:536`) → 404 on every bootstrap → `fetchCurrentWorkspaceRole` failed → bootstrap retried and `reset()` → any authed reload logged out. Rebuilt binary from current source; endpoint then returned 200. If a fresh build is deployed without this route being live, R-2 symptoms amplify.

## 3. Console errors & stack traces (Unhappy path runs)

- Only console error observed across 25 page loads (75 occurrences): `Failed to load resource: the server responded with a status of 401 (Unauthorized)` — every page load fires 3× (bootstrap silent-refresh retry loop, `stores/auth.ts` `bootstrap()` 3 attempts with 1s/2s backoff, no session present).
- No unhandled exceptions, no Vue warnings, no 404s after api rebuild, no other console output.
- Finding CN-1 (low): the intentional retry loop still pollutes DevTools console (violates checklist 13.10 "no console errors") and delays first paint ~3s on cold public pages (observed blank body ~3s on first `/forgot-password` load). Mitigate: skip refresh attempts when no refresh cookie present, or log 401s at debug level.

## 4. Happy path evidence

- Landing renders hero ("Stop guessing what your YouTube audience wants. Know for sure."), mobile 375px no horizontal overflow.
- Register 201; duplicate register → 409 + inline `An account with this email already exists.`; weak password disables submit (validation bypass blocked).
- Forgot password: unknown email → generic `If an account exists with this email, we've sent a password reset link...` (no enumeration).
- Reset password: bogus token → `This reset link has expired. Please request a new one.` (verified standalone; suite run flaked on selector timing).
- Route guards: all 7 protected paths redirect unauthenticated users to `/login` with `?next=` preserving target.
- Full lifecycle: register → login → `/onboarding` (verified, no channels).

## 5. Blocked

- `[blocked]` Verify-email token acceptance — requires `EMAIL_VERIFICATION_REQUIRED=true` env + email sink to obtain a real token. State exactly: `POST /auth/verify-email?token=<raw>` with a token issued by the backend; blocked because env auto-verifies and issues no token.

## 6. Notes

- Rate limit: auth endpoints default 10/min/IP; suites exhaust it (429 `rate limit exceeded`, `retry_after`). Run with `RATE_LIMIT_UNAUTH_MAX_PER_MIN` raised. This also demonstrated 429 → UI shows `rate limit exceeded` correctly.
- Pre-existing unrelated Go test compile errors observed (not part of this scope): `internal/lifecycle/export_worker_test.go` (`seedExportUser` arity), `internal/api/digest_handlers_test.go` (unused imports).
- Test script + raw results: `/tmp/opencode/browsertest/test.mjs`, `/tmp/opencode/results.json`; screenshots `/tmp/opencode/shots/`.