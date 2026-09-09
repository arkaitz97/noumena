## Context

`internal/auth/encryption.go` uses AES-256-GCM with a fresh random nonce (good) but keys off `ENCRYPTION_KEY` with a hardcoded fallback. `rate_limit.go` builds a memory token bucket keyed by IP from an untrusted header. `main.go`'s `http.Server` has zero timeouts and mounts rate limiting before auth.

## Goals / Non-Goals

**Goals:**
- No key material may ever come from source code.
- Rotatable encryption without bulk data migration.
- Rate limiting keyed by a source the client cannot spoof.
- HTTP server with timeouts, body caps, and security headers.
- No auth tokens in logs or traces.

**Non-Goals:**
- Not switching encryption algorithm (AES-256-GCM retained).
- Not building distributed rate limiting (single-instance memory limiter stays).
- Not adding WAF/proxy layers (that is infra-level).

## Decisions

### 1. Key rotation via key-ID prefix

Store ciphertext as `v1:<keyID>:<nonce><ciphertext>`. Config holds `ENCRYPTION_KEY` (current) and optional `ENCRYPTION_KEY_PREVIOUS`. Encrypt always with current; decrypt tries current then previous by matching the prefix ID. This lets operators rotate: set new key as current + old as previous, tokens decrypt fine, and re-encrypt happens lazily on next write.

| Option | Rejected because |
|---|---|
| Re-encrypt all rows in a migration job | Complex, risky, and unnecessary; lazy re-encryption is safe with dual-key decrypt |
| Store key ID in a separate DB column | Couples crypto to schema; prefix is self-describing |

### 2. Trusted client IP

Default: trust `RemoteAddr` only. If `TRUST_PROXY_HEADER` is set (e.g. behind Coolify/reverse proxy), read that single header, take the first value, and validate it parses as an IP. Never accept `X-Forwarded-For` from the client directly when it is also presented at the proxy. Document required proxy config.

### 3. Limiter ordering

Move `RateLimitMiddleware` after `AuthMiddleware` for authenticated routes (per-user bucket), and apply a separate anonymous limiter (trusted-IP bucket) to unauthenticated paths. Ensure `RateLimitEnabled` default becomes true for auth endpoints (login/register/verify/reset) regardless of global flag — see also config default for `RateLimitEnabled` (currently false).

### 4. HTTP server timeouts & headers

- `ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`, `WriteTimeout: 60s`, `IdleTimeout: 120s`.
- Wrap handlers with `http.MaxBytesReader` (e.g. 2 MiB global; webhook keeps its own cap).
- Middleware emitting `Strict-Transport-Security` (when HTTPS), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, and a base CSP for the SPA.

### 5. Query-string redaction

Custom request logger that logs `URL.Path` only. In `otel_middleware.go`, set the `http.target` attribute to the route pattern (`http.route`) instead of the raw target so query strings never reach OTLP.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Forcing ENCRYPTION_KEY breaks environments without it | Clear startup error; `.env.example` documents generation; old ciphertext unaffected when new key == old key |
| Wrong proxy trust config causes IP mis-keying (all users same IP) | Document reverse-proxy setup; tests for header parsing; observability metric for distinct bucket counts |
| Tighter timeouts kill long-running LLM/export HTTP calls | Export/LLM use their own http.Client timeouts; server write timeout only affects the API surface (SSE none) |
| CSP blocks inline scripts/styles in Vue build | Test SPA after enabling; use nonce/hash if needed; header configurable via env |
