<script setup lang="ts">
// Saved comments (/dashboard/saved). Reuses CommentCard + FilterBar
// (variant=saved). No summary cards. Its own URL-state. Unstar removes
// the row optimistically (handled in feedStore.unstarComment).

import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useFeedStore } from '../../stores/feed'
import { useChannelsStore } from '../../stores/channels'
import { useFilterUrlSync } from '../../lib/useFilterUrlSync'
import { CATEGORIES, type Category } from '../../lib/categories'
import type { FeedFilters } from '../../services/types'
import CommentCard from '../../components/CommentCard.vue'
import FilterBar from '../../components/FilterBar.vue'

const feed = useFeedStore()
const channels = useChannelsStore()

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
    // The saved view doesn't surface include_toxic / sort, so don't emit them.
    if (f.categories.length > 0) q.categories = f.categories.join(',')
    if (f.sentiment_min !== null && f.sentiment_min > -1) q.sentiment_min = String(f.sentiment_min)
    if (f.sentiment_max !== null && f.sentiment_max < 1) q.sentiment_max = String(f.sentiment_max)
    if (f.date_from) q.date_from = f.date_from
    if (f.date_to) q.date_to = f.date_to
    if (f.search) q.search = f.search
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
    return out
  },
})

function onUpdateFilters(partial: Partial<FeedFilters>): void {
  feed.applyFilters(partial)
  void feed.fetchSaved({ reset: true })
}

function onResetFilters(): void {
  feed.resetFilters()
  void feed.fetchSaved({ reset: true })
}

const sentinel = ref<HTMLElement | null>(null)
let observer: IntersectionObserver | null = null

function setupObserver(): void {
  if (!sentinel.value || typeof IntersectionObserver === 'undefined') return
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting && feed.savedHasMore && !feed.savedLoadingMore && !feed.savedLoading) {
          void feed.loadMoreSaved()
        }
      }
    },
    { rootMargin: '200px' },
  )
  observer.observe(sentinel.value)
}

onMounted(async () => {
  void channels.fetchAll()
  void feed.fetchSaved({ reset: true })
  await Promise.resolve()
  setupObserver()
})

onBeforeUnmount(() => {
  if (observer) {
    observer.disconnect()
    observer = null
  }
})
</script>

<template>
  <div class="max-w-5xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
    <div class="mb-6">
      <h2 class="text-2xl font-semibold leading-7 text-zinc-900 sm:text-3xl sm:tracking-tight">
        Saved comments
      </h2>
      <p class="mt-1 text-sm text-zinc-500">
        The signals you've explicitly starred for follow-up.
      </p>
    </div>

    <FilterBar
      :filters="feed.filters"
      :channels="channels.channels"
      variant="saved"
      class="mb-4"
      @update:filters="onUpdateFilters"
      @reset="onResetFilters"
    />

    <div v-if="feed.isInitialLoadSaved" class="space-y-3" aria-label="Loading saved comments">
      <div
        v-for="i in 3"
        :key="i"
        class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl h-28 animate-pulse"
      ></div>
    </div>

    <div
      v-else-if="feed.savedComments.length === 0"
      class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-10 text-center"
    >
      <p class="text-sm font-medium text-zinc-900">No saved comments yet.</p>
      <p class="text-xs text-zinc-500 mt-1">Star comments from your feed to keep track of them here.</p>
      <RouterLink
        to="/dashboard"
        class="mt-4 inline-flex items-center px-3 py-2 border border-zinc-200 shadow-sm text-sm font-medium rounded-xl text-zinc-700 bg-white hover:bg-zinc-50"
      >
        Go to feed
      </RouterLink>
    </div>

    <div v-else class="space-y-3">
      <CommentCard
        v-for="c in feed.savedComments"
        :key="c.id"
        :comment="c"
        :is-starred="true"
        @toggle-star="(id, next) => (next ? feed.starComment(id) : feed.unstarComment(id))"
      />
    </div>

    <div ref="sentinel" class="h-10 flex items-center justify-center">
      <span v-if="feed.savedLoadingMore" class="text-xs text-zinc-500">Loading more…</span>
      <span v-else-if="!feed.savedHasMore && feed.savedComments.length > 0" class="text-xs text-zinc-400">
        End of saved comments
      </span>
    </div>
  </div>
</template>
