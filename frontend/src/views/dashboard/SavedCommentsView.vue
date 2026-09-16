<script setup lang="ts">
// Saved comments (/dashboard/saved). Reuses CommentCard + FilterBar
// (variant=saved). No summary cards. Its own URL-state. Unstar removes
// the row optimistically (handled in feedStore.unstarComment).

import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Bookmark } from 'lucide-vue-next'
import { useFeedStore } from '../../stores/feed'
import { useChannelsStore } from '../../stores/channels'
import { useFilterUrlSync } from '../../lib/useFilterUrlSync'
import { parseFeedScope, serializeFeedScope } from '../../lib/feedScope'
import type { FeedFilters } from '../../services/types'
import Card from '../../components/Card.vue'
import CommentCard from '../../components/CommentCard.vue'
import FilterBar from '../../components/FilterBar.vue'
import PageHeader from '../../components/PageHeader.vue'

const feed = useFeedStore()
const channels = useChannelsStore()

useFilterUrlSync<FeedFilters>({
  get: () => feed.savedFilters,
  apply: (partial) => feed.applySavedFilters(partial),
  serialise: (f) => {
    return serializeFeedScope({ ...f, range: '7d', view: 'stream' })
  },
  parse: (query) => {
    return parseFeedScope(query, feed.savedFilters)
  },
})

function onUpdateFilters(partial: Partial<FeedFilters>): void {
  feed.applySavedFilters(partial)
  void feed.fetchSaved({ reset: true })
}

function onResetFilters(): void {
  feed.resetSavedFilters()
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
    <PageHeader
      class="mb-7"
      title="Saved comments"
      subtitle="The signals you've explicitly starred for follow-up."
    />

    <FilterBar
      :filters="feed.savedFilters"
      :channels="channels.channels"
      variant="saved"
      class="mb-4"
      @update:filters="onUpdateFilters"
      @reset="onResetFilters"
    />

    <div v-if="feed.isInitialLoadSaved" class="space-y-3" role="status" aria-label="Loading saved comments">
      <Card
        v-for="i in 3"
        :key="i"
        class="h-28 animate-pulse"
      />
    </div>

    <div v-else-if="feed.savedComments.length === 0">
      <Card padding="none" class="p-12 text-center">
        <div class="mx-auto w-12 h-12 rounded-xl bg-zinc-50 dark:bg-white/5 ring-1 ring-zinc-950/5 dark:ring-white/10 flex items-center justify-center text-zinc-400 dark:text-zinc-500 mb-4">
          <Bookmark class="w-5 h-5" />
        </div>
        <p class="text-sm font-semibold text-zinc-900 dark:text-white">No saved comments yet.</p>
        <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-1.5">Star comments from your feed to keep track of them here.</p>
        <RouterLink
          to="/dashboard"
          class="mt-4 inline-flex items-center px-3.5 py-2 text-xs font-semibold rounded-lg text-white bg-zinc-900 dark:bg-white dark:text-zinc-900 hover:bg-zinc-800 dark:hover:bg-zinc-100 transition-all duration-200 active:scale-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/70"
        >
          Go to feed
        </RouterLink>
      </Card>
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
