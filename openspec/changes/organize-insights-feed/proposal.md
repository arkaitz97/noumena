## Why

The current insights feed (`/dashboard`) surfaces comments as a flat, unsegmented chronological stream where critical AI-extracted signals (actionable video ideas, urgent questions, constructive critique) compete with low-signal comments. High-level analytical widgets in the sidebar are disconnected from the feed, actionable insights within comment cards are hidden behind collapsed toggles, and switching between triage workflows (e.g. reviewing questions vs. content ideas) requires opening and manipulating a complex multi-select filter popover.

Organizing the feed with intent-based views, visual signal prioritization, interactive cross-filtering, and optional video-level grouping dramatically reduces cognitive load and allows creators to quickly triage feedback and extract value from their audience.

## What Changes

- **Intent-Driven Quick Tabs**: Add top-level workflow tabs (`All`, `Actionable Insights`, `Questions`, `Ideas`, `Constructive Critique`) above the feed to switch triage modes in one click without opening the filter drawer.
- **Signal-Forward Comment Cards**: Redesign `CommentCard.vue` to prominently surface extracted actionable insights in dedicated callout banners at the top of cards rather than hiding them behind collapsed buttons. Add subtle visual category accent cues.
- **Interactive Bi-Directional Filtering**: Connect sidebar analytical widgets (clicking "Top Topics" chips or "Insight Distribution" bars) directly to the feed filters so clicking a topic or insight type immediately focuses the comments feed.
- **Group-by-Video View Option**: Introduce a view toggle (`Stream` vs. `Group by Video`) that clusters comments under collapsible video headers with signal badges and video metrics, removing repetitive video tags.
- **Layout & Information Architecture Refinements**: Relocate `ActionableInsightsPanel` or integrate its top highlights into the intent selector bar, removing the awkward split between sidebar charts and the stranded top card.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `frontend-dashboard`: Update requirements for the comment feed UI, comment card presentation, intent-based filtering, and interactive insights visualizations.

## Impact

- **Frontend Views & Components**:
  - `frontend/src/views/dashboard/InsightsFeed.vue`: Updated layout, intent tab integration, group-by-video display mode, and widget cross-filtering.
  - `frontend/src/components/CommentCard.vue`: Promoted insight callout display, redesigned metadata hierarchy, category accent styling.
  - `frontend/src/components/FilterBar.vue`: Integration of intent tabs and grouping controls.
  - `frontend/src/components/InsightTypeDistributionChart.vue` and topic lists: Add click-to-filter event emission.
- **Frontend Stores**:
  - `frontend/src/stores/feed.ts`: Support for active intent preset, grouping helper computed states, and topic filtering.
- **Backend / APIs**:
  - No backend schema breaking changes required; existing `/api/comments/feed` and `/api/insights/*` query parameters fully support the needed filter combinations (`insight_type`, `categories`, `search`).
