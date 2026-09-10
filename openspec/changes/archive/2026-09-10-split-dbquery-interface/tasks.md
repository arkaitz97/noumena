## 1. Define sub-interfaces in `internal/db/`

- [x] 1.1 Create `internal/db/auth_store.go` — define `AuthStore` interface with auth-related methods (user CRUD, tokens, email verification, password reset, Google SSO, login failures). Add compile-time assertion `var _ AuthStore = (DBQuery)(nil)`.
- [x] 1.2 Create `internal/db/billing_store.go` — define `BillingStore` interface (Stripe events, subscription updates, overage, channel count, quota reset, past-due grace). Add compile-time assertion.
- [x] 1.3 Create `internal/db/insights_store.go` — define `InsightsStore` interface (comment feeds, insights, sentiment, categories, toxicity, exports). Add compile-time assertion.
- [x] 1.4 Create `internal/db/channel_store.go` — define `ChannelStore` interface (channel listing, tracking, OAuth state, platform channels, sync, token management). Add compile-time assertion.
- [x] 1.5 Create `internal/db/digest_store.go` — define `DigestStore` interface (weekly digest CRUD, eligible comments, digest status, stale/deferred). Add compile-time assertion.
- [x] 1.6 Create `internal/db/llm_job_store.go` — define `LLMJobStore` interface (enqueue, process, mark completion/failure, recover stale). Add compile-time assertion.
- [x] 1.7 Create `internal/db/workspace_store.go` — define `WorkspaceStore` interface (workspace CRUD, members, invites, channel grants, active workspace). Add compile-time assertion.

## 2. Redefine `DBQuery` as composition

- [x] 2.1 In `internal/db/db.go`, replace the monolithic `DBQuery` body with `type DBQuery interface { AuthStore; BillingStore; InsightsStore; ChannelStore; DigestStore; LLMJobStore; WorkspaceStore; LifecycleStore; EmailLogStore; RateLimitStore }`.
- [x] 2.2 Verify `go build ./...` compiles — all existing implementations satisfy `DBQuery` implicitly.
- [x] 2.3 Run `go test ./...` — no behavioral changes, all tests pass.

## 3. Narrow handler field types

- [x] 3.1 Change `AuthHandler.Queries` from `db.DBQuery` to `db.AuthStore` in `internal/api/auth_handlers.go`. Update `setupAuthRouter` and `setupGoogleAuthRouter` test helpers.
- [x] 3.2 Change `BillingHandler.Queries` from `db.DBQuery` to `db.BillingStore` in `internal/api/billing_handlers.go`. Update `setupBillingRouter` test helper.
- [x] 3.3 Change `InsightsHandler.Queries` from `*db.SQLiteQuery` to `db.InsightsQuery` in `internal/api/insights_handlers.go` (separate interface; `InsightsStore` is embedded in DBQuery and cannot carry the concrete-only methods without forcing MockQuery to implement them).
- [x] 3.4 Change `StartTokenExpiryNotifier` and `runTokenExpiryScan` to accept `db.ChannelStore` instead of `db.DBQuery` in `internal/api/jobs.go`.
- [x] 3.5 Change `StartChannelPurgeJob` to accept `db.ChannelStore` instead of `db.DBQuery`.
- [x] 3.6 Verify `go build ./...` and `go test ./internal/api/...` pass.

## 4. Narrow worker types

- [x] 4.1 Change `auth.StartUnverifiedAccountCleanup` to accept `db.AuthStore` instead of `db.DBQuery`.
- [x] 4.2 Change `llm.Processor` to accept `db.LLMJobStore` (or keep `DBQuery` if sweep method requires broader interface).
- [x] 4.3 Verify `go build ./...` and `go test ./internal/auth/... ./internal/llm/...` pass.

## 5. Update `cmd/api/main.go` wiring

- [x] 5.1 Update handler construction to pass narrow interfaces (e.g. `AuthHandler{Queries: queries}` — since `queries` is `DBQuery` which satisfies `AuthStore`, no change needed at call sites).
- [x] 5.2 Verify `go build ./cmd/api/...` passes.

## 6. Final verification

- [x] 6.1 Run `go vet ./...` — no new warnings.
- [x] 6.2 Run `go test ./...` — all tests pass.
- [x] 6.3 Verify `git diff --stat` shows net negative or neutral line count. (N/A: a mechanical file-split adds lines. db.go −204, new store files +257.)
