<script setup lang="ts">
// Insights feed (/dashboard) — primary working surface.
//
// Composes: full-width sticky FilterBar on top, then a grid: right sidebar
// (desktop) with the Toxicity-shield KPI card + SentimentChart /
// InsightTypeDistributionChart / topic chips / WeeklyDigestCard (ordered by
// actionability), left column
// with the infinite-scroll CommentCard list and insight panels.
// Pollers: feed at 60 s, insights at 5 min — both visibility-gated.

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ChevronRight, Download, ExternalLink, MessageSquare, X } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'
import { useFeedStore } from '../../stores/feed'
import { useInsightsStore } from '../../stores/insights'
import { useChannelsStore } from '../../stores/channels'
import { useFilterUrlSync } from '../../lib/useFilterUrlSync'
import { usePolling } from '../../lib/usePolling'
import { useKeyboardShortcuts } from '../../composables/useKeyboardShortcuts'
import { useWorkspaceRole } from '../../composables/useWorkspaceRole'
import { CATEGORIES, HIGH_SIGNAL_CATEGORIES, type Category } from '../../lib/categories'
import { shortDate } from '../../lib/relativeTime'
import type { ExportFormat, FeedFilters, InsightType, WindowKey } from '../../services/types'
import type { FeedScope } from '../../lib/feedScope'
import { DEFAULT_FEED_FILTERS, parseFeedScope } from '../../lib/feedScope'
import { presetFilters, type FeedIntent } from '../../lib/intentTabs'
import Card from '../../components/Card.vue'
import CommentCard from '../../components/CommentCard.vue'
import FilterBar from '../../components/FilterBar.vue'
import FeedIntentTabs from '../../components/FeedIntentTabs.vue'
import InsightTypeDistributionChart from '../../components/InsightTypeDistributionChart.vue'
import SentimentChart from '../../components/SentimentChart.vue'
import ToxicityShield from '../../components/ToxicityShield.vue'
import WeeklyDigestCard from '../../components/WeeklyDigestCard.vue'
import KeyboardHelpModal from '../../components/KeyboardHelpModal.vue'
import PageHeader from '../../components/PageHeader.vue'

const feed = useFeedStore()
const insights = useInsightsStore()
const channels = useChannelsStore()
const route = useRoute()
const router = useRouter()
const { canExport } = useWorkspaceRole()

const VALID_SORTS: FeedFilters['sort'][] = ['newest', 'oldest']

function asString(v: unknown): string | null {
  if (typeof v === 'string') return v
  if (Array.isArray(v) && typeof v[0] === 'string') return v[0]
  return null
}

const VALID_INSIGHT_TYPES: InsightType[] = [
  'video_idea', 'editing_tip', 'feature_request',
  'collaboration_ask', 'improvement_suggestion',
  'content_format_request', 'other',
]

const RANGE_DAYS: Partial<Record<WindowKey, number>> = { '7d': 7, '30d': 30, '90d': 90 }

// Preset ranges materialise into date_from/date_to; the URL carries only the
// dates, not the range. Reverse-map the span back to its window so a reload
// restores the preset (and its widget fetches) instead of defaulting to 7d.
function rangeFromQuery(query: Record<string, unknown>): WindowKey | null {
  const from = asString(query.date_from)
  const to = asString(query.date_to)
  if (!from || !to) return null
  const span = Math.round((Date.parse(to) - Date.parse(from)) / 86_400_000)
  return (Object.keys(RANGE_DAYS) as WindowKey[]).find((k) => RANGE_DAYS[k] === span) ?? null
}

const range = ref<WindowKey>(rangeFromQuery(route.query) ?? '7d')
const intent = ref<FeedIntent>((asString(route.query.intent) as FeedIntent) || 'all')
const viewMode = ref<'stream' | 'video'>((asString(route.query.view) as 'stream' | 'video') || 'stream')
const collapsedVideos = ref(new Set<string>())

const serialiseFilters = (f: FeedFilters): Record<string, string> => {
  const q: Record<string, string> = {}
  if (f.channels.length > 0) q.channels = f.channels.join(',')
  const sortedCats = [...f.categories].sort().join(',')
  const defaultCats = [...HIGH_SIGNAL_CATEGORIES].sort().join(',')
  if (f.categories_specified || sortedCats !== defaultCats) q.categories = f.categories.join(',')
  if (f.insight_type) q.insight_type = f.insight_type
  if (f.date_from) q.date_from = f.date_from
  if (f.date_to) q.date_to = f.date_to
  if (f.search) q.search = f.search
  if (f.include_toxic) q.include_toxic = '1'
  if (f.sort && f.sort !== 'newest') q.sort = f.sort
  if (intent.value !== 'all') q.intent = intent.value
  if (viewMode.value === 'video') q.view = 'video'
  return q
}

const parseFilters = (query: Record<string, unknown>): Partial<FeedFilters> => {
  const out: Partial<FeedFilters> = {}
  const channelsRaw = asString(query.channels)
  if (channelsRaw) out.channels = channelsRaw.split(',').filter(Boolean)
  const catsRaw = asString(query.categories)
  if (catsRaw !== null) {
    out.categories = catsRaw
      .split(',')
      .filter((c): c is Category => (CATEGORIES as readonly string[]).includes(c))
  }
  if (Object.prototype.hasOwnProperty.call(query, 'categories')) out.categories_specified = true
  const insightType = asString(query.insight_type)
  if (insightType && (VALID_INSIGHT_TYPES as readonly string[]).includes(insightType)) {
    out.insight_type = insightType as InsightType
  }
  const df = asString(query.date_from)
  if (df) out.date_from = df
  const dt = asString(query.date_to)
  if (dt) out.date_to = dt
  const search = asString(query.search)
  if (search) out.search = search
  const includeToxic = asString(query.include_toxic)
  if (includeToxic === '1' || includeToxic === 'true') out.include_toxic = true
  const sort = asString(query.sort)
  if (sort && (VALID_SORTS as string[]).includes(sort)) out.sort = sort as FeedFilters['sort']
  return out
}

const groupedComments = computed(() => {
  // Ponytail ceiling: grouping only covers loaded pages; server-side grouping is upgrade path for huge feeds.
  const groups = new Map<string, typeof feed.comments>()
  for (const comment of feed.comments) {
    const key = comment.video_id || 'unknown'
    const group = groups.get(key) ?? []
    group.push(comment)
    groups.set(key, group)
  }
  return [...groups.entries()].map(([id, comments]) => ({
    id,
    title: comments[0]?.video_title || 'Unknown video',
    videoId: comments[0]?.video_id,
    comments,
    insightCount: comments.reduce((count, comment) => count + (comment.actionable_insights?.length ?? 0), 0),
  }))
})

function selectIntent(next: FeedIntent): void {
  hasScrolled = false
  intent.value = next
  feed.applyFilters({ ...presetFilters(next), intent: next === 'questions' || next === 'critique' ? 'all' : next })
  void feed.fetchFeed({ reset: true })
  void router.replace({ query: serialiseFilters(feed.filters) })
}

function selectTopic(topic: string): void {
  onUpdateFilters({ search: topic })
}

function selectInsightType(type: InsightType): void {
  onUpdateFilters({ insight_type: type })
}

function toggleVideo(id: string): void {
  if (collapsedVideos.value.has(id)) collapsedVideos.value.delete(id)
  else collapsedVideos.value.add(id)
}

const commentIndexMap = computed(() => {
  const map = new Map<string, number>()
  feed.comments.forEach((c, i) => map.set(c.id, i))
  return map
})

const allCollapsed = computed(() => {
  if (groupedComments.value.length === 0) return false
  return groupedComments.value.every((g) => collapsedVideos.value.has(g.id))
})

function toggleAllVideos(): void {
  if (allCollapsed.value) {
    collapsedVideos.value.clear()
  } else {
    collapsedVideos.value = new Set(groupedComments.value.map((g) => g.id))
  }
}

useFilterUrlSync<FeedFilters>({
  get: () => feed.filters,
  apply: (partial) => feed.applyFilters(partial),
  serialise: serialiseFilters,
  parse: parseFilters,
})

// Drill-down: navigating to this view with a filter query (e.g. an insight
// card link) re-parses the URL and refetches. UI-driven filter changes are
// already reflected in the store, so the equality guard avoids double fetch.
watch(
  () => route.query,
  (q) => {
    if (JSON.stringify(q) === JSON.stringify(serialiseFilters(feed.filters))) return
    const hydrated = parseFeedScope(q as Record<string, unknown>, DEFAULT_FEED_FILTERS())
    range.value = hydrated.range
    intent.value = hydrated.intent === 'ideas' || hydrated.intent === 'actionable' ? hydrated.intent : 'all'
    viewMode.value = asString(q.view) === 'video' ? 'video' : 'stream'
    feed.applyFilters(hydrated)
    void feed.fetchFeed({ reset: true })
  },
)

function onUpdateFilters(partial: Partial<FeedFilters>): void {
  hasScrolled = false
  feed.applyFilters(partial)
  void feed.fetchFeed({ reset: true })
}

function onResetFilters(): void {
  hasScrolled = false
  feed.resetFilters()
  intent.value = 'all'
  range.value = '7d'
  viewMode.value = 'stream'
  void feed.fetchFeed({ reset: true })
}

// Widgets follow the FilterBar: one channel (first selected), the screen
// range, and — for the actionable panel — the insight-type filter.
const widgetScope = computed<FeedScope>(() => ({
  ...feed.filters,
  intent: (intent.value === 'ideas' || intent.value === 'actionable' ? intent.value : 'all') as FeedScope['intent'],
  range: range.value,
  view: viewMode.value,
}))
const rangeDays = computed(() => RANGE_DAYS[range.value] ?? 30)
const toxicityWindow = computed<WindowKey>(() => range.value)

async function refreshInsights(force = false): Promise<void> {
  await insights.fetchActionableInsights(widgetScope.value, range.value, { force })
}

// Sentiment trend — the backend accepts an optional channel_id: with a
// channel filter selected the chart is scoped to that channel, without one
// it aggregates across every tracked channel (global trend).
// `fetchSentiment` is guarded for test doubles that predate the action.
async function refreshSentiment(force = false): Promise<void> {
  await insights.fetchSentiment?.(widgetScope.value, rangeDays.value, { force })
}

function refreshRangeWidgets(force: boolean): void {
  void refreshInsights(force)
  void insights.fetchToxicity(widgetScope.value, toxicityWindow.value, { force })
  void insights.fetchTrends(widgetScope.value, rangeDays.value, { force })
}

watch(range, () => refreshRangeWidgets(false))

watch(widgetScope, () => {
  refreshRangeWidgets(true)
  void refreshSentiment(true)
})

const exportOpen = ref(false)
const exportFormat = ref<ExportFormat>('csv')
const exportSubmitting = ref(false)

async function submitExport(): Promise<void> {
  exportSubmitting.value = true
  try {
    await feed.requestExport(exportFormat.value)
  } finally {
    exportSubmitting.value = false
    exportOpen.value = false
  }
}

const sentinel = ref<HTMLElement | null>(null)
let observer: IntersectionObserver | null = null
let hasScrolled = false

function onUserScroll(): void {
  hasScrolled = true
}

function setupObserver(): void {
  if (!sentinel.value || typeof IntersectionObserver === 'undefined') return
  if (typeof window !== 'undefined') {
    window.addEventListener('scroll', onUserScroll, { passive: true, once: true })
    window.addEventListener('wheel', onUserScroll, { passive: true, once: true })
    window.addEventListener('touchmove', onUserScroll, { passive: true, once: true })
  }
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        const canTrigger = hasScrolled || (typeof window !== 'undefined' && window.scrollY > 0)
        if (entry.isIntersecting && canTrigger && feed.hasMore && !feed.loadingMore && !feed.loading) {
          void feed.loadMore()
        }
      }
    },
    { rootMargin: '200px' },
  )
  observer.observe(sentinel.value)
}

function teardownObserver(): void {
  if (typeof window !== 'undefined') {
    window.removeEventListener('scroll', onUserScroll)
    window.removeEventListener('wheel', onUserScroll)
    window.removeEventListener('touchmove', onUserScroll)
  }
  if (observer) {
    observer.disconnect()
    observer = null
  }
}

// Keyboard shortcuts
const filterBarRef = ref<InstanceType<typeof FilterBar> | null>(null)
const focusedIndex = ref(-1)

function focusSearch() {
  filterBarRef.value?.$el?.querySelector('input[type="search"]')?.focus()
}

function starFocused() {
  if (focusedIndex.value < 0 || focusedIndex.value >= feed.comments.length) return
  const c = feed.comments[focusedIndex.value]
  if (c.is_starred) {
    void feed.unstarComment(c.id)
  } else {
    void feed.starComment(c.id)
  }
}

function navigateDown() {
  if (feed.comments.length === 0) return
  focusedIndex.value = Math.min(focusedIndex.value + 1, feed.comments.length - 1)
}

function navigateUp() {
  if (feed.comments.length === 0) return
  focusedIndex.value = Math.max(focusedIndex.value - 1, 0)
}

watch(focusedIndex, (idx) => {
  if (idx < 0) return
  const cards = document.querySelectorAll('[data-comment-card]')
  if (cards[idx]) {
    cards[idx].scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  }
})

const { helpVisible } = useKeyboardShortcuts({
  onStar: starFocused,
  onNavigateDown: navigateDown,
  onNavigateUp: navigateUp,
  onFocusSearch: focusSearch,
})

onMounted(async () => {
  void channels.fetchAll()
  refreshRangeWidgets(true)
  void insights.fetchLatestDigest({ force: true })
  void refreshSentiment(true)

  await feed.fetchFeed({ reset: true })
  setupObserver()
})

onBeforeUnmount(() => {
  teardownObserver()
  feed.stopExportPolling()
})

usePolling(() => feed.fetchFeed({ reset: true }), 60_000)
usePolling(async () => {
  await Promise.all([
    insights.fetchActionableInsights(widgetScope.value, range.value, { force: true }),
    insights.fetchToxicity(widgetScope.value, toxicityWindow.value, { force: true }),
    insights.fetchTrends(widgetScope.value, rangeDays.value, { force: true }),
    refreshSentiment(true),
  ])
}, 5 * 60_000)
</script>

<template>
  <div class="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
    <PageHeader
      class="mb-6"
      title="Insights feed"
      subtitle="Real-time signals across your channels."
    >
      <template #actions>
        <button
          type="button"
          @click="helpVisible = true"
          class="inline-flex items-center gap-2 text-xs font-semibold text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-white px-3 py-2 rounded-lg hover:bg-zinc-100/80 dark:hover:bg-white/5 transition-all duration-200 active:scale-95"
        >
          <kbd class="inline-flex items-center justify-center min-w-[1.4rem] h-5 px-1 py-0.5 text-[10px] font-mono font-semibold bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-white/10 rounded-md shadow-card dark:shadow-none">?</kbd>
          Keyboard shortcuts
        </button>
      </template>
    </PageHeader>

    <div class="mb-6 space-y-4">
      <FilterBar
        ref="filterBarRef"
        :filters="feed.filters"
        :channels="channels.channels"
        :range="range"
        variant="feed"
        @update:filters="onUpdateFilters"
        @update:range="range = $event"
        @export="canExport && (exportOpen = true)"
        @reset="onResetFilters"
      />
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-200 dark:border-white/10 pb-1.5">
        <FeedIntentTabs :active="intent" @select="selectIntent" />
        <div class="flex items-center gap-2 pb-1 sm:pb-0">
          <button
            v-if="viewMode === 'video' && groupedComments.length > 1"
            type="button"
            class="text-xs font-medium text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200 transition-colors px-2 py-1 rounded-md hover:bg-zinc-100 dark:hover:bg-white/5"
            @click="toggleAllVideos"
          >
            {{ allCollapsed ? 'Expand all' : 'Collapse all' }}
          </button>
          <div class="inline-flex items-center p-0.5 rounded-lg bg-zinc-100 dark:bg-white/5 ring-1 ring-zinc-950/5 dark:ring-white/10" role="group" aria-label="Feed view">
            <button
              type="button"
              :aria-pressed="viewMode === 'stream'"
              class="rounded-md px-2.5 py-1 text-xs font-medium transition-all duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/70"
              :class="viewMode === 'stream' ? 'bg-white text-zinc-900 shadow-sm dark:bg-zinc-800 dark:text-white font-semibold' : 'text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'"
              @click="viewMode = 'stream'; router.replace({ query: serialiseFilters(feed.filters) })"
            >
              Stream
            </button>
            <button
              type="button"
              :aria-pressed="viewMode === 'video'"
              class="rounded-md px-2.5 py-1 text-xs font-medium transition-all duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/70"
              :class="viewMode === 'video' ? 'bg-white text-zinc-900 shadow-sm dark:bg-zinc-800 dark:text-white font-semibold' : 'text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'"
              @click="viewMode = 'video'; router.replace({ query: serialiseFilters(feed.filters) })"
            >
              Group by Video
            </button>
          </div>
        </div>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-4 gap-6">
      <div class="order-1 lg:order-1 lg:col-span-3 space-y-4">
        <!-- Export result surface (fix-frontend-plumbing) -->
        <div
          v-if="feed.exportJob"
          class="rounded-xl ring-1 ring-zinc-950/5 dark:ring-white/10 p-4 bg-white dark:bg-zinc-900 shadow-card"
        >
          <div class="flex items-center gap-3">
            <span
              class="inline-block h-2 w-2 rounded-full ring-4 ring-transparent"
              :class="
                feed.exportJob.status === 'completed'
                  ? 'bg-emerald-500 ring-emerald-500/20'
                  : feed.exportJob.status === 'failed'
                    ? 'bg-rose-500 ring-rose-500/20'
                    : 'bg-amber-400 ring-amber-400/20 animate-pulse'
              "
            ></span>
            <p class="text-sm font-medium text-zinc-900 dark:text-white">
              Export
              <span class="capitalize">{{ feed.exportJob.status }}</span>
            </p>
            <div class="ml-auto flex items-center gap-2">
              <a
                v-if="feed.exportJob.status === 'completed' && (feed.exportJob.download_url || feed.exportJob.export_url)"
                :href="feed.exportJob.download_url ?? feed.exportJob.export_url!"
                class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold text-white bg-zinc-900 dark:bg-white dark:text-zinc-900 hover:bg-zinc-700 dark:hover:bg-zinc-200 transition-all duration-200 active:scale-95"
              >
                <Download class="w-3.5 h-3.5" /> Download
              </a>
              <button
                type="button"
                @click="feed.clearExportJob()"
                class="text-xs font-medium text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300 transition-colors"
              >
                Dismiss
              </button>
            </div>
          </div>
          <p v-if="feed.exportJob.status === 'failed' && feed.exportJob.error" class="mt-1 text-xs text-rose-600 dark:text-rose-400">
            {{ feed.exportJob.error }}
          </p>
          <p v-else-if="feed.exportJob.expires_at" class="mt-1 text-[11px] text-zinc-500 dark:text-zinc-400">
            Link expires {{ shortDate(feed.exportJob.expires_at) }}
          </p>
        </div>

        <div v-if="feed.isInitialLoad" class="space-y-3" role="status" aria-label="Loading comments">
          <div
            v-for="i in 4"
            :key="i"
            class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl h-28 animate-pulse"
          ></div>
        </div>

        <div v-else-if="feed.comments.length === 0">
          <Card padding="none" class="p-14 text-center">
            <div class="mx-auto w-12 h-12 rounded-xl bg-zinc-50 dark:bg-white/5 ring-1 ring-zinc-950/5 dark:ring-white/10 flex items-center justify-center text-zinc-400 dark:text-zinc-500 mb-4">
              <MessageSquare class="w-5 h-5" />
            </div>
            <p class="text-sm font-semibold text-zinc-900 dark:text-white">No comments match these filters.</p>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1.5">Try resetting filters or widening the date range.</p>
          </Card>
        </div>

        <TransitionGroup v-else tag="div" name="comment-list" class="space-y-3">
          <template v-if="viewMode === 'stream'">
            <CommentCard v-for="(c, idx) in feed.comments" :key="c.id" :comment="c" :is-focused="idx === focusedIndex" @toggle-star="(id, next) => (next ? feed.starComment(id) : feed.unstarComment(id))" />
          </template>
          <section v-for="group in groupedComments" v-else :key="group.id" class="space-y-3">
            <div class="flex items-center justify-between gap-3 rounded-xl bg-zinc-50 dark:bg-white/5 ring-1 ring-zinc-950/5 dark:ring-white/10 px-4 py-2.5 transition-colors duration-150">
              <button
                type="button"
                class="flex-1 text-left min-w-0 flex items-center gap-2.5 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/70 rounded-lg p-0.5 group"
                :aria-expanded="!collapsedVideos.has(group.id)"
                @click="toggleVideo(group.id)"
              >
                <ChevronRight
                  class="w-4 h-4 text-zinc-400 group-hover:text-zinc-700 dark:group-hover:text-zinc-200 transition-transform duration-200 shrink-0"
                  :class="!collapsedVideos.has(group.id) ? 'rotate-90' : ''"
                  aria-hidden="true"
                />
                <span class="truncate text-sm font-semibold text-zinc-900 dark:text-white">{{ group.title }}</span>
                <span class="shrink-0 inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium bg-zinc-200/60 dark:bg-white/10 text-zinc-600 dark:text-zinc-300 tabular-nums">
                  {{ group.comments.length }} comments · {{ group.insightCount }} insights
                </span>
              </button>
              <a
                v-if="group.videoId"
                :href="`https://www.youtube.com/watch?v=${group.videoId}`"
                target="_blank"
                rel="noopener"
                class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-xs font-medium text-zinc-500 dark:text-zinc-400 hover:text-brand-600 dark:hover:text-brand-400 hover:bg-zinc-100 dark:hover:bg-white/5 transition-colors shrink-0"
                @click.stop
                aria-label="Open video on YouTube"
              >
                <span>Open video</span>
                <ExternalLink class="w-3.5 h-3.5" aria-hidden="true" />
              </a>
            </div>
            <div v-if="!collapsedVideos.has(group.id)" class="space-y-3">
              <CommentCard
                v-for="c in group.comments"
                :key="c.id"
                :comment="c"
                :is-focused="commentIndexMap.get(c.id) === focusedIndex"
                :hide-video="true"
                @toggle-star="(id, next) => (next ? feed.starComment(id) : feed.unstarComment(id))"
              />
            </div>
          </section>
        </TransitionGroup>

        <div ref="sentinel" class="h-10 flex items-center justify-center">
          <span v-if="feed.loadingMore" class="text-xs text-zinc-500 dark:text-zinc-400">Loading more…</span>
          <span v-else-if="!feed.hasMore && feed.comments.length > 0" class="text-xs text-zinc-400">
            End of feed
          </span>
        </div>
      </div>

      <aside class="order-2 lg:order-2 lg:col-span-1 space-y-4">
        <Card class="p-5 space-y-5">
          <div class="flex items-center gap-3.5">
            <ToxicityShield :data="insights.toxicity" :window-key="toxicityWindow" compact />
            <div>
              <h2 class="text-xs font-semibold text-zinc-900 dark:text-white">Toxicity shield</h2>
              <p class="text-[11px] text-zinc-500 dark:text-zinc-400 mt-0.5">
                {{ insights.toxicity ? `${insights.toxicity.filtered_comments.toLocaleString()} filtered` : '—' }}
              </p>
            </div>
          </div>

          <div class="hidden lg:block space-y-5 pt-4 border-t border-zinc-100 dark:border-white/5">
            <SentimentChart :data="insights.sentiment" flat />

            <div
              v-if="insights.actionableInsights && (insights.actionableInsights.distribution?.length ?? 0) > 0"
              class="pt-4 border-t border-zinc-100 dark:border-white/5"
            >
              <InsightTypeDistributionChart
                :distribution="insights.actionableInsights.distribution"
                flat
                @select-type="selectInsightType"
              />
            </div>

            <div
              v-if="insights.trends && insights.trends.top_topics.length > 0"
              class="pt-4 border-t border-zinc-100 dark:border-white/5 space-y-2.5"
            >
              <h2 class="text-xs font-semibold text-zinc-900 dark:text-white">Top topics</h2>
              <div class="flex flex-wrap gap-1.5">
                <button
                  v-for="t in insights.trends.top_topics.slice(0, 12)"
                  :key="t.topic"
                  class="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium bg-zinc-50 dark:bg-white/5 text-zinc-600 dark:text-zinc-300 ring-1 ring-inset ring-zinc-500/10 dark:ring-white/10 transition-colors hover:bg-brand-50 dark:hover:bg-brand-400/10 hover:text-brand-700 dark:hover:text-brand-300"
                  type="button"
                  @click="selectTopic(t.topic)"
                >
                  {{ t.topic }}
                  <span class="ml-1 text-zinc-400 dark:text-zinc-500 font-mono tabular-nums">{{ t.count }}</span>
                </button>
              </div>
            </div>
          </div>
        </Card>

        <div class="hidden lg:block">
          <WeeklyDigestCard :digest="insights.latestDigest" :loading="insights.loadingDigest" />
        </div>
      </aside>
    </div>

    <Teleport to="body">
      <div v-if="exportOpen" class="fixed z-50 inset-0 overflow-y-auto" role="dialog" aria-modal="true">
        <div class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
          <div class="fixed inset-0 bg-zinc-950/50 backdrop-blur-sm dark:bg-black/60" aria-hidden="true" @click="exportOpen = false"></div>
          <span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>
          <div class="relative z-10 inline-block align-bottom bg-white dark:bg-zinc-900 rounded-xl px-4 pt-5 pb-6 text-left overflow-hidden shadow-pop ring-1 ring-zinc-950/5 dark:ring-white/10 transform transition-all sm:my-8 sm:align-middle sm:max-w-md sm:w-full sm:p-8">
            <div class="flex items-start justify-between">
              <div>
                <h3 class="text-xl font-bold tracking-tight text-zinc-900 dark:text-white">Export comments</h3>
                <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">Exports your current filtered view.</p>
              </div>
              <button
                type="button"
                aria-label="Close"
                @click="exportOpen = false"
                class="p-2 rounded-lg text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-white/5 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 transition-colors"
              >
                <X class="w-4 h-4" />
              </button>
            </div>
            <div class="mt-5">
              <label for="export-format" class="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">Format</label>
              <select
                id="export-format"
                v-model="exportFormat"
                class="appearance-none block w-full px-4 py-2.5 border border-zinc-200 dark:border-white/10 rounded-xl bg-white dark:bg-zinc-800/60 text-sm text-zinc-900 dark:text-zinc-100 shadow-sm transition-all duration-200 hover:border-zinc-300 dark:hover:border-white/20 focus:outline-none focus:ring-2 focus:ring-brand-500/30 focus:border-brand-500/70"
              >
                <option value="csv">CSV</option>
                <option value="json">JSON</option>
              </select>
            </div>
            <div class="mt-6 flex flex-row-reverse gap-3">
              <button
                type="button"
                :disabled="exportSubmitting"
                @click="submitExport"
                class="inline-flex justify-center items-center gap-1.5 py-2.5 px-5 rounded-xl text-sm font-semibold text-white bg-zinc-900 dark:bg-white dark:text-zinc-900 hover:bg-zinc-700 dark:hover:bg-zinc-200 shadow-card dark:shadow-none transition-all duration-200 ease-snappy active:scale-[0.98] disabled:opacity-50 disabled:pointer-events-none focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
              >
                <Download class="w-4 h-4" />
                {{ exportSubmitting ? 'Working…' : 'Start export' }}
              </button>
              <button
                type="button"
                @click="exportOpen = false"
                class="inline-flex justify-center items-center py-2.5 px-5 rounded-xl text-sm font-semibold text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <KeyboardHelpModal :visible="helpVisible" @close="helpVisible = false" />
  </div>
</template>

<style scoped>
.comment-list-enter-active,
.comment-list-leave-active,
.comment-list-move {
  transition: opacity 0.25s cubic-bezier(0.16, 1, 0.3, 1), transform 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.comment-list-enter-from,
.comment-list-leave-to {
  opacity: 0;
  transform: translateY(10px) scale(0.995);
}
.comment-list-leave-active {
  position: absolute;
}
</style>
