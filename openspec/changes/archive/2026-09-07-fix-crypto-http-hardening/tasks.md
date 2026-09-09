## 1. Encryption key safety

- [x] 1.1 Remove hardcoded fallback key; `Load()` fails when `ENCRYPTION_KEY` unset
- [x] 1.2 Add key versioning: encrypt writes `v1:<keyID>:<nonce><ct>`, decrypt matches prefix
- [x] 1.3 Add optional `ENCRYPTION_KEY_PREVIOUS` for dual-key decrypt during rotation
- [x] 1.4 Document rotation procedure in README/docs
- [x] 1.5 Tests: decrypt with current key; decrypt with previous key; tampered prefix fails; unset key errors

## 2. Trusted-IP rate limiting

- [x] 2.1 Rework `clientIP()`: default `RemoteAddr`, optional `TRUST_PROXY_HEADER` single header, validate IP parse
- [x] 2.2 Reject spoofable multi-value `X-Forwarded-For` handling
- [x] 2.3 Move limiter after `AuthMiddleware`; per-user bucket for authed, trusted-IP bucket for anonymous
- [x] 2.4 Default `RateLimitEnabled` on for auth endpoints
- [x] 2.5 Update `rate_limit_test.go`: spoofed header ignored; per-user keying; anonymous path works
- [x] 2.6 Document proxy trust config

## 3. HTTP server hardening

- [x] 3.1 Set ReadHeaderTimeout/ReadTimeout/WriteTimeout/IdleTimeout in main.go
- [x] 3.2 Add global `MaxBytesReader` middleware (2 MiB default)
- [x] 3.3 Add security-headers middleware (HSTS, nosniff, frame-deny, referrer, CSP)
- [x] 3.4 Verify webhook keeps its own body cap

## 4. Query-string redaction

- [x] 4.1 Replace chi Logger with path-only logger
- [x] 4.2 In otel_middleware set `http.target` to route pattern / strip query
- [x] 4.3 Test: `?token=` never appears in logs or span attributes

## 5. Verify

- [x] 5.1 `go vet ./...`, `go build ./...`, full `go test ./...` pass
- [x] 5.2 Manual: boot without ENCRYPTION_KEY fails fast
- [x] 5.3 Manual: authed request gets per-user rate bucket; spoofed XFF does not reset bucket
- [x] 5.4 Security headers present on sample responses; SPA loads under CSP
