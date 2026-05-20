// useInsightsStore — sentiment, toxicity, trends, latest digest. Caches
// per-`channelId` (with the special key '__all__' for "no channel filter")
// for 5 minutes so the chart's window toggle and the polling tick don't
// re-fetch unnecessarily.

import { ref } from 'vue'
import { defineStore } from 'pinia'
import * as insightsApi from '../services/insights'
import { toast } from '../lib/toast'
import type {
  SentimentResponse,
  ToxicityResponse,
  TrendsResponse,
  WeeklyDigest,
  WindowKey,
} from '../services/types'

const CACHE_MS = 5 * 60 * 1000

interface CacheEntry<T> {
  fetchedAt: number
  value: T
}

function cacheKey(channelId?: string, suffix?: string): string {
  return `${channelId ?? '__all__'}::${suffix ?? ''}`
}

function fresh<T>(entry: CacheEntry<T> | undefined): T | null {
  if (!entry) return null
  if (Date.now() - entry.fetchedAt > CACHE_MS) return null
  return entry.value
}

export const useInsightsStore = defineStore('insights', () => {
  const sentimentByKey = ref<Record<string, CacheEntry<SentimentResponse>>>({})
  const toxicityByKey = ref<Record<string, CacheEntry<ToxicityResponse>>>({})
  const trendsByKey = ref<Record<string, CacheEntry<TrendsResponse>>>({})
  const latestDigest = ref<WeeklyDigest | null>(null)
  const digestFetchedAt = ref<number>(0)
  const loadingDigest = ref(false)

  // Most-recently-fetched values (convenience getters for views).
  const sentiment = ref<SentimentResponse | null>(null)
  const toxicity = ref<ToxicityResponse | null>(null)
  const trends = ref<TrendsResponse | null>(null)

  async function fetchSentiment(
    channelId?: string,
    windowKey: WindowKey = '7d',
    opts: { force?: boolean } = {},
  ): Promise<SentimentResponse | null> {
    const key = cacheKey(channelId, windowKey)
    if (!opts.force) {
      const hit = fresh(sentimentByKey.value[key])
      if (hit) {
        sentiment.value = hit
        return hit
      }
    }
    try {
      const res = await insightsApi.sentiment(channelId, windowKey)
      sentimentByKey.value[key] = { fetchedAt: Date.now(), value: res }
      sentiment.value = res
      return res
    } catch {
      return sentiment.value
    }
  }

  async function fetchToxicity(
    channelId?: string,
    windowKey: '7d' | '30d' = '7d',
    opts: { force?: boolean } = {},
  ): Promise<ToxicityResponse | null> {
    const key = cacheKey(channelId, windowKey)
    if (!opts.force) {
      const hit = fresh(toxicityByKey.value[key])
      if (hit) {
        toxicity.value = hit
        return hit
      }
    }
    try {
      const res = await insightsApi.toxicityShield(channelId, windowKey)
      toxicityByKey.value[key] = { fetchedAt: Date.now(), value: res }
      toxicity.value = res
      return res
    } catch {
      return toxicity.value
    }
  }

  async function fetchTrends(
    channelId?: string,
    days = 7,
    opts: { force?: boolean } = {},
  ): Promise<TrendsResponse | null> {
    const key = cacheKey(channelId, `d${days}`)
    if (!opts.force) {
      const hit = fresh(trendsByKey.value[key])
      if (hit) {
        trends.value = hit
        return hit
      }
    }
    try {
      const res = await insightsApi.trends(channelId, days)
      trendsByKey.value[key] = { fetchedAt: Date.now(), value: res }
      trends.value = res
      return res
    } catch {
      return trends.value
    }
  }

  async function fetchLatestDigest(opts: { force?: boolean } = {}): Promise<void> {
    if (!opts.force && Date.now() - digestFetchedAt.value < CACHE_MS && latestDigest.value !== null)
      return
    loadingDigest.value = true
    try {
      latestDigest.value = await insightsApi.latestDigest()
      digestFetchedAt.value = Date.now()
    } catch {
      toast.error('Could not load the latest digest.')
    } finally {
      loadingDigest.value = false
    }
  }

  return {
    sentiment,
    toxicity,
    trends,
    latestDigest,
    loadingDigest,
    fetchSentiment,
    fetchToxicity,
    fetchTrends,
    fetchLatestDigest,
  }
})
