## 1. Schema and migration

- [ ] 1.1 Add `demand_watches`, `demand_watch_terms`, and `channel_discovery_consent` DDL plus indexes to `internal/db/schema.sql`
- [ ] 1.2 Add the next versioned migration in `internal/db/migrations.go` creating the three tables and indexes idempotently
- [ ] 1.3 Add migration test coverage in `internal/db/migrations_test.go` for fresh install, upgrade, and re-run (idempotent) paths
- [ ] 1.4 Add a DB test asserting a deleted `user_channel_tracking` row cascades its consent row, and a deleted workspace cascades its watches and terms

## 2. Database stores

- [ ] 2.1 Implement `internal/db/demand_discovery.go`: watch create/list/get/update/delete, term replace, consent get/upsert
- [ ] 2.2 Implement the discovery query: opted-in-channel scope, term match (FTS5), category filter, window filter, componentized per-channel aggregates
- [ ] 2.3 Implement the evidence query: matching comments for one channel within a watch, respecting opt-in
- [ ] 2.4 Define the `DemandDiscoveryStore` interface and embed it in `DBQuery` in `internal/db/db.go`
- [ ] 2.5 Add store tests covering consent exclusion, term matching, window bounds, and component calculations

## 3. Consent API

- [ ] 3.1 Add `PATCH /api/channels/{id}/discovery-consent` accepting `{opted_in}`; upsert the consent row for the caller's tracking row
- [ ] 3.2 Return the current consent state from `GET /api/channels/{id}/status`
- [ ] 3.3 Add handler tests: opt-in, opt-out, non-tracked channel, and unauthenticated request

## 4. Demand-watch and discovery API

- [ ] 4.1 Implement `internal/api/demand_handlers.go`: watch CRUD (`POST/GET/PATCH/DELETE /api/demand/watches`)
- [ ] 4.2 Implement `GET /api/demand/watches/{id}/creators?days=` returning ranked creators with the three score components and a coverage indicator
- [ ] 4.3 Implement `GET /api/demand/watches/{id}/evidence?channel_id=&limit=` returning matching comments
- [ ] 4.4 Implement `POST /api/demand/watches/{id}/export` streaming CSV/JSON via `internal/export`
- [ ] 4.5 Validate inputs: term presence/length/count cap, `days` bounds, watch ownership (403 on foreign workspace)
- [ ] 4.6 Mount the `/api/demand` group behind `DEMAND_DISCOVERY_API_ENABLED` in `cmd/api/main.go`; 503 when disabled
- [ ] 4.7 Add `DEMAND_DISCOVERY_API_ENABLED` to `internal/config/config.go` and `.env.example`
- [ ] 4.8 Add handler tests: watch CRUD, ranking shape, evidence consent boundary, export, disabled flag, invalid input

## 5. Frontend

- [ ] 5.1 Create `frontend/src/services/demand.ts` and `frontend/src/stores/demand.ts`
- [ ] 5.2 Create `frontend/src/views/dashboard/DemandDiscoveryView.vue`: watch list, watch editor (name/terms/window), ranked creator list with score components, evidence panel, export button
- [ ] 5.3 Add a discovery nav entry gated by the API flag / capability
- [ ] 5.4 Add the per-channel discovery-consent toggle in channel settings
- [ ] 5.5 Add Vitest tests for watch editing, ranking rendering, and consent toggle

## 6. Lifecycle and privacy

- [ ] 6.1 Include `demand_watches`, `demand_watch_terms`, and `channel_discovery_consent` in the GDPR export bundle
- [ ] 6.2 Confirm hard-delete removes all three (cascade or explicit) and add a test
- [ ] 6.3 Add a DB test proving discovery never returns comments from a non-opted-in channel

## 7. Verification

- [ ] 7.1 Run `make test`; `gofmt`/lint clean
- [ ] 7.2 Run frontend unit tests and lint
- [ ] 7.3 Manually verify a seeded watch ranks only opted-in channels and the evidence matches the rank
- [ ] 7.4 Run `openspec validate add-audience-demand-discovery --strict`
