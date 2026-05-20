<script setup lang="ts">
// Insights feed (/dashboard) — primary working surface.
//
// Composes: FilterBar + 4 summary cards + infinite-scroll CommentCard list +
// (desktop) sidebar with WeeklyDigestCard / topic chips / SentimentSparkline.
// Pollers: feed at 60 s, insights at 5 min — both visibility-gated.

import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Download, MessageSquare, TrendingDown, TrendingUp, Minus, X } from 'lucide-vue-next'
import { useFeedStore } from '../../stores/feed'
import { useInsightsStore } from '../../stores/insights'
import { useChannelsStore } from '../../stores/channels'
import { useBillingStore } from '../../stores/billing'
import { useFilterUrlSync } from '../../lib/useFilterUrlSync'
import { usePolling } from '../../lib/usePolling'
import { CATEGORIES, HIGH_SIGNAL_CATEGORIES, type Category } from '../../lib/categories'
import type { ExportFormat, FeedFilters, WindowKey } from '../../services/types'
import CommentCard from '../../components/CommentCard.vue'
import FilterBar from '../../components/FilterBar.vue'
import SentimentChart from '../../components/SentimentChart.vue'
import SentimentSparkline from '../../components/SentimentSparkline.vue'
import ToxicityShield from '../../components/ToxicityShield.vue'
import UsageBar from '../../components/UsageBar.vue'
import WeeklyDigestCard from '../../components/WeeklyDigestCard.vue'

const feed = useFeedStore()
const insights = useInsightsStore()
const channels = useChannelsStore()
const billing = useBillingStore()

// ---- URL ↔ filter sync -----------------------------------------------------
const VALID_SORTS: FeedFilters['sort'][] = ['newest', 'sentiment_desc', 'sentiment_asc']

function asString(v: unknown): string | null {
  if (typeof v === 'string') return v
  if (Array.isArray(v) && typeof v[0] === 'string') return v[0]
  return null
}

useFilterUrlSync<FeedFilters>({
  get: () => feed.filters,
  apply: (partial) => feed.applyFilters(partial),
  serialise: (f) => {
    const q: Record<string, string> = {}
    if (f.channels.length > 0) q.channels = f.channels.join(',')
    // Only emit `categories` if it differs from the default (high-signal six).
    const sortedCats = [...f.categories].sort().join(',')
    const defaultCats = [...HIGH_SIGNAL_CATEGORIES].sort().join(',')
    if (sortedCats !== defaultCats) q.categories = f.categories.join(',')
    if (f.sentiment_min !== null && f.sentiment_min > -1) q.sentiment_min = String(f.sentiment_min)
    if (f.sentiment_max !== null && f.sentiment_max < 1) q.sentiment_max = String(f.sentiment_max)
    if (f.date_from) q.date_from = f.date_from
    if (f.date_to) q.date_to = f.date_to
    if (f.search) q.search = f.search
    if (f.include_toxic) q.include_toxic = '1'
    if (f.sort && f.sort !== 'newest') q.sort = f.sort
    return q
  },
  parse: (query) => {
    const out: Partial<FeedFilters> = {}
    const channelsRaw = asString(query.channels)
    if (channelsRaw) out.channels = channelsRaw.split(',').filter(Boolean)
    const catsRaw = asString(query.categories)
    if (catsRaw) {
      out.categories = catsRaw
        .split(',')
        .filter((c): c is Category => (CATEGORIES as readonly string[]).includes(c))
    }
    const sMin = asString(query.sentiment_min)
    if (sMin !== null) {
      const n = Number(sMin)
      if (Number.isFinite(n)) out.sentiment_min = n
    }
    const sMax = asString(query.sentiment_max)
    if (sMax !== null) {
      const n = Number(sMax)
      if (Number.isFinite(n)) out.sentiment_max = n
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
  },
})

// ---- filter handlers ------------------------------------------------------
function onUpdateFilters(partial: Partial<FeedFilters>): void {
  feed.applyFilters(partial)
  void feed.fetchFeed({ reset: true })
}

function onResetFilters(): void {
  feed.resetFilters()
  void feed.fetchFeed({ reset: true })
}

// ---- summary cards --------------------------------------------------------
const sentimentWindow = ref<WindowKey>('7d')

async function refreshSentiment(force = false): Promise<void> {
  await insights.fetchSentiment(undefined, sentimentWindow.value, { force })
}

function onSentimentWindowChange(w: WindowKey): void {
  sentimentWindow.value = w
  void refreshSentiment(false)
}

const sentimentTrendDir = computed<'up' | 'down' | 'flat'>(() => {
  const s = insights.sentiment
  if (!s) return 'flat'
  // Compare current avg vs first half of series as a coarse "vs prior period"
  // proxy when the backend doesn't supply a delta.
  const series = s.series ?? []
  if (series.length < 4) return 'flat'
  const half = Math.floor(series.length / 2)
  const first = series.slice(0, half).reduce((a, b) => a + (b.avg ?? 0), 0) / Math.max(1, half)
  const second = series.slice(half).reduce((a, b) => a + (b.avg ?? 0), 0) / Math.max(1, series.length - half)
  const delta = second - first
  if (Math.abs(delta) < 0.05) return 'flat'
  return delta > 0 ? 'up' : 'down'
})

const totalCommentsWindow = computed(() => insights.sentiment?.count ?? null)

// ---- export modal --------------------------------------------------------
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

// ---- infinite scroll -----------------------------------------------------
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

// ---- mount ---------------------------------------------------------------
onMounted(async () => {
  // Kick off all reads in parallel; channels needed for FilterBar chips.
  void channels.fetchAll()
  void billing.fetchStatus()
  void feed.fetchFeed({ reset: true })
  void refreshSentiment(true)
  void insights.fetchToxicity(undefined, '7d', { force: true })
  void insights.fetchTrends(undefined, 7, { force: true })
  void insights.fetchLatestDigest({ force: true })

  // Wait a microtask for the sentinel ref to be in place before observing.
  await Promise.resolve()
  setupObserver()
})

onBeforeUnmount(teardownObserver)

// Pollers — usePolling auto-tears-down via onBeforeUnmount.
usePolling(() => feed.fetchFeed({ reset: true }), 60_000)
usePolling(async () => {
  await Promise.all([
    insights.fetchSentiment(undefined, sentimentWindow.value, { force: true }),
    insights.fetchToxicity(undefined, '7d', { force: true }),
    insights.fetchTrends(undefined, 7, { force: true }),
  ])
}, 5 * 60_000)
</script>

<template>
  <div class="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
    <!-- Header -->
    <div class="md:flex md:items-center md:justify-between mb-6">
      <div class="flex-1 min-w-0">
        <h2 class="text-2xl font-semibold leading-7 text-zinc-900 sm:text-3xl sm:tracking-tight">
          Insights feed
        </h2>
        <p class="mt-1 text-sm text-zinc-500">
          Real-time signals across your channels.
        </p>
      </div>
    </div>

    <!-- Summary cards -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      <!-- Total comments (window) -->
      <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5">
        <div class="flex items-center gap-2 text-zinc-500 text-xs font-medium uppercase tracking-wider">
          <MessageSquare class="w-3.5 h-3.5" /> Comments ({{ sentimentWindow }})
        </div>
        <p class="mt-2 text-2xl font-semibold tracking-tight text-zinc-900 tabular-nums">
          {{ totalCommentsWindow !== null ? totalCommentsWindow.toLocaleString() : '—' }}
        </p>
      </div>

      <!-- Avg sentiment + trend -->
      <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5">
        <div class="flex items-center gap-2 text-zinc-500 text-xs font-medium uppercase tracking-wider">
          Avg sentiment
        </div>
        <div class="mt-2 flex items-center gap-2">
          <p class="text-2xl font-semibold tracking-tight text-zinc-900 tabular-nums">
            {{ insights.sentiment ? (insights.sentiment.avg ?? 0).toFixed(2) : '—' }}
          </p>
          <span
            v-if="insights.sentiment"
            :class="[
              'inline-flex items-center gap-0.5 text-xs font-medium px-1.5 py-0.5 rounded-md',
              sentimentTrendDir === 'up'
                ? 'bg-emerald-50 text-emerald-700'
                : sentimentTrendDir === 'down'
                ? 'bg-rose-50 text-rose-700'
                : 'bg-zinc-100 text-zinc-600',
            ]"
          >
            <TrendingUp v-if="sentimentTrendDir === 'up'" class="w-3 h-3" />
            <TrendingDown v-else-if="sentimentTrendDir === 'down'" class="w-3 h-3" />
            <Minus v-else class="w-3 h-3" />
            {{ sentimentTrendDir }}
          </span>
        </div>
        <p class="text-[11px] text-zinc-400 mt-1">vs prior {{ sentimentWindow }}</p>
      </div>

      <!-- Toxicity shield donut -->
      <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5 flex items-center gap-3">
        <ToxicityShield :data="insights.toxicity" window-key="7d" compact />
        <div>
          <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider">Toxicity shield</p>
          <p class="text-[11px] text-zinc-400 mt-1">
            {{ insights.toxicity ? `${insights.toxicity.filtered_comments.toLocaleString()} filtered` : '—' }}
          </p>
        </div>
      </div>

      <!-- Quota -->
      <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5">
        <div class="flex items-center gap-2 text-zinc-500 text-xs font-medium uppercase tracking-wider mb-2">
          Quota
        </div>
        <UsageBar
          v-if="billing.status"
          :used="billing.status.comments_used"
          :limit="billing.status.comments_limit"
          unit="comments"
        />
        <p v-else class="text-xs text-zinc-400">Loading…</p>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-4 gap-6">
      <!-- Main column: filters + feed -->
      <div class="lg:col-span-3 space-y-4">
        <!-- Sticky FilterBar (sticks under the topbar; topbar h-16 = 64px) -->
        <div class="sticky top-16 z-20">
          <FilterBar
            :filters="feed.filters"
            :channels="channels.channels"
            variant="feed"
            @update:filters="onUpdateFilters"
            @export="exportOpen = true"
            @reset="onResetFilters"
          />
        </div>

        <!-- Sentiment chart full width on this column -->
        <SentimentChart
          :series="insights.sentiment?.series ?? []"
          :window-key="sentimentWindow"
          @update:window-key="onSentimentWindowChange"
        />

        <!-- Comment list -->
        <div v-if="feed.isInitialLoad" class="space-y-3" aria-label="Loading comments">
          <div
            v-for="i in 4"
            :key="i"
            class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl h-28 animate-pulse"
          ></div>
        </div>

        <div v-else-if="feed.comments.length === 0" class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-10 text-center">
          <p class="text-sm font-medium text-zinc-900">No comments match these filters.</p>
          <p class="text-xs text-zinc-500 mt-1">Try resetting filters or widening the date range.</p>
        </div>

        <div v-else class="space-y-3">
          <CommentCard
            v-for="c in feed.comments"
            :key="c.id"
            :comment="c"
            @toggle-star="(id, next) => (next ? feed.starComment(id) : feed.unstarComment(id))"
          />
        </div>

        <!-- Sentinel + load-more spinner -->
        <div ref="sentinel" class="h-10 flex items-center justify-center">
          <span v-if="feed.loadingMore" class="text-xs text-zinc-500">Loading more…</span>
          <span v-else-if="!feed.hasMore && feed.comments.length > 0" class="text-xs text-zinc-400">
            End of feed
          </span>
        </div>
      </div>

      <!-- Sidebar (desktop only) -->
      <aside class="hidden lg:block lg:col-span-1 space-y-4">
        <WeeklyDigestCard :digest="insights.latestDigest" :loading="insights.loadingDigest" />

        <!-- Top topics chip cloud -->
        <section
          v-if="insights.trends && insights.trends.top_topics.length > 0"
          class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5"
        >
          <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">Top topics</p>
          <div class="flex flex-wrap gap-1.5">
            <span
              v-for="t in insights.trends.top_topics.slice(0, 12)"
              :key="t.topic"
              class="inline-flex items-center px-2 py-0.5 rounded-md text-xs bg-zinc-50 text-zinc-700 ring-1 ring-inset ring-zinc-500/10"
            >
              {{ t.topic }}
              <span class="ml-1 text-zinc-400 tabular-nums">{{ t.count }}</span>
            </span>
          </div>
        </section>

        <!-- Sentiment sparkline -->
        <section
          v-if="insights.sentiment && (insights.sentiment.series?.length ?? 0) > 0"
          class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5"
        >
          <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">Sentiment trend</p>
          <SentimentSparkline :series="insights.sentiment.series" />
        </section>
      </aside>
    </div>

    <!-- Export modal -->
    <Teleport to="body">
      <div v-if="exportOpen" class="fixed z-50 inset-0 overflow-y-auto" role="dialog" aria-modal="true">
        <div class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
          <div class="fixed inset-0 bg-zinc-900/60 backdrop-blur-sm" aria-hidden="true" @click="exportOpen = false"></div>
          <span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>
          <div class="inline-block align-bottom bg-white rounded-2xl px-4 pt-5 pb-6 text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-md sm:w-full sm:p-8">
            <div class="flex items-start justify-between">
              <div>
                <h3 class="text-xl font-semibold text-zinc-900">Export comments</h3>
                <p class="mt-1 text-sm text-zinc-500">Exports your current filtered view.</p>
              </div>
              <button
                type="button"
                aria-label="Close"
                @click="exportOpen = false"
                class="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-700 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-zinc-900"
              >
                <X class="w-4 h-4" />
              </button>
            </div>
            <div class="mt-5">
              <label for="export-format" class="block text-sm font-medium text-zinc-700 mb-1.5">Format</label>
              <select
                id="export-format"
                v-model="exportFormat"
                class="block w-full px-3 py-2.5 border border-zinc-200 rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 bg-zinc-50/50"
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
                class="inline-flex justify-center items-center gap-1.5 py-2.5 px-5 rounded-xl text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-800 disabled:opacity-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900"
              >
                <Download class="w-4 h-4" />
                {{ exportSubmitting ? 'Working…' : 'Start export' }}
              </button>
              <button
                type="button"
                @click="exportOpen = false"
                class="inline-flex justify-center items-center py-2.5 px-5 rounded-xl text-sm font-medium text-zinc-700 bg-white border border-zinc-200 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
