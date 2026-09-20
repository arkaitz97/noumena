## 1. Schema and migration

- [ ] 1.1 Add `campaigns`, `campaign_assets`, and nullable `comments.asset_id` DDL plus indexes to `internal/db/schema.sql`
- [ ] 1.2 Add the next versioned migration in `internal/db/migrations.go` creating the tables and the `comments.asset_id` column/index idempotently
- [ ] 1.3 Implement the opportunistic backfill of `comments.asset_id` from stored assets by channel + video id
- [ ] 1.4 Add migration tests for fresh install, upgrade, re-run, and backfill
- [ ] 1.5 Add DB tests for `campaigns` workspace cascade and `campaign_assets` cascade

## 2. Database stores

- [ ] 2.1 Implement `internal/db/campaigns.go`: campaign CRUD, attach/detach assets, list campaign assets
- [ ] 2.2 Implement `internal/db/campaign_report.go`: summary aggregates, per-creator comparison, unassociated-comment count
- [ ] 2.3 Define the `CampaignStore` interface and embed it in `DBQuery` in `internal/db/db.go`
- [ ] 2.4 Add store tests for aggregate math on seeded data and for small-sample suppression

## 3. Ingestion association

- [ ] 3.1 Populate `comments.asset_id` during ingestion where the video id resolves to a known asset
- [ ] 3.2 Add a worker test asserting new comments are associated

## 4. Campaign API

- [ ] 4.1 Implement `internal/api/campaign_handlers.go`: `POST/GET/PATCH/DELETE /api/campaigns`, `GET /api/campaigns/{id}`
- [ ] 4.2 Implement asset attach/detach: `POST /api/campaigns/{id}/assets`, `DELETE /api/campaigns/{id}/assets/{asset_id}`
- [ ] 4.3 Implement `GET /api/campaigns/{id}/summary` returning the aggregate contract and the window used
- [ ] 4.4 Implement `GET /api/campaigns/{id}/creators` returning per-creator components with suppression
- [ ] 4.5 Implement `POST /api/campaigns/{id}/export` streaming CSV/JSON via `internal/export`
- [ ] 4.6 Enforce workspace scoping and roles (mutations require `admin`; reads require `viewer`+)
- [ ] 4.7 Mount the `/api/campaigns` group behind `CAMPAIGN_API_ENABLED` in `cmd/api/main.go`; 503 when disabled
- [ ] 4.8 Add `CAMPAIGN_API_ENABLED` to `internal/config/config.go` and `.env.example`
- [ ] 4.9 Add handler tests: CRUD, attach/detach, summary shape, comparison, export, cross-workspace 403, disabled flag

## 5. Billing entitlements

- [ ] 5.1 Add campaign entitlement fields to the plan catalog and `LimitsFor`
- [ ] 5.2 Count campaign comment processing against the workspace comment quota; keep overage comment-count based
- [ ] 5.3 Enforce the campaign entitlement on campaign creation with a clear upgrade error
- [ ] 5.4 Add billing tests for entitlement enforcement and quota accounting

## 6. Frontend

- [ ] 6.1 Create `frontend/src/services/campaigns.ts` and `frontend/src/stores/campaigns.ts`
- [ ] 6.2 Create `CampaignsView.vue` (list + create) and `CampaignDetailView.vue` (assets, summary, creators, export)
- [ ] 6.3 Add a campaign nav entry gated by the flag
- [ ] 6.4 Add Vitest tests for campaign creation, attach assets, and summary rendering

## 7. Lifecycle and privacy

- [ ] 7.1 Include `campaigns` and `campaign_assets` in the GDPR export for the owning user
- [ ] 7.2 Confirm hard-delete on workspace/account deletion and add a test
- [ ] 7.3 Verify the report/aggregates contain no suitability verdict or score beyond observed signals

## 8. Verification

- [ ] 8.1 Run `make test`; `gofmt`/lint clean
- [ ] 8.2 Run frontend unit tests and lint
- [ ] 8.3 Manually verify a seeded campaign's aggregate math against a hand computation
- [ ] 8.4 Confirm billing entitlement in Stripe test mode
- [ ] 8.5 Run `openspec validate add-sponsorship-campaign-intelligence --strict`
