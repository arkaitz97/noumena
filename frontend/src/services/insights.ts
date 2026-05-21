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
  windowKey: '7d' | '30d' = '7d',
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
