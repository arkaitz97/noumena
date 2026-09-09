# Spec: Frontend - Dashboard & Settings

## Overview
Build the main dashboard UI for insights feed, saved comments, sentiment visualization, channel management, billing, and account settings.

## Tech Stack
- Vue 3 with Composition API + `<script setup>`
- Pinia for state management
- Vue Router
- Axios
- Tailwind CSS v4
- Lucide Vue Next icons
- Charting library recommendation: `chart.js` with `vue-chartjs` OR lightweight SVG-based custom charts

## Routes (within DashboardLayout)

| Route | Component | Description |
|-------|-----------|-------------|
| `/dashboard` | InsightsFeed.vue | Main high-signal comment feed |
| `/dashboard/saved` | SavedCommentsView.vue | Starred comments |
| `/settings/channels` | ChannelsView.vue | Channel management |
| `/settings/billing` | BillingView.vue | Subscription & usage |
| `/settings/account` | AccountView.vue | Profile, export, deletion |

## State Management

### `useFeedStore`
```typescript
interface FeedState {
  comments: Comment[];
  total: number;
  loading: boolean;
  hasMore: boolean;
  filters: FilterState;
}

interface FilterState {
  channels: string[];
  categories: string[];
  sentimentMin: number | null;
  sentimentMax: number | null;
  dateFrom: string | null;
  dateTo: string | null;
  search: string;
  includeToxic: boolean;
  sort: 'newest' | 'sentiment_desc' | 'sentiment_asc';
}

// Actions:
- fetchFeed(reset?: boolean): Promise<void>
- loadMore(): Promise<void>
- applyFilters(filters: Partial<FilterState>): void
- starComment(id: string): Promise<void>
- unstarComment(id: string): Promise<void>
- exportFeed(format: 'csv' | 'json'): Promise<{ jobId?: string; downloadUrl?: string }>
```

### `useInsightsStore`
```typescript
interface InsightsState {
  sentiment: SentimentData | null;
  toxicity: ToxicityData | null;
  trends: TrendsData | null;
  latestDigest: WeeklyDigest | null;
}

// Actions:
- fetchSentiment(channelId?: string): Promise<void>
- fetchToxicity(channelId?: string): Promise<void>
- fetchTrends(channelId?: string, days?: number): Promise<void>
- fetchLatestDigest(): Promise<void>
```

## Views

### InsightsFeed.vue (Default Dashboard)

#### Layout
- **Top section**: Summary cards row
  - Total comments this week
  - Average sentiment (with trend indicator)
  - Toxicity Shield percentage
  - Comments processed / quota used
- **Filter bar**: Sticky below summary
  - Channel multi-select dropdown
  - Category multi-select pills (Question, Praise, Criticism, Content Idea, Collaboration Ask, Other)
  - Sentiment range slider (-1.0 to +1.0)
  - Date range picker (Last 7 days, 30 days, 90 days, Custom)
  - Search input (debounced 300ms)
  - "Include filtered" toggle (shows toxic/spam)
  - "Export" dropdown (CSV, JSON)
- **Comment feed**: Infinite scroll list
  - Each card shows:
    - Channel name badge
    - Author handle
    - Comment text (truncated with "Show more")
    - Category badge (color-coded)
    - Sentiment score (colored bar)
    - Topic tag
    - Posted timestamp (relative: "2h ago")
    - Star button (toggle)
    - "Processing..." badge for unprocessed comments
  - Skeleton loaders while loading
  - Empty state: "No comments match your filters. Try expanding the date range."
- **Right sidebar** (desktop only):
  - Latest Weekly Digest preview card
  - Top topics cloud

#### Interactions
- Infinite scroll: offset-based, load next 20 when scrolling near bottom
- Star toggle: optimistic update (flip immediately, revert on error)
- Filter changes: reset to page 0, fetch new results, update URL query params
- Export: show modal with format choice, then download or show "Queued" notification

### SavedCommentsView.vue
- Similar to InsightsFeed but only starred comments
- No sentiment/toxicity summary cards
- Filter bar without channel/category (or keep them but simpler)
- Bulk unstar option (optional)

### ChannelsView.vue
- List of tracked channels as cards
- Each card shows:
  - Platform icon (YouTube)
  - Channel name
  - Status badge (Active, Paused, Syncing)
  - Last synced timestamp
  - Comment count
  - Token expiry warning (if < 7 days)
- Actions per channel:
  - Pause / Resume button
  - Remove button (with confirmation modal)
  - Backfill window setting (if adding new — not for existing)
- "Connect YouTube Channel" button at top
- OAuth flow handled same as onboarding

### BillingView.vue
- Current plan card (highlighted):
  - Plan name, price, billing interval
  - Usage bars:
    - Channels: 2/3 used
    - Comments: 12,400/50,000 used
  - Progress bar with color (green < 75%, yellow < 95%, red ≥ 95%)
- Upgrade/Downgrade buttons:
  - Show all plans in comparison grid
  - Monthly/Annual toggle (show savings for annual)
  - Clicking upgrade opens Stripe Checkout in new tab
- Payment method section:
  - "Manage in Stripe Portal" button
- Invoice history (from Stripe Customer Portal link)
- Ousage alerts section (if near limit)

### AccountView.vue
- Profile section:
  - Email (with verification status)
  - Change password button
- Preferences:
  - Weekly digest email opt-in toggle
- Data & Privacy:
  - "Export my data (GDPR)" button → triggers export job, shows status
  - "Delete account" button:
    - Step 1: Warning modal with consequences explained
    - Step 2: Email confirmation input
    - On confirm: locks account, shows "Deletion scheduled" message, auto-logs out
- Security:
  - "Log out of all devices" button

## Components to Build

### CommentCard.vue
- Props: `comment: Comment`, `isStarred: boolean`
- Emits: `toggle-star`
- Responsive layout: stacked on mobile, horizontal on desktop

### FilterBar.vue
- Props: `filters: FilterState`, `channels: ChannelOption[]`
- Emits: `update:filters`, `export`, `reset`

### SentimentChart.vue
- Props: `data: SentimentData`
- Renders line chart with 7d/30d/90d toggle
- Color gradient based on sentiment

### ToxicityShield.vue
- Props: `data: ToxicityData`
- Circular progress indicator or donut chart
- Color: green (low), yellow (medium), red (high)

### WeeklyDigestCard.vue
- Props: `digest: WeeklyDigest`
- Collapsible sections for ideas, questions, sentiment trend
- "View full digest" link

### PlanComparison.vue
- Props: `plans: Plan[]`, `currentPlan: string`
- Grid comparing features across tiers
- CTA buttons per plan

## API Integration
- All data fetched from `/api/*` endpoints defined in other specs
- Polling for real-time feel: every 60 seconds for feed, every 5 minutes for insights
- Loading states for all async operations
- Error boundaries: toast notifications for API errors

## Testing Checklist
- [ ] Feed loads with infinite scroll
- [ ] Filters update results and URL params
- [ ] Star/unstar updates optimistically
- [ ] Export triggers download or queued state
- [ ] Sentiment chart renders correctly
- [ ] Channels can be paused and resumed
- [ ] Channel removal shows confirmation modal
- [ ] Billing page shows correct usage
- [ ] Plan upgrade opens Stripe Checkout
- [ ] Account deletion flow has two-step confirmation
- [ ] GDPR export triggers and shows status

## Dependencies to Add
```json
{
  "chart.js": "^4.4.0",
  "vue-chartjs": "^5.3.0",
  "date-fns": "^3.0.0"
}
```

## Files to Create/Modify
- `frontend/src/views/dashboard/InsightsFeed.vue`
- `frontend/src/views/dashboard/SavedCommentsView.vue`
- `frontend/src/views/settings/ChannelsView.vue`
- `frontend/src/views/settings/BillingView.vue`
- `frontend/src/views/settings/AccountView.vue`
- `frontend/src/views/settings/SettingsLayout.vue`
- `frontend/src/components/CommentCard.vue`
- `frontend/src/components/FilterBar.vue`
- `frontend/src/components/SentimentChart.vue`
- `frontend/src/components/ToxicityShield.vue`
- `frontend/src/components/WeeklyDigestCard.vue`
- `frontend/src/components/PlanComparison.vue`
- `frontend/src/stores/feed.ts`
- `frontend/src/stores/insights.ts`
- `frontend/src/stores/channels.ts`
- `frontend/src/services/api.ts`
