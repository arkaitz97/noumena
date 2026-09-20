## Why

Agencies and brands judge sponsored campaigns with vanity metrics — views, likes, and an engagement rate — because that is what influencer platforms (GRIN, CreatorIQ, Aspire) and campaign-measurement tools (InfluenceKit, trackio) expose. None of them show what the sponsored audience actually said: the questions it asked, the objections it raised, whether sentiment was positive, or whether the comment section was toxic. Noumena already classifies every comment, so it can answer those questions — but there is no way to group the sponsored videos into a campaign, and no campaign-scoped view or report.

This change adds sponsorships as a first-class concept: a workspace campaign that links content assets from `add-content-asset-tracking` and aggregates the audience signals of those assets, with a cross-creator comparison and a client-ready export. It is the differentiation bet behind the brand-expansion roadmap (ODI outcomes 2, 4, and 5) and it does not require any new LLM work — it reads what the classification pipeline already produced.

Outcome chain: outcome `brands and agencies judge campaigns by audience response, not vanity metrics` → opportunity `prove a campaign drove real audience interest and extract creative feedback (ODI 12 and 11)` → solution `campaign entity + asset linking + campaign aggregates + cross-creator comparison + report export` → experiment `a pilot brand reviews the campaign dashboard and changes at least one creative decision`.

## What Changes

- Add workspace-scoped **campaigns** (`draft`/`active`/`completed`) with a brand name and a run window, and a **campaign↔asset** join linking sponsored content assets to a campaign.
- Add a reliable **comment→asset association** so campaign aggregates can scope comments to specific sponsored assets (resolves the linkage open question from `add-content-asset-tracking`).
- Add **campaign aggregates** over the campaign's assets: comment volume, sentiment distribution and trend, category mix, top questions, constructive-criticism objections, top topics, actionable content ideas, and the toxicity/spam share (audience brand-safety signal).
- Add **cross-creator comparison**: per-creator (channel) components for the same campaign so an agency can compare how each creator's audience responded.
- Add a **campaign report export** (CSV/JSON) reusing the existing export encoder.
- Add the campaign dashboard: campaign list, campaign editor (assets, window), summary panel, creator comparison, and export.
- Add **billing entitlements** for campaigns so campaign usage is governed by the workspace plan and counted against the existing comment quota.
- Gate the campaign API behind `CAMPAIGN_API_ENABLED`.

## Capabilities

### New Capabilities
- `sponsorship-campaign-intelligence`: Workspace campaigns, campaign-asset linking, campaign-scoped audience aggregates, cross-creator comparison, and report export.

### Modified Capabilities
- `insights-api`: Read endpoints for campaign summary, creator comparison, and export, mounted alongside the existing insights routes with the same auth, rate-limit, and role enforcement.
- `storage-layer`: Adds `campaigns`, `campaign_assets`, and a nullable `comments.asset_id` association with indexes via a versioned migration, mirrored in the baseline schema.
- `frontend-dashboard`: Campaign list, campaign editor, campaign summary, creator comparison, and export.
- `billing`: Campaign entitlements per plan and campaign usage counted against the workspace comment quota.

## Impact

- **Database / migrations**: `internal/db/schema.sql`, `internal/db/migrations.go` — `campaigns`, `campaign_assets`, `comments.asset_id` (nullable, with backfill where the video id is known), and indexes. `campaigns` cascades on workspace deletion; `campaign_assets` cascades on campaign deletion.
- **Backend**:
  - `internal/db/campaigns.go` — new store embedded into `DBQuery` (`internal/db/db.go`).
  - `internal/db/campaign_report.go` — aggregate and comparison queries over campaign assets.
  - `internal/api/campaign_handlers.go` — campaign CRUD, asset attach/detach, summary, creators, export.
  - `internal/ingestion/` — populate `comments.asset_id` for newly ingested comments where the video id is known.
  - `cmd/api/main.go` — flag-gated route group.
  - `internal/config/config.go` + `.env.example` — `CAMPAIGN_API_ENABLED`.
  - `internal/billing/` — campaign entitlement and quota accounting.
- **Frontend**:
  - `frontend/src/views/dashboard/CampaignsView.vue`, `CampaignDetailView.vue`, `frontend/src/services/campaigns.ts`, `frontend/src/stores/campaigns.ts`.
- **Lifecycle / privacy**: campaigns and campaign assets are workspace-scoped; included in GDPR export for the owning user; campaign data is never exposed outside the workspace.
- **Compliance**: aggregates are computed only over assets from channels the workspace is authorized for; the report shows observed signals and not a suitability verdict.
- **Dependency**: requires `add-content-asset-tracking`. Enriching creator comparison with demand already exists in `add-audience-demand-discovery` (optional).
- **Non-goals**: no creator outreach, contracts, or payments; no paid-ads or conversion attribution; no non-YouTube platforms; no client-facing portal (deferred roadmap phase); no LLM changes.
