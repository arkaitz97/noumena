// Pagination regression (fix-frontend-ux-a11y §3.1): the feed/saved offset
// must be tracked independently of list length so optimistic unstar splices
// never re-request already-seen rows (duplicate) or skip rows (gap).

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as commentsApi from '../../services/comments'
import { useFeedStore } from '../feed'
import type { Comment, FeedFilters } from '../../services/types'

vi.mock('../../services/comments')
vi.mock('../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))

const PAGE = 20

function makeComment(id: number): Comment {
  return {
    id: `c${id}`,
    channel_id: 'ch1',
    body: `comment ${id}`,
    processed_at: '2026-09-01T00:00:00Z',
    category: 'question',
    actionable_insights: [],
    is_starred: false,
  }
}

function page(offset: number, total = 200): { comments: Comment[]; total: number; has_more: boolean } {
  const items: Comment[] = []
  for (let i = offset; i < Math.min(offset + PAGE, total); i++) items.push(makeComment(i))
  return { comments: items, total, has_more: offset + PAGE < total }
}

const filters: FeedFilters = {
  channels: [],
  categories: ['question'],
  insight_type: null,
  date_from: null,
  date_to: null,
  search: '',
  include_toxic: false,
  sort: 'newest',
}

describe('feed store pagination offsets', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.mocked(commentsApi.feed).mockImplementation(async (_f, opts) => page(opts?.offset ?? 0))
    vi.mocked(commentsApi.saved).mockImplementation(async (_f, opts) => page(opts?.offset ?? 0))
    vi.mocked(commentsApi.star).mockResolvedValue({} as never)
    vi.mocked(commentsApi.unstar).mockResolvedValue({} as never)
  })

  it('loadMore advances offset by fetched page size, not list length', async () => {
    const feed = useFeedStore()
    feed.applyFilters(filters)
    await feed.fetchFeed({ reset: true })
    expect(commentsApi.feed).toHaveBeenLastCalledWith(filters, { offset: 0, limit: 20 })

    await feed.loadMore()
    expect(commentsApi.feed).toHaveBeenLastCalledWith(filters, { offset: 20, limit: 20 })

    await feed.loadMore()
    expect(commentsApi.feed).toHaveBeenLastCalledWith(filters, { offset: 40, limit: 20 })
    expect(feed.comments).toHaveLength(60)
  })

  it('unstar in saved view does not shift the next-page offset', async () => {
    const feed = useFeedStore()
    feed.applyFilters(filters)
    await feed.fetchSaved({ reset: true })
    expect(feed.savedComments).toHaveLength(20)

    // Optimistic remove of one row from the first page.
    await feed.unstarComment('c0')
    expect(feed.savedComments).toHaveLength(19)

    // Next page must start at server offset 20 (row c20), not 19 (row c19
    // — already rendered) and not 18.
    await feed.loadMoreSaved()
    expect(commentsApi.saved).toHaveBeenLastCalledWith(feed.savedFilters, { offset: 20, limit: 20 })
    const ids = feed.savedComments.map((c) => c.id)
    expect(new Set(ids).size).toBe(ids.length)
    expect(ids).toContain('c20')
    // c19 was part of page 1 — it must appear exactly once (no re-fetch dup).
    expect(ids.filter((id) => id === 'c19')).toHaveLength(1)
  })

  it('failed page does not advance the offset', async () => {
    const feed = useFeedStore()
    feed.applyFilters(filters)
    await feed.fetchFeed({ reset: true })

    vi.mocked(commentsApi.feed).mockRejectedValueOnce(new Error('boom'))
    await feed.loadMore()
    expect(feed.comments).toHaveLength(20)

    await feed.loadMore()
    expect(commentsApi.feed).toHaveBeenLastCalledWith(filters, { offset: 20, limit: 20 })
  })

  it('star/unstar rollback keeps the offset stable', async () => {
    const feed = useFeedStore()
    feed.applyFilters(filters)
    await feed.fetchFeed({ reset: true })

    vi.mocked(commentsApi.unstar).mockRejectedValueOnce(new Error('nope'))
    await feed.unstarComment('c5')
    // Rollback restores the row; the offset must not have moved.
    await feed.loadMore()
    expect(commentsApi.feed).toHaveBeenLastCalledWith(filters, { offset: 20, limit: 20 })
  })
})
