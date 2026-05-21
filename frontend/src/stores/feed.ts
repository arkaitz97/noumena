// useFeedStore — owns the comment list, filters, and pagination for both
// /dashboard and /dashboard/saved. The two views call different fetch
// helpers (`fetchFeed` vs `fetchSaved`) but share the optimistic
// star/unstar plumbing and filter state.

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { AxiosError } from 'axios'
import * as commentsApi from '../services/comments'
import { HIGH_SIGNAL_CATEGORIES } from '../lib/categories'
import { toast } from '../lib/toast'
import type {
  ApiErrorBody,
  Comment,
  ExportFormat,
  FeedFilters,
} from '../services/types'

const PAGE_SIZE = 20

function defaultFilters(): FeedFilters {
  return {
    channels: [],
    categories: [...HIGH_SIGNAL_CATEGORIES],
    insight_type: null,
    date_from: null,
    date_to: null,
    search: '',
    include_toxic: false,
    sort: 'newest',
  }
}

function feedTimeoutCode(err: unknown): string | null {
  if (!(err instanceof AxiosError)) return null
  if (err.response?.status !== 408) return null
  const body = err.response.data as ApiErrorBody | undefined
  return body?.code ?? body?.error ?? null
}

export const useFeedStore = defineStore('feed', () => {
  // ---- shared filter state ----
  const filters = ref<FeedFilters>(defaultFilters())

  // ---- /dashboard feed ----
  const comments = ref<Comment[]>([])
  const total = ref(0)
  const hasMore = ref(false)
  const loading = ref(false)
  const loadingMore = ref(false)

  // ---- /dashboard/saved ----
  const savedComments = ref<Comment[]>([])
  const savedTotal = ref(0)
  const savedHasMore = ref(false)
  const savedLoading = ref(false)
  const savedLoadingMore = ref(false)

  function applyFilters(partial: Partial<FeedFilters>): void {
    filters.value = { ...filters.value, ...partial }
  }

  function resetFilters(): void {
    filters.value = defaultFilters()
  }

  async function fetchFeed(opts: { reset?: boolean } = {}): Promise<void> {
    const reset = opts.reset ?? true
    if (reset) {
      loading.value = true
    } else {
      loadingMore.value = true
    }
    try {
      const offset = reset ? 0 : comments.value.length
      const res = await commentsApi.feed(filters.value, { offset, limit: PAGE_SIZE })
      if (reset) {
        comments.value = res.comments
      } else {
        comments.value = [...comments.value, ...res.comments]
      }
      total.value = res.total
      hasMore.value = res.has_more
    } catch (err) {
      const code = feedTimeoutCode(err)
      if (code === 'feed_query_timeout') {
        toast.error('That query took too long. Try narrowing your filters.')
        // Per spec: preserve already-rendered rows, don't clear.
      } else {
        toast.error('Failed to load comments. Please try again.')
      }
    } finally {
      loading.value = false
      loadingMore.value = false
    }
  }

  async function loadMore(): Promise<void> {
    if (!hasMore.value || loadingMore.value || loading.value) return
    await fetchFeed({ reset: false })
  }

  async function fetchSaved(opts: { reset?: boolean } = {}): Promise<void> {
    const reset = opts.reset ?? true
    if (reset) {
      savedLoading.value = true
    } else {
      savedLoadingMore.value = true
    }
    try {
      const offset = reset ? 0 : savedComments.value.length
      const res = await commentsApi.saved(filters.value, { offset, limit: PAGE_SIZE })
      if (reset) {
        savedComments.value = res.comments
      } else {
        savedComments.value = [...savedComments.value, ...res.comments]
      }
      savedTotal.value = res.total
      savedHasMore.value = res.has_more
    } catch {
      toast.error('Failed to load saved comments. Please try again.')
    } finally {
      savedLoading.value = false
      savedLoadingMore.value = false
    }
  }

  async function loadMoreSaved(): Promise<void> {
    if (!savedHasMore.value || savedLoadingMore.value || savedLoading.value) return
    await fetchSaved({ reset: false })
  }

  function patchStarLocal(list: Comment[], id: string, value: boolean): Comment | null {
    const idx = list.findIndex((c) => c.id === id)
    if (idx < 0) return null
    const prior = { ...list[idx] }
    list[idx] = { ...list[idx], is_starred: value }
    return prior
  }

  async function starComment(id: string): Promise<void> {
    const priorFeed = patchStarLocal(comments.value, id, true)
    try {
      await commentsApi.star(id)
    } catch {
      if (priorFeed) {
        const idx = comments.value.findIndex((c) => c.id === id)
        if (idx >= 0) comments.value[idx] = priorFeed
      }
      toast.error('Could not save your star. Please try again.')
    }
  }

  async function unstarComment(id: string): Promise<void> {
    // Optimistically update the feed list (flip the flag).
    const priorFeed = patchStarLocal(comments.value, id, false)

    // For the saved view, unstar means "remove from list".
    const savedIdx = savedComments.value.findIndex((c) => c.id === id)
    let priorSaved: { item: Comment; index: number } | null = null
    if (savedIdx >= 0) {
      priorSaved = { item: savedComments.value[savedIdx], index: savedIdx }
      savedComments.value.splice(savedIdx, 1)
      savedTotal.value = Math.max(0, savedTotal.value - 1)
    }

    try {
      await commentsApi.unstar(id)
    } catch {
      if (priorFeed) {
        const idx = comments.value.findIndex((c) => c.id === id)
        if (idx >= 0) comments.value[idx] = priorFeed
      }
      if (priorSaved) {
        savedComments.value.splice(priorSaved.index, 0, priorSaved.item)
        savedTotal.value += 1
      }
      toast.error('Could not remove your star. Please try again.')
    }
  }

  async function requestExport(format: ExportFormat): Promise<void> {
    try {
      const res = await commentsApi.requestExport(filters.value, format)
      if (res.status === 'sync' && res.download_url) {
        window.location.assign(res.download_url)
      } else if (res.status === 'async') {
        toast.success("Export queued — we'll prepare your file.")
      }
    } catch {
      toast.error('Could not start your export. Please try again.')
    }
  }

  const isInitialLoad = computed(() => loading.value && comments.value.length === 0)
  const isInitialLoadSaved = computed(() => savedLoading.value && savedComments.value.length === 0)

  return {
    // filters
    filters,
    applyFilters,
    resetFilters,
    // feed
    comments,
    total,
    hasMore,
    loading,
    loadingMore,
    isInitialLoad,
    fetchFeed,
    loadMore,
    // saved
    savedComments,
    savedTotal,
    savedHasMore,
    savedLoading,
    savedLoadingMore,
    isInitialLoadSaved,
    fetchSaved,
    loadMoreSaved,
    // mutations
    starComment,
    unstarComment,
    requestExport,
  }
})
