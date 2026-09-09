## 1. Fix LinkGoogleConfirm account takeover

- [x] 1.1 Add authenticated-session requirement: reject when `UserIDKey` absent or empty
- [x] 1.2 Verify caller session user matches `req.Email` (fetch by session userID, compare normalized email)
- [x] 1.3 Verify Google ID token (RS256, aud, iss, exp) and require verified `sub == req.Sub`
- [x] 1.4 On any verification failure return 401/403 with no account mutation and no token issuance
- [x] 1.5 Update frontend link-confirmation flow to send the full ID token to `POST /auth/google/link`
- [x] 1.6 Add tests: unauthenticated call rejected; authenticated wrong-sub rejected; expired token rejected; legit flow issues tokens

## 2. Enforce mandatory, validated JWT secret

- [x] 2.1 `Load()` fails when `JWT_SECRET` unset
- [x] 2.2 `Load()` fails when secret equals legacy default or placeholder values
- [x] 2.3 `Load()` fails when secret < 32 bytes
- [x] 2.4 Remove secret-safety from STRICT_CONFIG gate (unconditional)
- [x] 2.5 Update `cmd/api/main.go` startup log to reflect enforced secret
- [x] 2.6 Update `.env.example` comment for JWT_SECRET generation
- [x] 2.7 Add config tests: unset / short / placeholder all error; valid 32-byte passes

## 3. Make invite tokens random

- [x] 3.1 `generateInviteToken()` uses `crypto/rand.Read`, returns error on failure
- [x] 3.2 Store token hashed (sha256) if currently raw; keep column, no data migration
- [x] 3.3 Propagate rand-error handling at call site (log + 500)
- [x] 3.4 Add test asserting two generated tokens differ and are not the legacy constant
- [x] 3.5 Verify existing invites still validate (backward compat)

## 4. Verify end-to-end

- [x] 4.1 `go build ./...` and `go vet ./...` clean
- [x] 4.2 `go test ./internal/api/... ./internal/auth/... ./internal/config/...` pass
- [x] 4.3 Manual: unauthenticated link attempt rejected; valid Google link succeeds
- [x] 4.4 Manual: boot without JWT_SECRET fails fast with clear error
