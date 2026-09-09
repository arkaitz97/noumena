## Context

Auth core is otherwise sound (PKCE + state, bcrypt≥12, refresh rotation, httpOnly cookies, in-memory access tokens). The three flaws are isolated hotspots. Two share a root cause: **trusting client-supplied identity claims without cryptographic proof**.

- `LinkGoogleConfirm` accepts `{email, sub}` JSON from an unauthenticated caller. It was intended for a "confirm link from Google" view but never verifies the Google ID token — the `sub` is attacker-arbitrary.
- `generateInviteToken` returns `hex("000102...1f")` — constant across all invocations.
- `config.Default.JWTSecret` ships a literal insecure default; `Load()` (config.go:218) replaces it only if env is set.

## Goals / Non-Goals

**Goals:**
- Remove the unauthenticated account-takeover path.
- Make JWT signing key mandatory, entropy-validated, non-guessable.
- Make invite tokens unpredictable via CSPRNG.

**Non-Goals:**
- Not redesigning the SSO flow.
- Not implementing encryption-key rotation here (separate `fix-crypto-http-hardening` change).
- Not invalidating already-issued valid invites.

## Decisions

### 1. LinkGoogleConfirm requires verified Google ID token

The handler must decode and verify a Google ID token (same path as `google_sso.go`'s `verifyGoogleIDToken`): RS256 against Google JWKS, `aud` matches client id, `iss` allowlisted, `exp` valid. The verified `sub` MUST equal the requested `sub`. Additionally, require an authenticated session (`UserIDKey`) whose user email matches `req.Email`. If any check fails → 401/403, no account mutation.

| Option | Rejected because |
|---|---|
| Keep endpoint open but require OAuth state round-trip | Reintroduces the same trust problem; state alone doesn't prove identity |
| Require only `sub` proof without session | Lets an attacker who controls a victim's Google account (or a stolen ID token) bypass session — session + proof is strictly safer |

### 2. JWT secret validation is unconditional, not behind STRICT_CONFIG

`Load()` must:
- fail if `JWT_SECRET` empty;
- fail if it equals the old default or any documented placeholder (`change-me`, `replace-me`);
- fail if len < 32 bytes;
- (warning only) if entropy is trivially low (e.g., all one character).

STRICT_CONFIG keeps its existing role for *other* config strictness but no longer controls secret safety.

### 3. Invite tokens from crypto/rand

```go
b := make([]byte, 32)
if _, err := rand.Read(b); err != nil { return "", err }
return hex.EncodeToString(b)
```

Existing tokens in `workspace_invites` stay valid until expiry — no migration needed. Store hashed (`sha256`) rather than raw if not already, matching refresh-token convention.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Removing default secret breaks local dev / existing `.env` files | `.env.example` documents required value; dev uses an explicit random value; startup error message names the env var |
| Verified-token requirement breaks the legit link-confirmation UI flow | Frontend flow must pass the full ID token (not just sub) to the endpoint; update `OAuthCallbackView`/link flow accordingly |
| Invite re-hash migration adds churn | Keep token column; only new tokens are random; old tokens expire on their existing schedule |
