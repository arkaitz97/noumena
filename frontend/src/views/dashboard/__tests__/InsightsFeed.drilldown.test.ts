// Feed drill-down (fix-frontend-ux-a11y §4.2/4.3): when the route query
// carries a filter (e.g. from an insight card link), the feed re-parses the
// URL and refetches with that filter applied.

import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as commentsApi from '../../../services/comments'
import InsightsFeed from '../InsightsFeed.vue'

vi.mock('../../../services/comments')
vi.mock('../../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
vi.mock('../../../stores/insights', () => ({
  useInsightsStore: () => ({
    actionableInsights: null,
    toxicity: null,
    trends: null,
    latestDigest: null,
    loadingDigest: false,
    fetchActionableInsights: vi.fn().mockResolvedValue(undefined),
    fetchToxicity: vi.fn().mockResolvedValue(undefined),
    fetchTrends: vi.fn().mockResolvedValue(undefined),
    fetchLatestDigest: vi.fn().mockResolvedValue(undefined),
  }),
}))
vi.mock('../../../stores/channels', () => ({
  useChannelsStore: () => ({ channels: [], fetchAll: vi.fn().mockResolvedValue(undefined) }),
}))
vi.mock('../../../stores/billing', () => ({
  useBillingStore: () => ({ status: null, nearLimit: false, fetchStatus: vi.fn().mockResolvedValue(undefined) }),
}))
vi.mock('../../../composables/useWorkspaceRole', () => ({
  useWorkspaceRole: () => ({ canExport: false, isAdmin: false }),
}))

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', name: 'dashboard', component: InsightsFeed },
      { path: '/settings/billing', name: 'billing', component: { template: '<div />' } },
    ],
  })
}

describe('insights feed drill-down refetch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(commentsApi.feed).mockResolvedValue({ comments: [], total: 0, has_more: false })
  })

  it('re-fetches with the insight_type filter after a query-driven navigation', async () => {
    const router = makeRouter()
    await router.push('/dashboard')
    await router.isReady()

    mount(InsightsFeed, {
      global: {
        plugins: [createPinia(), router],
        stubs: {
          KeyboardHelpModal: { template: '<div />' },
        },
      },
    })
    await flushPromises()

    await router.push({ path: '/dashboard', query: { insight_type: 'video_idea' } })
    await flushPromises()

    expect(commentsApi.feed).toHaveBeenLastCalledWith(
      expect.objectContaining({ insight_type: 'video_idea' }),
      { offset: 0, limit: 20 },
    )
  })

  it('does not double-fetch when the URL already matches store filters', async () => {
    const router = makeRouter()
    await router.push('/dashboard')
    await router.isReady()

    mount(InsightsFeed, {
      global: {
        plugins: [createPinia(), router],
        stubs: { KeyboardHelpModal: { template: '<div />' } },
      },
    })
    await flushPromises()
    const callsAfterMount = vi.mocked(commentsApi.feed).mock.calls.length

    await router.push({ path: '/dashboard', query: {} })
    await flushPromises()

    expect(vi.mocked(commentsApi.feed).mock.calls.length).toBe(callsAfterMount)
  })
})