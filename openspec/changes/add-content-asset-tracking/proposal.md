## Why

Noumena ingests comments per channel but has no first-class representation of the video (or, on other platforms, post) those comments belong to. `comments` has no asset foreign key, and the ingestion worker discovers videos from the uploads playlist only transiently — it never persists them (`openspec/specs/ingestion/spec.md`, quota-efficient video discovery). As a result the product cannot answer asset-level questions ("how did viewers respond to this specific sponsored video?") and cannot compute campaign-scoped aggregates, because there is nothing to scope a campaign to.

This change adds a `content_assets` entity and persists the videos the worker already discovers, plus an engagement-metric refresh and deterministic sponsored-content signals (`#ad`, `#sponsored`, "paid partnership with", "sponsored by"). It is the foundation for `add-sponsorship-campaign-intelligence`, which links assets into campaigns and aggregates audience response.

Outcome chain: outcome `brands know how audiences responded to each sponsored asset` → opportunity `prerequisite for campaign scoring (ODI scores 12/11/10)` → solution `persisted content assets + sponsored signals + metric refresh` → experiment `assets populate for connected channels and sponsored flags are correct on fixtures`.

## What Changes

- Persist every video the ingestion worker discovers from a channel's uploads playlist as a `content_asset`, deduplicated by `(platform_channel_id, platform_asset_id)`, linked to its channel.
- Add a lightweight **metrics-refresh worker** that periodically refreshes views, likes, and comment counts for assets of actively tracked channels, within the existing YouTube quota budget.
- Add **deterministic sponsored-content signals**: scan asset title and description for ad/sponsorship markers and store the matched signals; expose an `is_sponsored` boolean and the raw signals list. No LLM, no scoring.
- Add read endpoints: list assets for a tracked channel and fetch a single asset.
- Add an asset panel to the channel detail view.
- Gate the asset API behind `CONTENT_ASSET_API_ENABLED`.

## Capabilities

### New Capabilities
- `content-asset-tracking`: Persisted per-channel content assets with engagement metrics and deterministic sponsored-content signals, plus their read surface and refresh worker.

### Modified Capabilities
- `ingestion`: The worker persists discovered videos as content assets; a metrics-refresh pass updates asset statistics within the YouTube quota budget.
- `storage-layer`: Adds the `content_assets` table and indexes via a versioned migration, mirrored in the baseline schema.
- `frontend-dashboard`: Asset list and detail surface on the channel view.

## Impact

- **Database / migrations**: `internal/db/schema.sql`, `internal/db/migrations.go` — `content_assets` table with indexes; `ON DELETE CASCADE` from `platform_channels`.
- **Backend**:
  - `internal/db/content_assets.go` — new store embedded into `DBQuery` (`internal/db/db.go`).
  - `internal/ingestion/` — persist discovered videos; add the metrics-refresh pass and sponsored-signal detection.
  - `internal/api/asset_handlers.go` — list/detail routes.
  - `cmd/api/main.go` — flag-gated route group.
  - `internal/config/config.go` + `.env.example` — `CONTENT_ASSET_API_ENABLED`, refresh cadence.
- **Frontend**: `frontend/src/views/dashboard/` asset panel, `frontend/src/services/assets.ts`, `frontend/src/stores/assets.ts`.
- **Lifecycle / privacy**: assets cascade with the platform channel; included in GDPR export for the tracking user.
- **Compliance**: assets and metrics are read from the authorized channel's own uploads playlist and `videos.list` under the tracker's OAuth token — no public third-party reads. Sponsored detection is a literal metadata signal, not a suitability score; whether it needs to be declared under YouTube's derived-metrics policy is an explicit task.
- **Non-goals**: no campaign grouping (Change C), no LLM classification of assets, no non-YouTube platforms, no asset-level payment or deal tracking.
- **Dependency**: none beyond the existing channel model. `add-sponsorship-campaign-intelligence` depends on this change.
