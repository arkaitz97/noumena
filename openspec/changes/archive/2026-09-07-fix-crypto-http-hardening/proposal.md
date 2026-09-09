## Why

Hardening findings from the security audit:

1. **Encryption key fallback** — `internal/auth/encryption.go:13`: when `ENCRYPTION_KEY` is unset, code falls back to a hardcoded constant `0123456789abcdef0123456789abcdef`. Every YouTube OAuth token encrypted at rest uses a publicly-known key. Additionally there is no key versioning, so key rotation is impossible without invalidating all stored tokens.
2. **Spoofable rate-limit keying** — `internal/api/rate_limit.go:112` and `clientIP()` (`workspace_handlers.go:405`) trust the first `X-Forwarded-For`/`X-Real-IP` value sent by the client → any caller can reset their bucket by sending a spoofed header.
3. **Limiter runs before auth** — `cmd/api/main.go:223` mounts `RateLimitMiddleware` inside `/api` before `AuthMiddleware`, so `isAuthenticated` is always false and even authed users are throttled at the anonymous rate and keyed by spoofable IP.
4. **HTTP server zero timeouts** — `cmd/api/main.go:481` sets no `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout` → slowloris/hung-connection exposure; no global body-size cap (only the Stripe webhook has 1MiB); no security headers (HSTS, CSP, X-Content-Type-Options, X-Frame-Options).
5. **Query-string token leakage** — chi `middleware.Logger` and `otelhttp` spans log `http.target` including the full query string; `?token=` from email-verify/reset links lands in access logs and OTLP.

## What Changes

- Encryption:
  - Remove the hardcoded fallback key. Startup fails when `ENCRYPTION_KEY` is unset.
  - Add key versioning: ciphertext payload prefixed with a key ID; support multiple keys (current + previous) for decryption, enabling rotation without bulk re-encryption.
  - Add a documented rotation procedure (generate new key, keep old for decrypt, re-encrypt lazily/on next write).
- Rate limiting:
  - Trust only the configured proxy header (`TRUST_PROXY` header name) or `RemoteAddr` when no proxy is configured; never trust client-supplied `X-Forwarded-For` blindly.
  - Mount the limiter AFTER auth so authenticated requests get their per-user bucket; anonymous paths get IP bucket from the trusted source.
- HTTP server hardening (`cmd/api/main.go`): sensible timeouts, global `MaxBytesReader`, security-header middleware.
- Observability: redact query strings in access logs and OTel `http.target` (log path only; set tracer attribute to `http.route`).

## Capabilities

### New Capabilities

- `crypto-key-versioning`: Multiple encryption keys with ciphertext key-ID prefixes and a documented rotation path.

### Modified Capabilities

- `api-rate-limiting`: Client IP is derived from a trusted source; authenticated requests are limited per-user.

## Impact

- `internal/auth/encryption.go`, `internal/api/rate_limit.go`, `internal/api/workspace_handlers.go` (`clientIP`), `internal/api/middleware.go`, `internal/api/otel_middleware.go`, `cmd/api/main.go`, `internal/config/config.go`
- Tests: `internal/auth/auth_test.go`, `internal/api/rate_limit_test.go`, `internal/api/otel_middleware_test.go`
- Ops: `ENCRYPTION_KEY` becomes mandatory; existing DB ciphertext remains decryptable if current key = old key; rotation adds a key ID.
