// 429 handling regression test (fix-frontend-plumbing §7): a 429 response
// must surface the rate-limit banner state with Retry-After parsed
// (seconds or HTTP date) and reject the request.

import { createPinia, setActivePinia } from 'pinia'
import type { AxiosAdapter, AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import apiClient, { registerAuthBridge } from '../api'
import { useRateLimitStore } from '../../stores/rateLimit'

function stubBridge(): {
  getAccessToken: ReturnType<typeof vi.fn>
  setAccessToken: ReturnType<typeof vi.fn>
  refreshAccessToken: ReturnType<typeof vi.fn>
  reset: ReturnType<typeof vi.fn>
} {
  return {
    getAccessToken: vi.fn(() => 'tok'),
    setAccessToken: vi.fn(),
    refreshAccessToken: vi.fn(),
    reset: vi.fn(),
  }
}

function make429Adapter(retryAfter: string): AxiosAdapter {
  return async (config: InternalAxiosRequestConfig): Promise<AxiosResponse> => {
    const err: any = new Error('Request failed with status code 429')
    err.config = config
    err.response = {
      status: 429,
      statusText: 'Too Many Requests',
      headers: { 'retry-after': retryAfter },
      config,
      data: { error: 'rate_limited' },
    } satisfies AxiosResponse
    err.isAxiosError = true
    throw err
  }
}

describe('api 429 → rate-limit banner', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    registerAuthBridge(stubBridge())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('parses Retry-After seconds and shows the banner', async () => {
    apiClient.defaults.adapter = make429Adapter('45')
    const store = useRateLimitStore()
    await expect(apiClient.get('/channels')).rejects.toBeTruthy()
    expect(store.visible).toBe(true)
    expect(store.retryAfterSeconds).toBe(45)
  })

  it('parses Retry-After HTTP-date and converts to seconds', async () => {
    const future = new Date(Date.now() + 60_000)
    apiClient.defaults.adapter = make429Adapter(future.toUTCString())
    const store = useRateLimitStore()
    await expect(apiClient.get('/channels')).rejects.toBeTruthy()
    expect(store.visible).toBe(true)
    expect(store.retryAfterSeconds).toBeGreaterThanOrEqual(55)
    expect(store.retryAfterSeconds).toBeLessThanOrEqual(65)
  })

  it('dismiss hides the banner', async () => {
    apiClient.defaults.adapter = make429Adapter('10')
    const store = useRateLimitStore()
    await expect(apiClient.get('/channels')).rejects.toBeTruthy()
    store.dismiss()
    expect(store.visible).toBe(false)
  })
})