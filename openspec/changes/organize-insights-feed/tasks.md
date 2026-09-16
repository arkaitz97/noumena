## 1. Intent Tabs Component & State

- [x] 1.1 Define intent presets and types in `frontend/src/lib/intentTabs.ts` (`All`, `Actionable`, `Questions`, `Ideas`, `Critique`)
- [x] 1.2 Create `frontend/src/components/FeedIntentTabs.vue` with active state indicators and count badges
- [x] 1.3 Wire intent tab selection to `useFeedStore` filters and URL query parameters in `InsightsFeed.vue`
- [x] 1.4 Add unit tests for `FeedIntentTabs.vue` verifying preset switching and URL synchronization

## 2. Signal-Forward CommentCard Redesign

- [x] 2.1 Refactor `CommentCard.vue` to render the primary actionable insight as a prominent top callout banner with confidence score and type badge
- [x] 2.2 Add category accent borders and styling to visually distinguish questions, ideas, and critique
- [x] 2.3 Streamline author, video, and timestamp metadata layout to prevent awkward wrapping
- [x] 2.4 Update `CommentCard.spec.ts` to verify prominent insight banner rendering and category accents

## 3. Interactive Sidebar Cross-Filtering

- [x] 3.1 Update top topics chips in `InsightsFeed.vue` to trigger topic filtering on click
- [x] 3.2 Update `InsightTypeDistributionChart.vue` to emit `@select-type` on bar or label click
- [x] 3.3 Connect chart selection in `InsightsFeed.vue` to filter the feed by the clicked insight type
- [x] 3.4 Adjust layout so `ActionableInsightsPanel.vue` fits harmoniously without pushing the feed down

## 4. Group-by-Video View Mode

- [x] 4.1 Implement `groupByVideo` computed property and state in `InsightsFeed.vue`
- [x] 4.2 Add view mode toggle (`Stream` vs `Group by Video`) to feed controls
- [x] 4.3 Create collapsible video section header displaying video title, comment count, insight count, and YouTube link
- [x] 4.4 Verify infinite scroll works seamlessly in both flat stream and grouped video views

## 5. Verification & End-to-End Tests

- [x] 5.1 Run and update Vitest unit tests (`pnpm test` / `npm test`)
- [x] 5.2 Update Playwright tests in `frontend/e2e/specs/feed.spec.ts` covering intent tabs and video grouping
- [x] 5.3 Run axe accessibility checks to maintain WCAG 2.1 compliance
