## Context

`DBQuery` in `internal/db/db.go` is a 130+ method interface covering every data access pattern in the application: user CRUD, auth tokens, channels, billing, workspaces, LLM jobs, digests, exports, rate limiting, and email logging. Every handler, worker, and test depends on it.

The codebase already has precedent for narrow interfaces — `billing.QuotaCronStore`, `billing.TrialReminderStore`, `billing.OverageInvoicingStore`, and `email.EmailLogStore` each define 2–4 methods. These exist because the billing package can't import `db` (import cycle), so they define their own narrow contracts. The problem is that the *handlers* in `api/` still hold the full `DBQuery` even though each uses a small subset.

## Goals / Non-Goals

**Goals:**
- Define ~10 narrow interfaces that express what each handler/worker actually needs.
- Keep `DBQuery` as a composition of all sub-interfaces so existing code compiles unchanged.
- Update handler struct fields to use narrow interfaces (e.g. `AuthHandler.Queries` becomes `AuthStore`).
- Refactor `MockQuery` so tests compose only the stubs relevant to the handler under test.
- Zero behavioral changes — pure type-level refactor.

**Non-Goals:**
- Splitting the SQLite/Postgres implementation files (those stay as-is).
- Changing SQL queries or adding new database methods.
- Migrating to a different ORM or query builder.
- Changing any API endpoints or wire protocols.

## Decisions

### 1. Interface-per-consumer, not interface-per-domain

**Decision**: Define interfaces based on what each *consumer* (handler/worker) uses, not by database table or domain concept.

**Rationale**: A domain-based split (UserStore, ChannelStore) would still leave handlers needing 3–4 interfaces each. A consumer-based split means each handler field is exactly one interface. The downside is method overlap across interfaces (e.g. `GetUserByID` appears in both `AuthStore` and `BillingStore`), but this is acceptable because the underlying implementation satisfies all interfaces simultaneously.

**Alternative considered**: Domain-based interfaces (UserStore, ChannelStore, BillingStore). Rejected because it doesn't reduce handler dependency count — `AuthHandler` would need UserStore + TokenStore + EmailVerificationStore.

### 2. DBQuery stays as super-interface

**Decision**: `DBQuery` becomes `type DBQuery interface { AuthStore; BillingStore; InsightsStore; ... }`.

**Rationale**: Backward compatible. Code that passes `DBQuery` to a function expecting `AuthStore` works implicitly (embedding). No migration wave needed. Handlers can adopt narrow interfaces incrementally.

**Alternative considered**: Delete `DBQuery` entirely. Rejected because it would force a monolithic migration of every function signature.

### 3. Narrow interfaces live in `db/` package, not handler packages

**Decision**: All sub-interfaces are declared in `internal/db/db.go` (or split into per-file in `internal/db/`).

**Rationale**: Avoids import cycles. Handler packages already import `db`; if interfaces lived in handler packages, `db` couldn't reference them. The `db` package is the natural home for data-access contracts.

### 4. MockQuery stays as one struct, satisfies all interfaces

**Decision**: Keep `MockQuery` as a single struct with all methods. Tests pass `MockQuery` to handlers expecting narrow interfaces — Go's structural typing handles the rest.

**Alternative considered**: Per-handler mock structs. Rejected because it multiplies test boilerplate. The existing `MockQuery` already works; splitting it adds no value.

### 5. Interface naming convention: `<Consumer>Store`

**Decision**: Use `AuthStore`, `BillingStore`, `InsightsStore`, `ChannelStore`, `DigestStore`, `LLMJobStore`, `WorkspaceStore`, `LifecycleStore`, `EmailLogStore`, `RateLimitStore`.

**Rationale**: "Store" is the established convention in this codebase (`QuotaCronStore`, `TrialReminderStore`). The prefix identifies the consumer.

## Risks / Trade-offs

- **[Method duplication across interfaces]** → Acceptable. `GetUserByID` will appear in multiple interfaces. The implementation satisfies all of them. This is the cost of ISP.
- **[Large diff]** → Mitigate by doing one interface at a time. Each step is independently compilable and testable.
- **[Interface drift over time]** → Add compile-time assertions (`var _ AuthStore = (DBQuery)(nil)`) to catch drift.
- **[Test refactor scope]** → `MockQuery` stays as-is; tests don't need changes unless they explicitly type-assert to `DBQuery`.
