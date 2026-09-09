<script setup lang="ts">
// Saved comments (/dashboard/saved). Reuses CommentCard + FilterBar
// (variant=saved). No summary cards. Its own URL-state. Unstar removes
// the row optimistically (handled in feedStore.unstarComment).

import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Bookmark } from 'lucide-vue-next'
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
    if (f.categories.length > 0) q.categories = f.categories.join(',')
    if (f.insight_type) q.insight_type = f.insight_type
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
    const insightType = asString(query.insight_type)
    if (insightType) out.insight_type = insightType as FeedFilters['insight_type']
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
    <div class="mb-7">
      <p class="eyebrow rise mb-2">Bookmarks</p>
      <h1 class="rise d2 text-display-sm font-bold text-zinc-900 dark:text-white">
        Saved comments
      </h1>
      <p class="rise d3 mt-2 text-sm text-zinc-500 dark:text-zinc-400">
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

    <div v-if="feed.isInitialLoadSaved" class="space-y-3" role="status" aria-label="Loading saved comments">
      <div
        v-for="i in 3"
        :key="i"
        class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl h-28 animate-pulse"
      ></div>
    </div>

    <div
      v-else-if="feed.savedComments.length === 0"
      class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-12 text-center"
    >
      <div class="mx-auto w-12 h-12 rounded-xl bg-zinc-50 dark:bg-white/5 ring-1 ring-zinc-950/5 dark:ring-white/10 flex items-center justify-center text-zinc-400 dark:text-zinc-500 mb-4">
        <Bookmark class="w-5 h-5" />
      </div>
      <p class="text-sm font-medium text-zinc-900 dark:text-white">No saved comments yet.</p>
      <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1">Star comments from your feed to keep track of them here.</p>
      <RouterLink
        to="/dashboard"
        class="mt-4 inline-flex items-center px-3 py-2 border border-zinc-200 dark:border-white/10 shadow-card text-sm font-medium rounded-xl text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
      >
        Go to feed
      </RouterLink>
    </div>

    <TransitionGroup v-else tag="div" name="saved-list" class="space-y-3">
      <CommentCard
        v-for="c in feed.savedComments"
        :key="c.id"
        :comment="c"
        :is-starred="true"
        @toggle-star="(id, next) => (next ? feed.starComment(id) : feed.unstarComment(id))"
      />
    </TransitionGroup>

    <div ref="sentinel" class="h-10 flex items-center justify-center">
      <span v-if="feed.savedLoadingMore" class="text-xs text-zinc-500 dark:text-zinc-400">Loading more…</span>
      <span v-else-if="!feed.savedHasMore && feed.savedComments.length > 0" class="text-xs text-zinc-500 dark:text-zinc-400">
        End of saved comments
      </span>
    </div>
  </div>
</template>

<style scoped>
.saved-list-enter-active,
.saved-list-leave-active,
.saved-list-move {
  transition:
    opacity 0.3s cubic-bezier(0.16, 1, 0.3, 1),
    transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.saved-list-enter-from,
.saved-list-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
.saved-list-leave-active {
  position: absolute;
}
</style>
