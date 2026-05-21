## Context

Noumena currently classifies YouTube comments using LLMs into categories (question, praise, constructive_criticism, etc.) with a sentiment score (-1.0 to +1.0), topic label, and confidence. The sentiment score is used in feed sorting/filtering, aggregate trend charts, and weekly digest summaries. User feedback indicates sentiment scores are abstract and not actionable — creators want concrete suggestions extracted from their audience comments.

The LLM processing pipeline already supports batch classification, provider failover, and confidence thresholds. The database schema stores sentiment_score as NUMERIC(4,2). The frontend renders sentiment trend charts and supports sentiment-based feed filtering.

## Goals / Non-Goals

**Goals:**
- Replace sentiment scoring with structured actionable insight extraction per comment
- Provide creators with concrete, extractable suggestions: video ideas, editing tips, feature requests, collaboration opportunities
- Maintain backward compatibility for existing LLM fields (category, topic, confidence)
- Keep the same batch processing architecture and failover mechanism
- Update all downstream consumers: API, frontend, weekly digest

**Non-Goals:**
- No backfill of actionable_insights for already-processed comments (LLM re-processing is cost-prohibitive)
- No change to the comment ingestion pipeline
- No change to billing/quota mechanics
- No change to category classification (8 existing categories remain)
- No real-time insight notification system

## Decisions

### Decision 1: Actionable insights as JSONB array on comments table

**Choice:** Store `actionable_insights` as a JSONB column on the `comments` table rather than a separate normalized table.

**Rationale:** Insights are tightly coupled to their source comment, queries always filter by channel/user (not cross-comment insight search), and JSONB enables GIN indexing for efficient containment queries. A separate table would add join complexity without meaningful query benefit.

**Alternatives considered:**
- Separate `comment_insights` table: rejected due to join overhead and no independent insight lifecycle
- Separate `actionable_insights` column per type (video_ideas, editing_tips, etc.): rejected because the set of insight types may evolve

**Schema:**
```json
[
  {
    "type": "video_idea",
    "description": "Make a tutorial on how you color grade your footage",
    "confidence": 0.85
  },
  {
    "type": "editing_tip",
    "description": "The transition at 3:42 feels abrupt, try a crossfade",
    "confidence": 0.72
  }
]
```

**Insight types:** `video_idea`, `editing_tip`, `feature_request`, `collaboration_ask`, `improvement_suggestion`, `content_format_request`, `other`

### Decision 2: Single LLM call for both category and insights

**Choice:** Extend the existing batch classification prompt to extract actionable_insights alongside the existing category/topic/confidence fields, rather than making a second LLM call.

**Rationale:** The LLM already reads each comment for classification; adding insight extraction to the same prompt adds minimal token overhead vs. a second call. Keeps the processor architecture unchanged (one LLM call per batch).

**Alternatives considered:**
- Second LLM call for insights only: rejected due to 2x API cost and latency
- Post-processing heuristic extraction: rejected due to poor quality vs. LLM understanding

### Decision 3: Remove sentiment entirely, do not deprecate

**Choice:** Remove `sentiment_score` column, endpoint, and frontend components in a single migration rather than deprecating gradually.

**Rationale:** Sentiment is not used by any external integrations (no public webhook includes it). The field is purely internal to Noumena's own features. A clean removal reduces migration complexity and avoids maintaining dead code.

**Alternatives considered:**
- Deprecate first, remove later: rejected because there are no external consumers and it extends the maintenance window
- Keep sentiment but hide from UI: rejected because it wastes DB storage and LLM tokens

### Decision 4: Insights aggregation via materialized view

**Choice:** Create a PostgreSQL materialized view `mv_actionable_insights_agg` that pre-computes insight counts and top descriptions per channel per time window, refreshed on a cron.

**Rationale:** Insight aggregation queries (GROUP BY type, ORDER BY count DESC, deduplicate similar descriptions) are expensive on raw JSONB. A materialized view keeps API response times under the 2000ms query timeout.

**Refresh strategy:** Refresh every 15 minutes via the existing job scheduler, or trigger refresh after LLM processor completes a batch for a channel.

**Alternatives considered:**
- Real-time aggregation on every API call: rejected due to JSONB query cost at scale
- Redis cache layer: rejected to avoid adding a new infrastructure dependency

### Decision 5: Feed filter replaces sentiment range with insight type

**Choice:** Replace `sentiment_min`/`sentiment_max` feed query params with `insight_type` param that filters comments containing at least one insight of the specified type.

**Rationale:** Maintains the feed filtering UX pattern while aligning with the new data model. Multiple types can be comma-separated.

## Risks / Trade-offs

- **[Risk] LLM prompt complexity increases** — Adding insight extraction to the classification prompt may reduce category classification accuracy. → **Mitigation:** A/B test prompt with confidence threshold monitoring; if category accuracy drops below 90%, split into two calls.
- **[Risk] No backfill for existing comments** — Historical comments lose sentiment data with no replacement. → **Mitigation:** Accept as trade-off; communicate to users that actionable insights apply to new comments going forward.
- **[Risk] JSONB storage growth** — Each comment now stores an array of insight objects instead of a single float. → **Mitigation:** Cap insights at 5 per comment; average insight object is ~100 bytes vs. 4 bytes for sentiment, but only ~30% of comments yield actionable insights.
- **[Risk] Materialized view staleness** — 15-minute refresh window means insights aggregation may lag. → **Mitigation:** Acceptable for dashboard use case; add manual refresh endpoint for power users.
- **[Trade-off] Insight deduplication is approximate** — Similar video ideas from different comments won't be perfectly deduplicated. → **Mitigation:** Use LLM-based deduplication in the aggregation layer with a similarity threshold.

## Migration Plan

1. **Database migration** (single transaction):
   a. Add `actionable_insights JSONB` column to `comments` table
   b. Create GIN index on `actionable_insights`
   c. Drop `sentiment_score` column
   d. Drop sentiment-related indexes
   e. Create `mv_actionable_insights_agg` materialized view

2. **Backend deployment** (rolling, feature-flagged):
   a. Deploy new LLM prompt and processor with `LLM_ACTIONABLE_INSIGHTS_ENABLED=true`
   b. Deploy new insights API endpoints behind `INSIGHTS_API_ENABLED` (already exists)
   c. Deploy updated feed handlers (sentiment params become no-ops, insight_type param added)

3. **Frontend deployment** (feature-flagged):
   a. Deploy new actionable insights components behind feature flag
   b. Remove sentiment chart components in same deploy (flag controls visibility)

4. **Rollback strategy:**
   - If LLM accuracy drops: set `LLM_ACTIONABLE_INSIGHTS_ENABLED=false`, processor falls back to category-only prompt
   - If API issues: revert frontend flag to show legacy UI (requires sentiment column to be retained in migration — add it back as nullable if rollback needed)
   - Full rollback: revert all deployments, re-add `sentiment_score` column via reverse migration

## Open Questions

- Should insight types be configurable per channel, or fixed globally? → **Tentative:** Fixed globally for v1, configurable in future.
- Should the weekly digest LLM prompt use raw comments or pre-extracted insights as input? → **Tentative:** Use pre-extracted insights for cost efficiency, but include a sample of raw comments for context.
- What is the maximum number of insights per comment? → **Tentative:** 5, enforced by validation in `ParseClassificationResponse`.
