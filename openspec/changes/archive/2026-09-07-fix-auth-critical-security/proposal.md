## Why

Security audit found three exploitable auth flaws that must be fixed before any production traffic:

1. `POST /auth/google/link` (`internal/api/auth_handlers.go:812` `LinkGoogleConfirm`) is **unauthenticated**. An attacker supplies any victim email + a `sub` of their choosing; the handler sets `google_sub` on the victim's account and issues access + refresh tokens. Full account takeover with only a target email address.
2. `JWTSecret` defaults to the publicly-known constant `dev-only-insecure-secret-change-me` (`internal/config/config.go:138`). `Load()` only overrides when `JWT_SECRET` is set and never validates length/entropy, so production can silently boot with a forgeable signing key.
3. Workspace invite tokens are deterministic: `generateInviteToken()` (`internal/api/workspace_handlers.go:397`) fills a 32-byte buffer with `byte(i)` (0..31), producing the same hex token for every invite ever issued. Invite secrecy is zero; anyone can join any workspace whose email constraint they satisfy.

## What Changes

- `LinkGoogleConfirm` becomes authenticated and proof-bound:
  - Require an authenticated session for the victim account.
  - Require a verified Google ID token whose `sub` equals the `sub` being linked (same validation as the SSO flow: RS256, aud, iss, exp, nonce).
  - Never issue tokens from this endpoint without that proof.
- JWT secret handling:
  - `Load()` fails fast when `JWT_SECRET` is unset, equals the legacy default, or is shorter than 32 bytes.
  - `STRICT_CONFIG` no longer gates secret validation (secret safety is unconditional).
- `generateInviteToken()` uses `crypto/rand.Read` (32 bytes, hex-encoded); existing tokens remain valid until their TTL expires.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `auth`: Google account-linking requires authenticated session + verified Google identity; JWT signing key is mandatory and validated; workspace invites use cryptographically random tokens.

## Impact

- `internal/auth/jwt.go`, `internal/auth/encryption.go` (if key rotation lands in this change, see crypto-hardening change instead — keep scope tight)
- `internal/api/auth_handlers.go`, `internal/api/workspace_handlers.go`
- `internal/config/config.go`
- `cmd/api/main.go` (strictness log message may change)
- Tests: `internal/api/auth_handlers_test.go`, `internal/api/workspace_test.go`, `internal/config/config_test.go`
- Existing valid invite links: unaffected (TTL-based expiry).
