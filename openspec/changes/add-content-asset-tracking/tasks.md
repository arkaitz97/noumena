## 1. Schema and migration

- [ ] 1.1 Add `content_assets` DDL plus indexes to `internal/db/schema.sql`
- [ ] 1.2 Add the next versioned migration in `internal/db/migrations.go` creating the table and indexes idempotently
- [ ] 1.3 Add migration tests for fresh install, upgrade, and re-run
- [ ] 1.4 Add a DB test asserting assets cascade when their `platform_channels` row is deleted

## 2. Database store

- [ ] 2.1 Implement `internal/db/content_assets.go`: upsert asset, get by id, list by channel, update metrics, list due for metric refresh
- [ ] 2.2 Define the `ContentAssetStore` interface and embed it in `DBQuery` in `internal/db/db.go`
- [ ] 2.3 Add store tests covering dedupe on `(platform_channel_id, platform_asset_id)`, metric updates, and sponsored-signal round-trip

## 3. Ingestion persistence

- [ ] 3.1 Persist an asset when the worker discovers a video during the uploads-playlist walk (idempotent upsert)
- [ ] 3.2 Capture and store the video id used for comment association
- [ ] 3.3 Add sponsored-signal detection (configurable markers) over title and description; set `is_sponsored` and `sponsored_signals`
- [ ] 3.4 Add worker tests for asset upsert idempotency and sponsored detection on fixtures

## 4. Metrics refresh

- [ ] 4.1 Implement the metrics-refresh pass: select assets due, batch `videos.list(part=statistics)` up to 50 ids, update counts
- [ ] 4.2 Route refresh calls through the existing quota-budget admission and `quotaExceeded` hard stop
- [ ] 4.3 Schedule the refresh pass on the configured cadence; skip when the budget is spent
- [ ] 4.4 Add tests for batching, budget suppression, and `metrics_refreshed_at` advancement

## 5. API

- [ ] 5.1 Implement `internal/api/asset_handlers.go`: `GET /api/assets?channel_id=`, `GET /api/assets/{id}`, `POST /api/assets/{id}/refresh`
- [ ] 5.2 Enforce workspace access (only channels the caller's workspace can access) and role rules (refresh requires `analyst`+)
- [ ] 5.3 Mount the routes behind `CONTENT_ASSET_API_ENABLED` in `cmd/api/main.go`; 503 when disabled
- [ ] 5.4 Add `CONTENT_ASSET_API_ENABLED` and refresh cadence config to `internal/config/config.go` and `.env.example`
- [ ] 5.5 Add handler tests: list, detail, cross-workspace 403, refresh, disabled flag

## 6. Frontend

- [ ] 6.1 Create `frontend/src/services/assets.ts` and `frontend/src/stores/assets.ts`
- [ ] 6.2 Add an asset panel to the channel detail view listing assets with metrics and a sponsored badge
- [ ] 6.3 Add Vitest tests for asset rendering and sponsored badge display

## 7. Lifecycle and privacy

- [ ] 7.1 Include `content_assets` derived data in the GDPR export for the tracking owner
- [ ] 7.2 Confirm asset hard-delete on channel deletion and add a test

## 8. Verification

- [ ] 8.1 Run `make test`; `gofmt`/lint clean
- [ ] 8.2 Run frontend unit tests and lint
- [ ] 8.3 Manually verify assets populate after an ingestion walk and metrics refresh within budget
- [ ] 8.4 Run `openspec validate add-content-asset-tracking --strict`
