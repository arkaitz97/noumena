## Context

The frontend authenticates with an in-memory Bearer access token and an HttpOnly refresh cookie used only by `POST /auth/refresh`. The invite email currently links to `GET /invites/accept?token=...`, mounted on the anonymous router group with no `AuthMiddleware`; `UserIDKey` is therefore never set and the handler always redirects to `/register?invite=...`. The SPA has no `/invite` route (its catch-all redirects to `/`) and no service consumes `?invite=`, so invites cannot be accepted. The `multi-seat-workspaces` spec already describes an Accept Invite flow, but it assumes identity is available on a top-level GET, which the auth model does not support.

A top-level browser navigation to the backend carries no access token and the refresh cookie is scoped to the refresh call, so the membership mutation must originate from the authenticated SPA over XHR. This design keeps the email token as the capability and binds acceptance to account email.

Stakeholders: invited users (accept flow), workspace Admins (invites/seat limits), and operators (email link compatibility, rate limiting).

## Goals / Non-Goals

**Goals:**
- Make invite acceptance work end-to-end for both new and existing users.
- Keep the emailed link usable and idempotent, with no membership mutation on navigation.
- Preserve the email-binding authorization model (only the account owning the invited email can join).
- Reuse the existing atomic, seat-guarded admission so concurrent accepts cannot exceed the plan cap.
- Keep previously sent invite emails functional via a redirect shim.
- Avoid leaking account existence through the public preview.

**Non-Goals:**
- Invite revocation / expiry-management UI, or changing invite creation and rate limiting.
- Resending or editing invites.
- Email template redesign (only the link target changes).
- Any database schema change (reuses `workspace_invites` and `workspace_members`).
- Switching the auth model away from in-memory Bearer tokens.

## Decisions

### D1: Accept via authenticated `POST /api/invites/accept`, not on the link GET
The SPA holds the pending token and submits it through the authenticated API client once a session exists.
- **Why**: the browser sends no identity on navigation; the mutation must be XHR so the Bearer token and `RequireVerifiedEmail` guard apply.
- **Alternatives**: (a) cookie-authenticated server-side accept on GET — rejected: requires widening the refresh cookie path and adding CSRF protection to a state-changing GET; (b) token-only anonymous accept — rejected: any forwarded link would join, losing email binding; (c) auto-accept on login — rejected: implicit, and cannot cleanly surface wrong-account/seat-full outcomes.

### D2: Route placement
- `POST /api/invites/accept` → authenticated `/api` group, inheriting `AuthMiddlewareWithQueries`, `RequireVerifiedEmail`, `RateLimitMiddleware`, and `WithWorkspaceID`.
- `GET /api/invites/preview` → anonymous group with `AuthRateLimitMiddleware`.
- `GET /invites/accept?token=` → 302 to `${AppBaseURL}/invite?token=`, no membership change.
- **Why**: placement gives correct guards for free and keeps the anonymous surface to a read-only preview plus a redirect.
- **Alternative**: a dedicated new middleware stack — rejected as unnecessary duplication.

### D3: Authorization is authenticated email match
Acceptance SHALL require the authenticated account email to equal the invite email (case-insensitive), and the `/api` group's verified-email guard SHALL already prove control of that address. On mismatch, return 403 and do not consume the invite.
- **Why**: the token is a capability delivered to an address; the account proves ownership of that address. This is the minimal sufficient control.

### D4: Pending token persisted in `sessionStorage`
The SPA stashes the raw token under a single key, stripped from the URL once read, and clears it on success or terminal failure.
- **Why**: it must survive full-page login/OAuth/verify round-trips, stay out of authenticated request URLs (Referer leakage), and not be shared across tabs.
- **Alternatives**: in-memory (lost on OAuth redirect), `localStorage` (outlives the flow, cross-tab leakage), cookie (server-sent, CSRF).

### D5: Reuse atomic seat-guarded admission and make acceptance idempotent
Use `AddWorkspaceMemberAtomic` with the resolved seat limit as the authority; on `ErrSeatLimit` return 409 without consuming the invite. If the user is already a member, return 200, still set the workspace active, and consume the invite.
- **Why**: closes the check-then-act race (already fixed elsewhere) and avoids duplicate-member errors on retries.

### D6: Public preview returns minimal, non-enumerating data
`GET /api/invites/preview?token=` returns `{workspace_name, email, role}` for valid invites and a generic 410 otherwise; no endpoint reports whether an account exists for the email.
- **Why**: lets the SPA show "Join <workspace> as <role>" and prefill signup without an enumeration oracle. The invited email is already known to the link holder.
- **Alternative**: embed signed workspace metadata in the link — rejected: extra key management for no benefit.

### D7: Legacy link compatibility via redirect shim
Keep `GET /invites/accept` as a 302 to the SPA deep link so already-sent emails work unchanged.
- **Why**: no coordination between deploy and inbox delivery; additive and cheap.

## Risks / Trade-offs

- **Unverified new users blocked by `RequireVerifiedEmail`** → the router already diverts unverified sessions to `/verify-email`; `sessionStorage` preserves the token and the invite view retries on return. Explicitly test this path.
- **Token leakage via Referer** → strip the token from the URL after reading, and set `Referrer-Policy: no-referrer` for the invite route. Token is single-use with a 7-day TTL.
- **OAuth popup context** → if the Google popup opens a separate tab, `sessionStorage` is per-tab and the token could be lost. Mitigate by keeping the pending token in the opener tab and completing acceptance after the popup hands control back; if the popup is cross-tab, fall back to the top-level redirect flow. Verify against the existing OAuth popup implementation.
- **Preview enumeration** → rate-limited, constant generic 410, and no account-existence signal; the invited email is inherent to the link.
- **Concurrent accepts** → seat-atomic insert admits exactly one at the cap; the loser gets 409 and the invite remains consumable by a valid seat.
- **Idempotency double-consume** → consumption is a guarded `UPDATE ... WHERE consumed_at IS NULL`; a replay after success returns the already-member 200 path.

## Migration Plan

1. Backend: add `POST /api/invites/accept`, `GET /api/invites/preview`, repoint `GET /invites/accept` to the SPA redirect, and change the invite email link target. No schema change. Deployable independently; old emails immediately flow through the shim.
2. Frontend: add the `/invite` route, accept/preview service calls, pending-token persistence, and error states.
3. Rollback: revert the email link target and frontend route; the shim and new endpoints are additive and can remain. No data migration to undo.

## Open Questions

- Should acceptance require a verified email (recommended, consistent with the `/api` guard), or accept any session whose email matches? 
- Should the already-member path always re-activate the invited workspace, or preserve the user's current active workspace? (recommended: activate, matching "accept invite" intent.)
