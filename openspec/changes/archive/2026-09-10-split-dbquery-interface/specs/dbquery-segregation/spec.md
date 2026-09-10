## ADDED Requirements

### Requirement: Narrow per-consumer interfaces
The `db` package SHALL define narrow interfaces for each handler/worker consumer. Each interface SHALL contain only the methods that consumer actually calls. The following interfaces SHALL be defined:
- `AuthStore` — user CRUD, auth tokens, email verification, password reset, Google SSO
- `BillingStore` — subscription updates, Stripe events, overage, channel count
- `InsightsStore` — comment feeds, insights, sentiment, exports
- `ChannelStore` — channel listing, tracking, OAuth state, platform channels
- `DigestStore` — weekly digest CRUD, eligible comments, digest status
- `LLMJobStore` — LLM job enqueue, process, mark completion/failure
- `WorkspaceStore` — workspace CRUD, members, invites, channel grants
- `LifecycleStore` — account deletion, GDPR export (already exists, embedded)
- `EmailLogStore` — email send log, idempotency check (already exists, embedded)
- `RateLimitStore` — rate limit increment, status, cleanup

`InsightsQuery` SHALL also be defined as the full surface of the insights handlers (feed queries, search/thread/bulk mutations, aggregates, exports). It SHALL be declared separately and SHALL NOT be embedded in `DBQuery`, because its methods are implemented only by `*SQLiteQuery`; embedding them would force every `DBQuery` consumer and `MockQuery` to implement them.

#### Scenario: Interface compilation
- **WHEN** the `db` package is compiled
- **THEN** each listed interface SHALL be a valid Go interface type with no duplicate method signatures

### Requirement: DBQuery as composition
`DBQuery` SHALL be defined as an interface embedding all sub-interfaces. It SHALL NOT declare any additional methods beyond those inherited from its embedded interfaces.

#### Scenario: Backward compatibility
- **WHEN** existing code passes a `DBQuery` value to a function accepting `AuthStore`
- **THEN** the code SHALL compile without changes (Go implicit interface satisfaction)

### Requirement: Handler field types narrow
Each handler struct in `internal/api/` SHALL declare its `Queries` field as the narrow interface it actually uses, not `DBQuery`. The mapping is:
- `AuthHandler.Queries` → `AuthStore`
- `BillingHandler.Queries` → `BillingStore`
- `InsightsHandler.Queries` → `InsightsQuery` (separate interface; see the narrow-interfaces requirement)

#### Scenario: AuthHandler receives AuthStore
- **WHEN** `AuthHandler` is constructed with a `Queries` field
- **THEN** the field type SHALL be `db.AuthStore`, not `db.DBQuery`

### Requirement: Compile-time assertions
Each sub-interface file SHALL include a compile-time assertion that `DBQuery` satisfies the interface:
```go
var _ AuthStore = (DBQuery)(nil)
```

#### Scenario: Drift detection
- **WHEN** a method is added to `DBQuery` that belongs to an existing sub-interface
- **THEN** the compile-time assertion SHALL fail if the method is not added to the sub-interface

### Requirement: Worker interface alignment
Workers in `internal/billing/`, `internal/auth/`, and `internal/llm/` SHALL accept narrow interfaces instead of `DBQuery` where they already define local interfaces. Specifically:
- `billing.QuotaCronStore` methods SHALL be a subset of `db.BillingStore` (or remain separate if import cycle prevents alignment)
- `auth.StartUnverifiedAccountCleanup` SHALL accept `db.AuthStore` instead of `db.DBQuery`

#### Scenario: Auth cleanup accepts narrow type
- **WHEN** `StartUnverifiedAccountCleanup` is called with a `db.AuthStore` value
- **THEN** it SHALL compile and function correctly
