// insights store — sentiment series (implement-missing-product-features
// §1.5): fetchSentiment caches per channel+days (a missing channel_id maps
// to the global aggregate, cached under its own key), and keeps the previous
// value on error (feature-gated 503 etc.) instead of clearing the chart.

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as insightsApi from '../../services/insights'
import { useInsightsStore } from '../insights'
import type { SentimentData } from '../../services/insights'

vi.mock('../../services/insights')
vi.mock('../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))

function makeData(days = 7): SentimentData {
  return {
    days,
    series: [{ date: '2026-09-01', negative: 2, neutral: 10, positive: 5, score: -2 }],
  }
}

describe('insights store sentiment series', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.mocked(insightsApi.getSentiment).mockResolvedValue(makeData())
  })

  it('fetches with channel_id and days and exposes the series', async () => {
    const store = useInsightsStore()
    const res = await store.fetchSentiment('ch-1', 30)
    expect(insightsApi.getSentiment).toHaveBeenCalledTimes(1)
    expect(insightsApi.getSentiment).toHaveBeenCalledWith('ch-1', 30)
    expect(res?.days).toBe(7)
    expect(store.sentiment?.series).toHaveLength(1)
  })

  it('caches per channel+days; force bypasses the cache', async () => {
    const store = useInsightsStore()
    await store.fetchSentiment('ch-1', 7)
    await store.fetchSentiment('ch-1', 7)
    await store.fetchSentiment('ch-1', 90)
    await store.fetchSentiment('ch-1', 7, { force: true })
    expect(insightsApi.getSentiment).toHaveBeenCalledTimes(3)
    expect(insightsApi.getSentiment).toHaveBeenNthCalledWith(3, 'ch-1', 7)
  })

  it('fetches the global aggregate without a channel_id', async () => {
    const store = useInsightsStore()
    const res = await store.fetchSentiment(undefined, 7)
    expect(insightsApi.getSentiment).toHaveBeenCalledTimes(1)
    expect(insightsApi.getSentiment).toHaveBeenCalledWith(undefined, 7)
    expect(res?.series).toHaveLength(1)
    expect(store.sentiment?.series).toHaveLength(1)
  })

  it('caches global and per-channel series separately', async () => {
    const store = useInsightsStore()
    await store.fetchSentiment(undefined, 7)
    await store.fetchSentiment('ch-1', 7)
    await store.fetchSentiment(undefined, 7)
    expect(insightsApi.getSentiment).toHaveBeenCalledTimes(2)
  })

  it('keeps the previous value when the fetch fails (feature-gated 503)', async () => {
    const store = useInsightsStore()
    await store.fetchSentiment('ch-1', 7)
    expect(store.sentiment?.series).toHaveLength(1)

    vi.mocked(insightsApi.getSentiment).mockRejectedValueOnce(new Error('503'))
    const res = await store.fetchSentiment('ch-1', 7, { force: true })
    expect(res).toEqual(makeData())
    expect(store.sentiment?.series).toHaveLength(1)
  })
})