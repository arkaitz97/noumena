## Context

`add-content-asset-tracking` persists each video a channel publishes as a `content_assets` row with engagement metrics and sponsored signals. Comments already carry `category`, `topic`, `sentiment`, `actionable_insights`, and `posted_at` from the LLM pipeline, but are keyed only to `platform_channel_id` — there is no association from a comment to the specific video it belongs to. That association was left as an open question in the content-asset change; campaign aggregates make it necessary, because "how did the audience respond to *this sponsored video*" cannot be answered from a channel-level join.

The workspace model already provides the tenancy and role primitives (`workspaces`, `workspace_members`, `channel_grants`), and every campaign-asset read can be constrained to channels the workspace is authorized for. The billing capability already defines plan limits and comment-quota accounting, into which campaign entitlements slot.

Two constraints shape the design:
- **No new model work**: the value is aggregation of existing classification, and re-running the LLM would add cost without new signal.
- **Authorized-representative only**: aggregates are shown to the workspace that is authorized for the underlying channels; no third-party channel data is pulled in.

`(hypothesis)` We assume comment-level signals (objections, questions, sentiment) change a creative decision more than engagement rates do. The pilot's kill criterion tests this directly.

## Goals / Non-Goals

**Goals:**
- Model a sponsored campaign in a workspace and link the content assets it covers.
- Scope comments reliably to campaign assets via a durable association.
- Produce campaign aggregates from existing classification: volume, sentiment trend, category mix, questions, objections, topics, ideas, toxicity share.
- Compare creators side by side for the same campaign.
- Export a client-ready report.
- Govern campaign usage through plan entitlements and the existing comment quota.
- Gate the surface behind a runtime flag.

**Non-Goals:**
- No LLM re-classification, new labels, or new providers.
- No creator outreach, CRM, contracts, e-signature, or payments.
- No paid-media, conversion, or revenue attribution; this measures audience response, not sales.
- No client-facing portal or external seats (deferred roadmap phase).
- No non-YouTube platforms; the model is platform-generic but only YouTube is exercised.
- No brand-suitability score or "safe to sponsor" verdict.

## Decisions

### 1. Campaigns are workspace-scoped, assets link many-to-many
**Decision:** `campaigns(id, workspace_id → workspaces(id) ON DELETE CASCADE, name, brand_name, status CHECK(draft,active,completed), starts_at, ends_at, created_by, created_at, updated_at)` and `campaign_assets(campaign_id → campaigns(id) ON DELETE CASCADE, asset_id → content_assets(id) ON DELETE CASCADE, created_at, PRIMARY KEY(campaign_id, asset_id))`.

Rationale: a campaign spans multiple creators/assets and an asset could appear in more than one campaign (e.g., a recurring partnership), so the join is many-to-many and cascades cleanly. Alternative: a `campaign_id` column on assets — rejected because it forbids reuse and couples asset ingestion to campaign state.

### 2. Durable comment→asset association via a nullable `comments.asset_id`
**Decision:** add `comments.asset_id` (nullable, `REFERENCES content_assets(id) ON DELETE SET NULL`), populated at ingestion where the video id is known, and backfilled opportunistically for existing comments from the channel's stored assets. A partial index supports campaign queries.

Rationale: campaign aggregates need a per-asset comment scope; resolving it by video id at query time would require the video id on every comment anyway, and a JSON join or repeated string matching is slower and fragile. `ON DELETE SET NULL` keeps comment history intact if an asset row is purged. This resolves the open question left in `add-content-asset-tracking`. Alternative: derive asset id per query from a stored video id — kept as fallback for un-backfilled rows, but not the primary path.

### 3. Campaign aggregates are read-time SQL over existing classification
**Decision:** `GET /api/campaigns/{id}/summary` computes, over comments where `asset_id IN campaign_assets` and within the campaign window:
- `comment_volume`, `unique_authors`
- `sentiment` distribution and a per-day trend
- `category_mix`
- `top_questions` (category `question`)
- `objections` (category `constructive_criticism`)
- `top_topics`
- `top_content_ideas` (category `content_idea` and/or `actionable_insights`)
- `toxicity_share` (toxic + spam) as the audience brand-safety signal

No materialization: campaigns are small and reads are request-time, matching the existing insights aggregate approach.

Rationale: reuses the classification already paid for; exposes observed signals rather than an opaque score; keeps the aggregate contract small and testable.

### 4. Cross-creator comparison uses the same component model as demand discovery
**Decision:** `GET /api/campaigns/{id}/creators` returns per-channel rows with engagement totals (from assets), comment volume, sentiment composition, questions, objections, and toxicity share, so creators are comparable on identical definitions.

Rationale: agencies compare creators; using one defined set of components prevents apples-to-oranges reporting. Consistency with the demand-discovery componentization keeps the product's language coherent.

### 5. Report export reuses the existing encoder
**Decision:** `POST /api/campaigns/{id}/export` streams CSV/JSON via `internal/export`, including campaign metadata, aggregate rows, and the per-creator comparison.

Rationale: consistent escaping, streaming, and TTL behavior with existing exports; the deferred roadmap phase adds PDF and the client portal, not a second export engine.

### 6. Billing entitlements are additive
**Decision:** campaign count and campaign comment processing are governed by plan entitlements (new fields on the plan catalog) and counted against the workspace's existing comment quota; overage stays comment-count based.

Rationale: campaigns must not become an unmetered bypass of the comment quota; keeping overage comment-based preserves the existing billing spec contract. Alternative: separate campaign metering — rejected as unnecessary complexity for the first iteration.

### 7. One flag for the surface
**Decision:** `CAMPAIGN_API_ENABLED` gates the `/api/campaigns*` routes and the dashboard nav.

Rationale: consistent with `INSIGHTS_API_ENABLED`, `DEMAND_DISCOVERY_API_ENABLED`, and `CONTENT_ASSET_API_ENABLED`; lets the feature ship dark and be piloted per environment.

## Risks / Trade-offs

- **Backfill coverage of `comments.asset_id`** (dominant technical risk) → older comments may lack an asset link, under-counting campaign aggregates. Mitigation: backfill from stored assets per channel; expose `unassociated_comment_count` in the summary so the gap is visible rather than hidden; fall back to window+channel scope only if linkage is unavailable, clearly labeled.
- **Aggregate correctness across timezones/windows** → all windows are UTC and bounded by `starts_at`/`ends_at`; the summary returns the exact window used.
- **Small samples mislead** → a campaign with few comments can look decisive. Mitigation: return `comment_volume` alongside every share and suppress per-creator rows below a configured minimum comment count with an explicit label.
- **Compliance** → aggregates are restricted to channels the workspace is authorized for, and the report presents observed signals, not a suitability verdict. A task verifies no derived suitability metric is introduced.
- **Billing coupling** → campaign quota changes touch the billing spec; validated in Stripe test mode before enabling.
- **Dependence on content assets** → campaign linking requires `add-content-asset-tracking`; the summary errors clearly if assets or the association are absent.

## Migration Plan

1. Apply after `add-content-asset-tracking` (its `content_assets` table must exist).
2. Versioned migration creates `campaigns`, `campaign_assets`, adds nullable `comments.asset_id` with a partial index, and performs an opportunistic backfill joining stored comments to assets by channel and video id where derivable.
3. Deploy backend with `CAMPAIGN_API_ENABLED=false`; ingestion begins populating `asset_id` for new comments.
4. Enable the flag per environment once aggregates match a manual spot-check.
5. Rollback: additive. Reverting the binary is safe; `comments.asset_id` is nullable and ignored by old code. Tables can remain or be dropped manually.

## Open Questions

- Should `campaigns` support cloning a previous campaign's asset set for recurring partnerships? Deferred; likely a UI convenience, not a schema need.
- What is the minimum comment count below which a per-creator row should be suppressed? Proposed default 20, configurable.
- Should the report include a per-asset row as well as the per-creator rollup? Current design includes both in the export.
