## MODIFIED Requirements

### Requirement: Storage interface definitions
The `db.DBQuery` interface SHALL be redefined as a composition of narrow sub-interfaces (AuthStore, BillingStore, InsightsStore, ChannelStore, DigestStore, LLMJobStore, WorkspaceStore, LifecycleStore, EmailLogStore, RateLimitStore). Each sub-interface SHALL contain only the methods consumed by its target handler/worker. `DBQuery` SHALL embed all sub-interfaces and SHALL NOT declare additional methods beyond the composition.

#### Scenario: DBQuery embeds sub-interfaces
- **WHEN** `DBQuery` is defined in `internal/db/db.go`
- **THEN** it SHALL be `type DBQuery interface { AuthStore; BillingStore; InsightsStore; ChannelStore; DigestStore; LLMJobStore; WorkspaceStore; LifecycleStore; EmailLogStore; RateLimitStore }`

#### Scenario: Existing implementations satisfy DBQuery
- **WHEN** `SQLiteQuery` or `PostgresQuery` implements all methods of all sub-interfaces
- **THEN** it SHALL implicitly satisfy `DBQuery` via Go structural typing without any code changes to the implementation structs
