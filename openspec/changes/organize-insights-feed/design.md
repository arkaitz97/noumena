## Context

The insights feed on `/dashboard` is the core surface for Noumena users to review viewer comments and extracted signals. Currently:
- The screen stacks heavy sidebar widgets against an unbounded stream of `CommentCard` components.
- Comments are presented chronologically without structural grouping or priority weighting.
- Actionable insights are hidden behind an accordion toggle within each card, obscuring the primary value proposition.
- All filtering is locked behind a popover modal, creating unnecessary friction when shifting between triage mindsets (e.g., answering questions vs. reviewing video ideas).

This design details the frontend architecture to organize the feed into high-intent triage modes, surface signals clearly, and introduce video-level grouping.

## Goals / Non-Goals

**Goals:**
- **Rapid Triage**: Allow 1-click switching between common creator workflows (`All`, `Actionable`, `Questions`, `Ideas`, `Critique`) via top-level intent tabs.
- **Signal-Forward Cards**: Redesign `CommentCard.vue` so the most critical actionable insight is rendered immediately in a top callout banner with confidence indicators and semantic category cues.
- **Bi-Directional Filtering**: Enable clicking on topics in the trends list or bars in the distribution chart to immediately focus the comments feed.
- **Video-Centric Grouping**: Provide an optional "Group by Video" toggle that groups comments under collapsible video headers, aggregating comment count and signal stats.
- **URL & Store Consistency**: Keep all intent tab and grouping states synchronized with URL query params and the Pinia `feed` store.

**Non-Goals:**
- Modifying backend SQL schemas or creating new API endpoints; existing `/api/comments/feed` query parameters (`categories`, `insight_type`, `search`, `channels`) and `/api/insights/*` endpoints are fully sufficient.
- Implementing direct YouTube comment reply execution (handled in a separate change).

## Decisions

### Decision 1: Intent Tabs as Filter Synthesizers
- **Decision**: Intent tabs (`All`, `Actionable`, `Questions`, `Ideas`, `Critique`) act as high-level filter synthesizers mapped directly to existing `FeedFilters` (`categories` and `insight_type`).
- **Rationale**: Avoids creating duplicate routes. When a user clicks "Questions", it applies `{ categories: ['question'] }`. When "Actionable" is clicked, it applies `{ insight_type: null, categories: ['content_idea', 'question', 'constructive_criticism'] }` or sets `insight_type` appropriately. If custom filters are selected in the advanced popover, the tab indicator shows "Custom".
- **Alternatives Considered**: Separate route paths (e.g. `/dashboard/questions`, `/dashboard/ideas`). Rejected because it breaks filter composability (e.g. searching or scoping to date ranges across intents) and fractures URL state.

### Decision 2: Client-Side Grouping with Stable Bucket Keys
- **Decision**: The "Group by Video" mode is computed reactively over `feed.comments` in the frontend using `video_id || 'unknown'` as the bucket key.
- **Rationale**: Each `Comment` payload already returns `video_id`, `video_title`, and `channel_name`. Client-side grouping allows instantaneous switching between Stream view and Grouped view without refetching or backend architectural changes.
- **State Handling**: Collapsed/expanded states for video groups are tracked in a `Set<string>` in component state, defaulting to expanded for the first 3 videos and collapsed for older ones to prevent DOM bloat.

### Decision 3: Promoted Insight Banner on Comment Cards
- **Decision**: If a comment has `actionable_insights.length > 0`, the highest-confidence insight is rendered as a stylized callout header inside the card. If multiple insights exist, a subtle `+N more insights` badge allows expanding the remaining ones.
- **Rationale**: Noumena's core proposition is extracting actionable value from audience feedback. Hiding this behind an accordion toggle buried under the body text suppresses the primary signal.

### Decision 4: Interactive Topic Cross-Filtering
- **Decision**: Emit a `@select-topic` event from the Top Topics widget and wire it to set `filters.search` or `filters.topic` in `feed.ts`.
- **Rationale**: Users seeing a spike in `#audio-issues` in the trends sidebar expect clicking it to immediately reveal those comments.

## Risks / Trade-offs

- **[Risk] Infinite pagination in Group-by-Video mode**: Loading the next page of 20 comments may contain comments belonging to an earlier video already rendered at the top of the feed.
  - *Mitigation*: The grouping computed property merges comments by `video_id` preserving video order by latest comment date. An indicator on the video group header clearly shows how many comments are loaded.
- **[Risk] Mobile responsive layout density**: Adding intent tabs and grouping controls could crowd the header on small screens.
  - *Mitigation*: Use a responsive horizontally scrollable tab list with container queries (`@container`) and touch-friendly padding.

## Migration Plan

1. Frontend-only change; no database or backend deployment dependencies.
2. Update unit tests (`CommentCard.spec.ts`, `InsightsFeed.drilldown.test.ts`) and Playwright e2e tests (`feed.spec.ts`) to assert intent tabs, card callouts, and grouping behavior.
3. Deploy frontend bundle.
