## Why

`DBQuery` is a 130+ method god interface that every handler, worker, and test depends on. This violates the Interface Segregation Principle: each consumer uses 1–27 methods but must depend on the entire set. Consequences:
- Adding a method to one domain (e.g. billing) forces recompilation of unrelated packages (auth, LLM, ingestion).
- Test doubles (`MockQuery`) must implement 130+ stubs even when a handler only needs 8 methods.
- Change blur: it's unclear which methods belong to which capability, making the codebase harder to navigate.

## What Changes

- **Split `DBQuery` into ~10 narrow, per-consumer interfaces** (e.g. `AuthStore`, `BillingStore`, `InsightsStore`, `ChannelStore`, `DigestStore`, `LLMJobStore`, `WorkspaceStore`, `LifecycleStore`, `EmailLogStore`, `RateLimitStore`).
- **Keep `DBQuery` as a composition** of all sub-interfaces for backward compatibility — existing code that passes `DBQuery` still works.
- **Update handler struct fields** from `db.DBQuery` to the narrow interface they actually need (e.g. `AuthHandler.Queries` becomes `AuthStore`).
- **Refactor `MockQuery`** so tests can compose only the stubs they need per handler.
- **No schema changes, no migration, no API changes.** Pure internal refactor.

## Capabilities

### New Capabilities
- `dbquery-segregation`: Split the monolithic DBQuery interface into per-consumer narrow interfaces following ISP. Defines the interface boundaries, composition pattern, and migration strategy.

### Modified Capabilities
- `storage-layer`: The storage interface definitions change (DBQuery becomes a composition of sub-interfaces). No behavioral change.

## Impact

- **Go packages affected**: `internal/db`, `internal/api`, `internal/auth`, `internal/billing`, `internal/llm`, `internal/ingestion`, `internal/lifecycle`, `cmd/api`
- **Test files affected**: Every `*_test.go` that uses `MockQuery` (~15 files)
- **Breaking changes**: None — `DBQuery` remains as a super-interface; callers can migrate incrementally
- **Risk**: Medium — touch-points are many but each change is mechanical (type assertion or field-type swap)
