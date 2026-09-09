<script setup lang="ts">
// Insights feed (/dashboard) — primary working surface.
//
// Composes: FilterBar + 4 summary cards + infinite-scroll CommentCard list +
// (desktop) sidebar with WeeklyDigestCard / topic chips / InsightTypeDistributionChart.
// Pollers: feed at 60 s, insights at 5 min — both visibility-gated.

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Download, MessageSquare, TrendingUp, X } from 'lucide-vue-next'
import { RouterLink, useRoute } from 'vue-router'
import { useFeedStore } from '../../stores/feed'
import { useInsightsStore } from '../../stores/insights'
import { useChannelsStore } from '../../stores/channels'
import { useBillingStore } from '../../stores/billing'
import { useFilterUrlSync } from '../../lib/useFilterUrlSync'
import { usePolling } from '../../lib/usePolling'
import { useKeyboardShortcuts } from '../../composables/useKeyboardShortcuts'
import { useWorkspaceRole } from '../../composables/useWorkspaceRole'
import { CATEGORIES, HIGH_SIGNAL_CATEGORIES, type Category } from '../../lib/categories'
import { shortDate } from '../../lib/relativeTime'
import type { ExportFormat, FeedFilters, InsightType, WindowKey } from '../../services/types'
import Card from '../../components/Card.vue'
import CommentCard from '../../components/CommentCard.vue'
import FilterBar from '../../components/FilterBar.vue'
import ActionableInsightsPanel from '../../components/ActionableInsightsPanel.vue'
import InsightTypeDistributionChart from '../../components/InsightTypeDistributionChart.vue'
import SentimentChart from '../../components/SentimentChart.vue'
import SentimentSparkline from '../../components/SentimentSparkline.vue'
import ToxicityShield from '../../components/ToxicityShield.vue'
import UsageBar from '../../components/UsageBar.vue'
import WeeklyDigestCard from '../../components/WeeklyDigestCard.vue'
import KeyboardHelpModal from '../../components/KeyboardHelpModal.vue'

const feed = useFeedStore()
const insights = useInsightsStore()
const channels = useChannelsStore()
const billing = useBillingStore()
const route = useRoute()
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

const serialiseFilters = (f: FeedFilters): Record<string, string> => {
  const q: Record<string, string> = {}
  if (f.channels.length > 0) q.channels = f.channels.join(',')
  const sortedCats = [...f.categories].sort().join(',')
  const defaultCats = [...HIGH_SIGNAL_CATEGORIES].sort().join(',')
  if (sortedCats !== defaultCats) q.categories = f.categories.join(',')
  if (f.insight_type) q.insight_type = f.insight_type
  if (f.date_from) q.date_from = f.date_from
  if (f.date_to) q.date_to = f.date_to
  if (f.search) q.search = f.search
  if (f.include_toxic) q.include_toxic = '1'
  if (f.sort && f.sort !== 'newest') q.sort = f.sort
  return q
}

const parseFilters = (query: Record<string, unknown>): Partial<FeedFilters> => {
  const out: Partial<FeedFilters> = {}
  const channelsRaw = asString(query.channels)
  if (channelsRaw) out.channels = channelsRaw.split(',').filter(Boolean)
  const catsRaw = asString(query.categories)
  if (catsRaw) {
    out.categories = catsRaw
      .split(',')
      .filter((c): c is Category => (CATEGORIES as readonly string[]).includes(c))
  }
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
    feed.applyFilters(parseFilters(q as Record<string, unknown>))
    void feed.fetchFeed({ reset: true })
  },
)

function onUpdateFilters(partial: Partial<FeedFilters>): void {
  feed.applyFilters(partial)
  void feed.fetchFeed({ reset: true })
}

function onResetFilters(): void {
  feed.resetFilters()
  void feed.fetchFeed({ reset: true })
}

const insightsWindow = ref<WindowKey>('7d')

async function refreshInsights(force = false): Promise<void> {
  await insights.fetchActionableInsights(undefined, insightsWindow.value, { force })
}

function onInsightsWindowChange(w: WindowKey): void {
  insightsWindow.value = w
  void refreshInsights(false)
}

// Sentiment trend — the backend accepts an optional channel_id: with a
// channel filter selected the chart is scoped to that channel, without one
// it aggregates across every tracked channel (global trend).
// `fetchSentiment` is guarded for test doubles that predate the action.
const sentimentDays = ref(7)
const sentimentChannelId = computed(() => feed.filters.channels[0] ?? null)

async function refreshSentiment(force = false): Promise<void> {
  await insights.fetchSentiment?.(sentimentChannelId.value ?? undefined, sentimentDays.value, { force })
}

function onSentimentDaysChange(days: number): void {
  sentimentDays.value = days
  void refreshSentiment(false)
}

watch(sentimentChannelId, () => {
  void refreshSentiment(true)
})

const totalCommentsCount = computed(() => insights.actionableInsights?.total_insights ?? null)

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

function setupObserver(): void {
  if (!sentinel.value || typeof IntersectionObserver === 'undefined') return
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting && feed.hasMore && !feed.loadingMore && !feed.loading) {
          void feed.loadMore()
        }
      }
    },
    { rootMargin: '200px' },
  )
  observer.observe(sentinel.value)
}

function teardownObserver(): void {
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

const { helpVisible } = useKeyboardShortcuts({
  onStar: starFocused,
  onNavigateDown: navigateDown,
  onNavigateUp: navigateUp,
  onFocusSearch: focusSearch,
})

onMounted(async () => {
  void channels.fetchAll()
  void billing.fetchStatus()
  void feed.fetchFeed({ reset: true })
  void refreshInsights(true)
  void insights.fetchToxicity(undefined, '7d', { force: true })
  void insights.fetchTrends(undefined, 7, { force: true })
  void insights.fetchLatestDigest({ force: true })
  void refreshSentiment(true)

  await Promise.resolve()
  setupObserver()
})

onBeforeUnmount(() => {
  teardownObserver()
  feed.stopExportPolling()
})

usePolling(() => feed.fetchFeed({ reset: true }), 60_000)
usePolling(async () => {
  await Promise.all([
    insights.fetchActionableInsights(undefined, insightsWindow.value, { force: true }),
    insights.fetchToxicity(undefined, '7d', { force: true }),
    insights.fetchTrends(undefined, 7, { force: true }),
    refreshSentiment(true),
  ])
}, 5 * 60_000)
</script>

<template>
  <div class="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
    <div class="md:flex md:items-center md:justify-between mb-6">
      <div class="flex-1 min-w-0">
        <h1 class="text-2xl font-semibold leading-7 text-zinc-900 dark:text-zinc-100 sm:text-3xl sm:tracking-tight">
          Insights feed
        </h1>
        <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
          Real-time signals across your channels.
        </p>
      </div>
      <button
        type="button"
        @click="helpVisible = true"
        class="mt-4 md:mt-0 inline-flex items-center gap-1.5 text-xs font-medium text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 px-2.5 py-1.5 rounded-lg hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
      >
        <kbd class="inline-flex items-center justify-center min-w-[1.25rem] px-1 py-0.5 text-[10px] font-mono font-semibold bg-zinc-100 dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded">?</kbd>
        Keyboard shortcuts
      </button>
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      <Card>
        <div class="flex items-center gap-2 text-zinc-500 dark:text-zinc-400 text-xs font-medium uppercase tracking-wider">
          <MessageSquare class="w-3.5 h-3.5" /> Insights ({{ insightsWindow }})
        </div>
        <p class="mt-2 text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 font-mono tabular-nums">
          {{ totalCommentsCount !== null ? totalCommentsCount.toLocaleString() : '—' }}
        </p>
      </Card>

      <Card class="flex items-center gap-3">
        <ToxicityShield :data="insights.toxicity" window-key="7d" compact />
        <div>
          <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider">Toxicity shield</p>
          <p class="text-[11px] text-zinc-500 dark:text-zinc-400 mt-1">
            {{ insights.toxicity ? `${insights.toxicity.filtered_comments.toLocaleString()} filtered` : '—' }}
          </p>
        </div>
      </Card>

      <Card>
        <div class="flex items-center gap-2 text-zinc-500 dark:text-zinc-400 text-xs font-medium uppercase tracking-wider mb-2">
          Quota
        </div>
        <UsageBar
          v-if="billing.status"
          :used="billing.status.comments_used"
          :limit="billing.status.comments_limit"
          unit="comments"
        />
        <p v-else class="text-xs text-zinc-500 dark:text-zinc-400">Loading…</p>
        <RouterLink
          v-if="billing.nearLimit && billing.status?.plan === 'free'"
          to="/settings/billing"
          class="mt-2 inline-flex items-center text-xs font-semibold text-brand-600 dark:text-brand-400 hover:underline"
        >
          Upgrade plan →
        </RouterLink>
      </Card>

      <Card>
        <div class="flex items-center gap-2 text-zinc-500 dark:text-zinc-400 text-xs font-medium uppercase tracking-wider">
          <TrendingUp class="w-3.5 h-3.5" /> Top insight type
        </div>
        <p class="mt-2 text-lg font-semibold tracking-tight text-zinc-900 dark:text-zinc-100">
          {{
            insights.actionableInsights?.distribution?.length
              ? insights.actionableInsights.distribution.sort((a, b) => b.count - a.count)[0].type.replace(/_/g, ' ')
              : '—'
          }}
        </p>
        <p class="text-[11px] text-zinc-500 dark:text-zinc-400 mt-1">
          {{
            insights.actionableInsights?.distribution?.length
              ? `${insights.actionableInsights.distribution.sort((a, b) => b.count - a.count)[0].count} mentions`
              : 'vs prior ' + insightsWindow
          }}
        </p>
      </Card>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-4 gap-6">
      <div class="lg:col-span-3 space-y-4">
        <div class="sticky top-16 z-20">
          <FilterBar
            ref="filterBarRef"
            :filters="feed.filters"
            :channels="channels.channels"
            variant="feed"
            @update:filters="onUpdateFilters"
            @export="canExport && (exportOpen = true)"
            @reset="onResetFilters"
          />
        </div>

        <!-- Export result surface (fix-frontend-plumbing) -->
        <div
          v-if="feed.exportJob"
          class="rounded-xl ring-1 ring-zinc-200 dark:ring-zinc-700 p-4 bg-white dark:bg-zinc-900 shadow-sm"
        >
          <div class="flex items-center gap-3">
            <span
              class="inline-block h-2 w-2 rounded-full"
              :class="
                feed.exportJob.status === 'completed'
                  ? 'bg-emerald-500'
                  : feed.exportJob.status === 'failed'
                    ? 'bg-rose-500'
                    : 'bg-amber-500 animate-pulse'
              "
            ></span>
            <p class="text-sm font-medium text-zinc-900 dark:text-zinc-100">
              Export
              <span class="capitalize">{{ feed.exportJob.status }}</span>
            </p>
            <div class="ml-auto flex items-center gap-2">
              <a
                v-if="feed.exportJob.status === 'completed' && (feed.exportJob.download_url || feed.exportJob.export_url)"
                :href="feed.exportJob.download_url ?? feed.exportJob.export_url!"
                class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-white bg-zinc-900 dark:bg-zinc-100 dark:text-zinc-900 hover:bg-zinc-800"
              >
                <Download class="w-3.5 h-3.5" /> Download
              </a>
              <button
                type="button"
                @click="feed.clearExportJob()"
                class="text-xs text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300"
              >
                Dismiss
              </button>
            </div>
          </div>
          <p v-if="feed.exportJob.status === 'failed' && feed.exportJob.error" class="mt-1 text-xs text-rose-600">
            {{ feed.exportJob.error }}
          </p>
          <p v-else-if="feed.exportJob.expires_at" class="mt-1 text-[11px] text-zinc-500 dark:text-zinc-400">
            Link expires {{ shortDate(feed.exportJob.expires_at) }}
          </p>
        </div>

        <ActionableInsightsPanel
          :data="insights.actionableInsights"
          :window-key="insightsWindow"
          @update:window-key="onInsightsWindowChange"
        />

        <SentimentChart :data="insights.sentiment" @update:days="onSentimentDaysChange" />

        <div v-if="feed.isInitialLoad" class="space-y-3" role="status" aria-label="Loading comments">
          <div
            v-for="i in 4"
            :key="i"
            class="bg-white dark:bg-zinc-900 shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl h-28 animate-pulse"
          ></div>
        </div>

        <div v-else-if="feed.comments.length === 0">
          <Card padding="none" class="p-10 text-center">
            <p class="text-sm font-medium text-zinc-900 dark:text-zinc-100">No comments match these filters.</p>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1">Try resetting filters or widening the date range.</p>
          </Card>
        </div>

        <TransitionGroup v-else tag="div" name="comment-list" class="space-y-3">
          <CommentCard
            v-for="(c, idx) in feed.comments"
            :key="c.id"
            :comment="c"
            :is-focused="idx === focusedIndex"
            @toggle-star="(id, next) => (next ? feed.starComment(id) : feed.unstarComment(id))"
          />
        </TransitionGroup>

        <div ref="sentinel" class="h-10 flex items-center justify-center">
          <span v-if="feed.loadingMore" class="text-xs text-zinc-500 dark:text-zinc-400">Loading more…</span>
          <span v-else-if="!feed.hasMore && feed.comments.length > 0" class="text-xs text-zinc-400">
            End of feed
          </span>
        </div>
      </div>

      <aside class="hidden lg:block lg:col-span-1 space-y-4">
        <WeeklyDigestCard :digest="insights.latestDigest" :loading="insights.loadingDigest" />

        <section
          v-if="insights.trends && insights.trends.top_topics.length > 0"
          class="space-y-0"
        >
          <Card padding="none" class="p-5">
            <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider mb-2">Top topics</p>
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="t in insights.trends.top_topics.slice(0, 12)"
                :key="t.topic"
                class="inline-flex items-center px-2 py-0.5 rounded-md text-xs bg-zinc-50 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300 ring-1 ring-inset ring-zinc-500/10 dark:ring-zinc-400/10"
              >
                {{ t.topic }}
                <span class="ml-1 text-zinc-500 dark:text-zinc-400 font-mono tabular-nums">{{ t.count }}</span>
              </span>
            </div>
          </Card>
        </section>

        <InsightTypeDistributionChart
          v-if="insights.actionableInsights && (insights.actionableInsights.distribution?.length ?? 0) > 0"
          :distribution="insights.actionableInsights.distribution"
        />

        <SentimentSparkline :data="insights.sentiment" />
      </aside>
    </div>

    <Teleport to="body">
      <div v-if="exportOpen" class="fixed z-50 inset-0 overflow-y-auto" role="dialog" aria-modal="true">
        <div class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
          <div class="fixed inset-0 bg-zinc-900/60 backdrop-blur-sm" aria-hidden="true" @click="exportOpen = false"></div>
          <span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>
          <div class="relative z-10 inline-block align-bottom bg-white dark:bg-zinc-900 rounded-xl px-4 pt-5 pb-6 text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-md sm:w-full sm:p-8">
            <div class="flex items-start justify-between">
              <div>
                <h3 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">Export comments</h3>
                <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400">Exports your current filtered view.</p>
              </div>
              <button
                type="button"
                aria-label="Close"
                @click="exportOpen = false"
                class="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-zinc-900"
              >
                <X class="w-4 h-4" />
              </button>
            </div>
            <div class="mt-5">
              <label for="export-format" class="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">Format</label>
              <select
                id="export-format"
                v-model="exportFormat"
                class="block w-full px-3 py-2.5 border border-zinc-200 dark:border-zinc-700 rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 bg-zinc-50/50 dark:bg-zinc-800 dark:text-zinc-100"
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
                class="inline-flex justify-center items-center gap-1.5 py-2.5 px-5 rounded-xl text-sm font-medium text-white bg-zinc-900 dark:bg-zinc-100 dark:text-zinc-900 hover:bg-zinc-800 dark:hover:bg-zinc-200 disabled:opacity-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900"
              >
                <Download class="w-4 h-4" />
                {{ exportSubmitting ? 'Working…' : 'Start export' }}
              </button>
              <button
                type="button"
                @click="exportOpen = false"
                class="inline-flex justify-center items-center py-2.5 px-5 rounded-xl text-sm font-medium text-zinc-700 dark:text-zinc-300 bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 hover:bg-zinc-50 dark:hover:bg-zinc-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900"
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
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.comment-list-enter-from,
.comment-list-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
.comment-list-leave-active {
  position: absolute;
}
</style>
