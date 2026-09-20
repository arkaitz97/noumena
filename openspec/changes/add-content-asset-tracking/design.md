## Context

Noumena's ingestion worker resolves a channel's uploads playlist once (cached on `platform_channels`) and walks it with `playlistItems.list`, fetching comment threads per video (`openspec/specs/ingestion/spec.md`, quota-efficient video discovery and resumable incremental sync). Today those video identities are used transiently and discarded — nothing below the channel level is persisted. The dashboard is channel-centric: it can show a channel's comments, sentiment, and topics, but not "which video did this come from, and how did that video perform".

The campaign-intelligence change that follows needs an asset to scope a campaign to, and needs engagement context (views/likes/comments) alongside comment sentiment. Building that on transient discovery would force every campaign read to re-hit the YouTube API; persisting assets once lets campaign aggregation be a pure SQL read.

Constraints:
- All YouTube reads stay within the tracker's OAuth authorization and the existing daily quota budget (`openspec/specs/ingestion/spec.md`, YouTube quota budgets and suppression).
- Comments are already keyed to `platform_channel_id`; assets must link comments to a specific video without a schema rewrite of `comments`.
- YouTube's Developer Policies allow showing channel-owner data to the owner/authorized representatives; sponsored-content detection must remain a literal metadata signal rather than a derived suitability judgment.

`(hypothesis)` We assume asset-level engagement plus comment sentiment is more useful to a brand than comment sentiment alone. This is validated as part of the campaign-intelligence pilot, not here.

## Goals / Non-Goals

**Goals:**
- Persist one row per real video (asset) per channel, deduplicated, with stable identity.
- Refresh views/likes/comment counts on a schedule within the existing quota budget.
- Detect and store deterministic sponsored-content signals from asset metadata.
- Expose a minimal read surface and a channel-view asset panel.
- Keep comments joinable to assets without changing the `comments` primary identity.

**Non-Goals:**
- No campaign grouping or campaign-level aggregates (Change C).
- No LLM or model-based classification of assets.
- No non-YouTube platforms or non-video asset types yet (schema stays platform-generic).
- No creator/deal management, contracts, or payments.
- No audience demographics or brand-suitability scoring.

## Decisions

### 1. A generic `content_assets` table keyed by platform asset id
**Decision:** `content_assets(id, platform_channel_id → platform_channels(id) ON DELETE CASCADE, platform, platform_asset_id, title, description, url, published_at, duration_seconds, view_count, like_count, comment_count, is_sponsored, sponsored_signals JSON, metrics_refreshed_at, created_at, UNIQUE(platform_channel_id, platform_asset_id))`.

Rationale: `platform` is stored even though only YouTube is live, so the table does not need a migration to add platforms later. `UNIQUE(platform_channel_id, platform_asset_id)` makes ingestion idempotent and re-runs safe (`ON CONFLICT DO NOTHING`/upsert), matching the comment dedupe pattern. Alternative: columns on `comments` — rejected because view/like counts are asset-level, not comment-level, and would be duplicated per comment.

### 2. Comments link to assets by video id, resolved at read time
**Decision:** do not add a NOT NULL foreign key on `comments`; join `comments.platform_comment_id` context to the asset through the video id captured during ingestion. Where a comment needs a direct asset link (Change C), resolve it via `(platform_channel_id, video_id)`.

Rationale: `comments` currently stores `platform_comment_id` but not the parent video id explicitly, and rewriting every comment row is risky. The ingestion path knows the video when it inserts comments; the minimal change is to persist the asset as the videos are walked and to associate at that boundary. If a stable direct link proves necessary, a nullable `asset_id` column can be added in a follow-up migration after backfilling from the stored video id. This is called out as an open question rather than assumed.

### 3. Persist during the existing walk; refresh on a separate cadence
**Decision:** the ingestion worker upserts an asset when it first discovers a video during playlist walking. A separate metrics-refresh pass (scheduled per channel, e.g. daily) calls `videos.list(part=statistics)` in batches of up to 50 ids (1 quota unit per call) and updates counts.

Rationale: discovery is already happening and is nearly free; metrics change continuously, so they need a different cadence than comment syncing. Batching 50 ids per unit keeps the cost negligible and inside the existing budget. Alternative: refresh metrics inside every poll — wasteful and unnecessary for slow-moving counts.

### 4. Deterministic sponsored detection, signals stored verbatim
**Decision:** scan `title` and `description` case-insensitively for a configurable marker set (`#ad`, `#sponsored`, `#paidpartnership`, "paid partnership with", "sponsored by", "in partnership with") and store the matched markers in `sponsored_signals`; set `is_sponsored` when any match. No model, no score.

Rationale: deterministic and explainable; the brand/campaign layer (Change C) can combine this with an explicit campaign asset list, so a missed marker is not fatal. Explicitly avoids creating a "brand-suitability score", which the YouTube Developer Policies prohibit. Alternative: an LLM classifier — rejected for cost, opacity, and policy risk at this stage.

### 5. Read surface is minimal and flag-gated
**Decision:** `GET /api/assets?channel_id=` and `GET /api/assets/{id}`, plus `POST /api/assets/{id}/refresh` for an on-demand refresh, gated by `CONTENT_ASSET_API_ENABLED`.

Rationale: Change C consumes assets internally; this change only needs enough surface to verify correctness and give users visibility. The flag lets the schema ship dark, consistent with existing rollout patterns.

### 6. Metrics refresh respects quota suppression
**Decision:** the refresh pass uses the same quota-budget admission and `quotaExceeded` hard-stop as ingestion, and defers cleanly when the budget is spent.

Rationale: metrics are lower priority than comment ingestion; they must never starve it or break the daily budget invariant.

## Risks / Trade-offs

- **Video-id linkage for existing comments** (dominant technical risk) → existing comments predate assets and have no stored video id. Mitigation: store the video id going forward, backfill opportunistically from the channel's playlist on the next walk, and keep the asset↔comment association resolved rather than a hard FK until proven. Called out as an open question.
- **Quota pressure from metrics refresh** → mitigated by batching and by using the existing budget admission; deferred when the budget is spent.
- **Sponsored detection false negatives/positives** → accepted; the campaign layer lets a user explicitly include assets, and `sponsored_signals` shows exactly what matched.
- **Compliance of sponsored detection** → treated as a literal metadata signal; a task confirms whether it must be declared under YouTube's derived-metrics/storage policy.
- **Platform-generic schema drift** → the table is designed platform-agnostically but only YouTube is exercised, so non-YouTube code paths are untested; documented as a known limitation until Change B's multi-platform successor.

## Migration Plan

1. Add a versioned migration creating `content_assets` and indexes (`platform_channel_id`, `published_at`, a partial index on `is_sponsored = 1`, and `metrics_refreshed_at` for the refresh scan). Mirror in `schema.sql`.
2. Deploy backend with `CONTENT_ASSET_API_ENABLED=false`.
3. Asset persistence begins immediately (ingestion upserts); metrics refresh is enabled with the flag.
4. Backfill: existing channels' assets populate on the next ingestion walk; no separate backfill job is required.
5. Rollback: additive. Reverting the binary is safe; the table can remain or be dropped manually.

## Open Questions

- Should `comments` gain a nullable `asset_id` with a backfill from the stored video id, rather than resolving the asset per query? Deferred until Change C's query shape is known.
- What refresh cadence and asset age cutoff balance freshness against quota for very large channels? To be tuned from measured unit spend.
- Should sponsored signals include brand mentions from a configured brand list, or only generic markers? Generic markers only for now; brand lists belong to campaigns.
