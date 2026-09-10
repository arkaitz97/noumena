## 1. Define sub-interfaces in `internal/db/`

- [ ] 1.1 Create `internal/db/auth_store.go` — define `AuthStore` interface with auth-related methods (user CRUD, tokens, email verification, password reset, Google SSO, login failures). Add compile-time assertion `var _ AuthStore = (DBQuery)(nil)`.
- [ ] 1.2 Create `internal/db/billing_store.go` — define `BillingStore` interface (Stripe events, subscription updates, overage, channel count, quota reset, past-due grace). Add compile-time assertion.
- [ ] 1.3 Create `internal/db/insights_store.go` — define `InsightsStore` interface (comment feeds, insights, sentiment, categories, toxicity, exports). Add compile-time assertion.
- [ ] 1.4 Create `internal/db/channel_store.go` — define `ChannelStore` interface (channel listing, tracking, OAuth state, platform channels, sync, token management). Add compile-time assertion.
- [ ] 1.5 Create `internal/db/digest_store.go` — define `DigestStore` interface (weekly digest CRUD, eligible comments, digest status, stale/deferred). Add compile-time assertion.
- [ ] 1.6 Create `internal/db/llm_job_store.go` — define `LLMJobStore` interface (enqueue, process, mark completion/failure, recover stale). Add compile-time assertion.
- [ ] 1.7 Create `internal/db/workspace_store.go` — define `WorkspaceStore` interface (workspace CRUD, members, invites, channel grants, active workspace). Add compile-time assertion.

## 2. Redefine `DBQuery` as composition

- [ ] 2.1 In `internal/db/db.go`, replace the monolithic `DBQuery` body with `type DBQuery interface { AuthStore; BillingStore; InsightsStore; ChannelStore; DigestStore; LLMJobStore; WorkspaceStore; LifecycleStore; EmailLogStore; RateLimitStore }`.
- [ ] 2.2 Verify `go build ./...` compiles — all existing implementations satisfy `DBQuery` implicitly.
- [ ] 2.3 Run `go test ./...` — no behavioral changes, all tests pass.

## 3. Narrow handler field types

- [ ] 3.1 Change `AuthHandler.Queries` from `db.DBQuery` to `db.AuthStore` in `internal/api/auth_handlers.go`. Update `setupAuthRouter` and `setupGoogleAuthRouter` test helpers.
- [ ] 3.2 Change `BillingHandler.Queries` from `db.DBQuery` to `db.BillingStore` in `internal/api/billing_handlers.go`. Update `setupBillingRouter` test helper.
- [ ] 3.3 Change `InsightsHandler.Queries` from `db.DBQuery` to `db.InsightsStore` in `internal/api/insights_handlers.go`. Update test helpers.
- [ ] 3.4 Change `StartTokenExpiryNotifier` and `runTokenExpiryScan` to accept `db.ChannelStore` instead of `db.DBQuery` in `internal/api/jobs.go`.
- [ ] 3.5 Change `StartChannelPurgeJob` to accept `db.ChannelStore` instead of `db.DBQuery`.
- [ ] 3.6 Verify `go build ./...` and `go test ./internal/api/...` pass.

## 4. Narrow worker types

- [ ] 4.1 Change `auth.StartUnverifiedAccountCleanup` to accept `db.AuthStore` instead of `db.DBQuery`.
- [ ] 4.2 Change `llm.Processor` to accept `db.LLMJobStore` (or keep `DBQuery` if sweep method requires broader interface).
- [ ] 4.3 Verify `go build ./...` and `go test ./internal/auth/... ./internal/llm/...` pass.

## 5. Update `cmd/api/main.go` wiring

- [ ] 5.1 Update handler construction to pass narrow interfaces (e.g. `AuthHandler{Queries: queries}` — since `queries` is `DBQuery` which satisfies `AuthStore`, no change needed at call sites).
- [ ] 5.2 Verify `go build ./cmd/api/...` passes.

## 6. Final verification

- [ ] 6.1 Run `go vet ./...` — no new warnings.
- [ ] 6.2 Run `go test ./...` — all tests pass.
- [ ] 6.3 Verify `git diff --stat` shows net negative or neutral line count.
