## Context

Noumena is a Go 1.25 + chi backend over embedded SQLite, with a Vue 3 + Vite frontend. Comments are ingested once per `platform_channels` row and shared across trackers; each comment already carries `category`, `topic`, `sentiment`, `actionable_insights`, and `posted_at` from the LLM pipeline (`internal/db/schema.sql:215`). Access is workspace-scoped: `GET /api/comments/feed` only returns comments from channels the caller's workspace can access, resolving through `user_channel_tracking.workspace_id` and `channel_grants` (`openspec/specs/multi-seat-workspaces/spec.md`).

Two facts constrain the design:

1. **Consent is the legal boundary.** Under the YouTube API Services Developer Policies, aggregated API data may only be viewable to the content owner or their authorized representatives, and derived metrics/storage may require an audited quota-extension request. Demand discovery must therefore never expose a channel's comments to a brand unless the creator explicitly opted in. Consent is per creator (tracking row), not per `platform_channels` row, because the shared channel model lets several unrelated users track the same real channel.

2. **The corpus is the asset and its cold start is the dominant product risk.** Discovery quality scales with the number of opted-in channels. A brand's demand watch can only rank channels Noumena has ingested, so early results are thin. The Phase-0 concierge experiment exists to test whether even a hand-built shortlist changes a decision.

`(hypothesis)` No agency interviews or usage analytics exist yet to confirm the ODI inputs (importance 9 / satisfaction 3). This design deliberately reuses existing primitives so the build is cheap and the bet can be killed early.

## Goals / Non-Goals

**Goals:**
- Give a workspace a reusable, named demand target ("product/category interest watch") defined by terms and a date window.
- Return a ranked creator shortlist over the opted-in corpus with transparent, componentized scores.
- Show the exact comments that justify each rank (evidence), scoped to opted-in channels.
- Enforce creator opt-in as the hard access boundary; default is off.
- Reuse existing comment search, classification, export, workspace, and migration machinery.
- Gate the whole surface behind a runtime flag.

**Non-Goals:**
- No third-party/public-data discovery (no API-key reads of non-opted-in channels, no scraping).
- No LLM changes; ranking operates on already-classified comments.
- No creator contact enrichment, outreach, or CRM.
- No cross-platform support beyond the existing YouTube-only ingestion.
- No campaign or sponsored-asset tracking (separate change).
- No global admin surface.

## Decisions

### 1. Consent as a `channel_discovery_consent` side table keyed by tracking row
**Decision:** `channel_discovery_consent(id, user_channel_tracking_id UNIQUE → user_channel_tracking(id) ON DELETE CASCADE, opted_in, opted_in_at, updated_at)`. A channel is discoverable when at least one `active` tracking row for it has `opted_in = 1`.

Rationale: consent belongs to the person who connected the channel, and one real channel may have several trackers with different choices; a channel is discoverable if *any* active tracker opts in. A side table (rather than a column on the already-wide `user_channel_tracking`) keeps an explicit audit surface and cascades cleanly on account deletion. Alternatives: (a) column on `user_channel_tracking` — fewer joins, but no distinct record for audit; (b) column on `platform_channels` — wrong scope, a shared row cannot hold one creator's consent.

### 2. Demand watches are workspace-scoped with normalized terms
**Decision:** `demand_watches(id, workspace_id, name, description, status, created_by, created_at, updated_at)` and `demand_watch_terms(id, demand_watch_id, term, created_at, UNIQUE(demand_watch_id, term))`.

Rationale: a brand runs several targets (a product, a competitor, a category); terms vary per target and should be editable without rewriting a watch. Normalizing terms keeps matching a simple `JOIN` and lets the same term serve multiple watches. Alternative: a JSON term blob — rejected because it defeats indexed matching and per-term testing.

### 3. Reuse the comment corpus and classification; do not re-classify
**Decision:** discovery matches existing comments by term and category; it does not call the LLM.

Rationale: the signal (`question`, `content_idea`, `collaboration_ask` + `topic`) already exists and is paid for. Re-classification would add cost with no new signal. Matching uses the existing FTS5 index over `comments.body` for term matching, scoped to opted-in channels.

### 4. Componentized, transparent ranking
**Decision:** rank each channel by three named components returned in the response:
- **Demand intensity** — weighted count of matching high-signal comments (questions and content ideas weighted above praise), with a freshness decay over the watch window.
- **Engagement quality** — share of matching comments that are non-toxic and non-spam.
- **Brand safety** — inverse of the channel's toxicity/spam share over the window.

A composite score combines them; the response always includes the components so the user can see *why* a creator ranked where they did.

Rationale: the differentiation is evidence, not a black-box number; and the componentization keeps the product on the safe side of the YouTube "no custom brand-suitability scores" clause — the tool exposes observed comment signals and a transparent ordering, not a claimed safety verdict. Alternative: a single opaque score — rejected for both trust and policy reasons.

### 5. Consent-scoped discovery is enforced in SQL, not in the handler
**Decision:** every discovery query joins through `channel_discovery_consent` to opted-in active trackers. There is no "include all channels" path.

Rationale: the access boundary must hold even if a handler is added later; encoding it in the query layer makes the safe path the only path. Alternative: a middleware flag — rejected because it is easy to bypass per-query.

### 6. Feature-flag the whole surface
**Decision:** `DEMAND_DISCOVERY_API_ENABLED` gates all `/api/demand/*` routes and the dashboard nav entry (mirroring `INSIGHTS_API_ENABLED`).

Rationale: the corpus, ranking weights, and consent UX all need iteration; the surface can ship dark and be enabled per environment. When disabled, routes return 503 and the migration remains harmless.

### 7. Export reuses the existing encoder
**Decision:** the shortlist export streams CSV/JSON through `internal/export`, scoped to opted-in channels.

Rationale: consistent escaping, streaming, and TTL behavior with existing comment exports.

## Risks / Trade-offs

- **Cold-start corpus** (dominant product risk) → discovery is only as good as the number of opted-in channels. Mitigated by the Phase-0 concierge experiment and by framing the corpus as a growable network-effect asset; surfaced to users as a coverage indicator, not hidden.
- **Compliance risk** (dominant viability risk) → mitigated by creator-opt-in-only, no public reads, SQL-enforced consent, and transparent (not verdict-like) signals. Residual: YouTube's derived-metrics/storage policy may require an audited quota-extension request; tracked as an explicit task and confirmed in Phase 0 legal review.
- **Consent friction reduces the corpus** → the toggle must be clearly beneficial to creators (they surface to relevant brands). If opt-in rates are too low, the bet is re-evaluated at the Phase-1 kill gate.
- **Ranking manipulation** → a creator could boost a product in their own comments. Mitigated by reporting raw components and evidence rather than a single score, and by deduplicating near-identical comment bodies in intensity counting.
- **Performance on a growing corpus** → reuse FTS5 + indexed joins; the query is read-only and bounded by the watch window. If needed, add a per-channel aggregate cache in a later change (not now).
- **Watch terms are workspace-private but comments are shared** → evidence excerpts are only returned for opted-in channels; the consent boundary is checked before any comment body is selected.

## Migration Plan

1. Add a versioned migration creating `demand_watches`, `demand_watch_terms`, and `channel_discovery_consent` plus their indexes (`workspace_id`, `demand_watch_id`, `user_channel_tracking_id`, and a partial index on `opted_in = 1`). Mirror the DDL in `schema.sql` for fresh installs.
2. Deploy backend first with `DEMAND_DISCOVERY_API_ENABLED=false`; the tables exist but no routes are exposed.
3. Enable the flag per environment once ranking and consent UX are reviewed.
4. Rollback: additive. Reverting the binary is safe; tables can remain. Dropping them manually is the full rollback. No backfill — consent defaults to off, so no data is exposed on deploy.

## Open Questions

- Should demand intensity count distinct authors as well as comments, to reduce single-user inflation? Current design counts comments with dedup of near-identical bodies; distinct-author weighting is a candidate refinement.
- Should a paused watch keep its terms and history, or be archived? Current design keeps it (`status = paused`) so it can be resumed; hard delete is explicit.
- Is a per-channel aggregate cache needed before the corpus reaches the size where ranking queries degrade? Deferred until measured.
