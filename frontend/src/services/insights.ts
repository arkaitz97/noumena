// /api/insights/* surface — toxicity-shield, trends, actionable-insights, latest
// digest. The digest endpoint returns 404 with code `no_digest_yet` for
// users whose first weekly digest has not been generated yet — we
// translate that into `null` so the caller can render an inline empty
// state without inspecting the error.

import { AxiosError } from 'axios'
import apiClient from './api'
import type {
  ApiErrorBody,
  InsightAggregatesResponse,
  ToxicityResponse,
  TrendsResponse,
  WeeklyDigest,
  WindowKey,
} from './types'

interface InsightsParams {
  channel_id?: string
  window?: string
  days?: number
}

function buildParams(channelId?: string, opts: { window?: WindowKey; days?: number } = {}): InsightsParams {
  const out: InsightsParams = {}
  if (channelId) out.channel_id = channelId
  if (opts.window) out.window = opts.window
  if (opts.days) out.days = opts.days
  return out
}

export async function actionableInsights(
  channelId?: string,
  windowKey: WindowKey = '7d',
): Promise<InsightAggregatesResponse> {
  const { data } = await apiClient.get<InsightAggregatesResponse>('/insights/actionable-insights', {
    params: buildParams(channelId, { window: windowKey }),
  })
  return data
}

export async function toxicityShield(
  channelId?: string,
  windowKey: WindowKey = '7d',
): Promise<ToxicityResponse> {
  const { data } = await apiClient.get<ToxicityResponse>('/insights/toxicity-shield', {
    params: buildParams(channelId, { window: windowKey }),
  })
  return data
}

export async function trends(
  channelId?: string,
  days = 7,
): Promise<TrendsResponse> {
  const { data } = await apiClient.get<TrendsResponse>('/insights/trends', {
    params: buildParams(channelId, { days }),
  })
  return data
}

// Sentiment trend — GET /api/insights/sentiment?channel_id=&days=.
// channel_id is optional: when omitted the series aggregates across every
// channel the caller tracks (global trend); when present it must be tracked
// by the caller (403 otherwise). days ∈ {7,30,90} (default 7); series may
// be empty but is never null. The endpoint is feature-gated server-side
// (503) — callers degrade gracefully to an empty chart.

export interface SentimentDay {
  date: string
  negative: number
  neutral: number
  positive: number
  score: number
}

export interface SentimentData {
  days: number
  series: SentimentDay[]
}

export async function getSentiment(channelId?: string, days = 7): Promise<SentimentData> {
  const params: Record<string, string | number> = { days }
  if (channelId) params.channel_id = channelId
  const { data } = await apiClient.get<SentimentData>('/insights/sentiment', { params })
  return data
}

export async function latestDigest(): Promise<WeeklyDigest | null> {
  try {
    const { data } = await apiClient.get<WeeklyDigest>('/insights/digest/latest')
    return data
  } catch (err) {
    if (err instanceof AxiosError && err.response?.status === 404) {
      const body = err.response.data as ApiErrorBody | undefined
      if (!body?.code || body.code === 'no_digest_yet') return null
    }
    throw err
  }
}
