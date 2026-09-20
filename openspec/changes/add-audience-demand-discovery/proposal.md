## Why

Brands and marketing agencies cannot see what creator audiences are already asking for. The signal exists — viewers ask "where can I get X?", "does this work for Y?", and "please review Z" in comments — but it is buried across hundreds of channels and is invisible to every incumbent: influencer platforms (CreatorIQ, GRIN, Aspire) sell roster operations and vanity engagement, and social-listening tools (Brand24, Hootsuite, Octolens) track web mentions, not creator-audience demand. Noumena already classifies every ingested comment into `question`, `content_idea`, `collaboration_ask`, and `topic`, so the raw material for "which creators' audiences want what we sell, and why?" is already in the database.

Current state: comments are only visible through a creator's own tracked channels (`GET /api/comments/feed` is scoped to the caller's workspace channels), there is no way to define a product/category target, no cross-corpus ranking, and no consent signal for whether a creator's data may be used for brand-facing discovery. This change establishes the demand-discovery capability and the creator-opt-in consent that makes it compliant under the YouTube API Services Developer Policies.

Key compliance premise `(source: https://developers.google.com/youtube/terms/developer-policies)`: aggregated API data may be viewable only to the content owner or an authorized representative. Demand discovery therefore operates exclusively over channels whose creator has opted in — never over third-party channels read via a public API key or scraped.

## What Changes

- Add per-creator discovery consent on tracked channels: a `channel_discovery_consent` table (default `opted_in = 0`), a channel consent endpoint, and a consent toggle in channel settings. A channel is discoverable only while at least one active tracker has opted in.
- Add workspace-scoped **demand watches**: a named target (product/category/competitor) defined by a set of terms, with a lifecycle (`active`/`paused`).
- Add a **discovery query**: match opted-in channels' comments against a watch's terms across high-signal categories, compute per-channel demand intensity, engagement quality, and brand safety, and return a ranked creator shortlist.
- Add an **evidence view** per creator: the matching comments (with category, sentiment, topic, posted-at) that justify the rank, respecting opt-in.
- Add a **shortlist export** (CSV/JSON) reusing the existing export encoder.
- Gate the entire API behind `DEMAND_DISCOVERY_API_ENABLED`.
- Add the demand-discovery dashboard view (watch list, watch editor, ranked creators, evidence panel, export).

Outcome chain: outcome `brands choose better-matched creators` → opportunity `find creators whose audiences already ask for my category (ODI 15)` → solution `demand watches + ranked shortlist over the opted-in corpus` → experiment `pilot agency exports and acts on ≥1 shortlist within 30 days`.

## Capabilities

### New Capabilities
- `audience-demand-discovery`: Workspace demand watches (term sets + lifecycle), the ranked creator-shortlist query over the creator-opt-in corpus, the evidence view, and shortlist export.

### Modified Capabilities
- `channel-management`: Per-creator discovery-consent state on tracked channels and the consent endpoint.
- `insights-api`: Read-only demand-watch and discovery endpoints, gated by a feature flag.
- `storage-layer`: `demand_watches`, `demand_watch_terms`, and `channel_discovery_consent` tables plus indexes via a versioned migration, mirrored in the baseline schema.
- `frontend-dashboard`: Demand-discovery view and the channel consent toggle.

## Impact

- **Database / migrations**: `internal/db/schema.sql`, `internal/db/migrations.go` — three new tables with indexes; both watch tables cascade on workspace deletion; consent cascades on tracking-row deletion (and therefore on account deletion).
- **Backend**:
  - `internal/db/demand_discovery.go` — new store embedded into `DBQuery` (`internal/db/db.go`).
  - `internal/api/demand_handlers.go` — watch CRUD, creators ranking, evidence, export.
  - `internal/api/channel_handlers.go` — `PATCH /api/channels/{id}/discovery-consent`.
  - `cmd/api/main.go` — flag-gated route group.
  - `internal/config/config.go` + `.env.example` — `DEMAND_DISCOVERY_API_ENABLED`.
- **Frontend**:
  - `frontend/src/views/dashboard/DemandDiscoveryView.vue`, `frontend/src/services/demand.ts`, `frontend/src/stores/demand.ts`.
  - `frontend/src/views/settings/` — discovery consent toggle per channel.
- **Lifecycle / privacy**: new tables included in GDPR export; consent rows deleted with the tracking row; discovery never reads comments from non-opted-in channels.
- **Non-goals**: no LLM changes (reuses existing classification), no new platform ingestion, no creator contact data or outreach, no public-data discovery, no campaign/asset tracking (that is `add-content-asset-tracking`).
- **Dependency**: none; runs on the existing corpus and channel model.
