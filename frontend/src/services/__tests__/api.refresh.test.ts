// Refresh-coalescing regression test for `services/api.ts`.
//
// Implements `frontend-auth §5.6`: two concurrent 401s must share a single
// underlying refreshAccessToken() call (single-flight) and both originals
// must be retried with the new bearer.
//
// We exercise the *real* apiClient and response interceptor exported from
// `services/api.ts`; the only injected piece is a stub AuthBridge wired in
// via `registerAuthBridge`. Per-request `config.adapter` is used to fake
// HTTP responses so we never depend on `axios.defaults.adapter` (which
// would leak across tests).

import type { AxiosAdapter, AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import apiClient, { registerAuthBridge, registerRouter } from '../api'

interface StubBridge {
  getAccessToken: ReturnType<typeof vi.fn>
  setAccessToken: ReturnType<typeof vi.fn>
  refreshAccessToken: ReturnType<typeof vi.fn>
  reset: ReturnType<typeof vi.fn>
}

const STALE_TOKEN = 'stale-token'
const FRESH_TOKEN = 'fresh-token'

function makeBridge(overrides?: {
  refresh?: () => Promise<string>
}): StubBridge {
  // Mirror the real auth-store contract: refreshAccessToken updates the
  // token that getAccessToken returns, so the request interceptor's
  // attachAuth() picks up the fresh token on the retry.
  let current = STALE_TOKEN
  const defaultRefresh = () => {
    current = FRESH_TOKEN
    return Promise.resolve(FRESH_TOKEN)
  }
  const refresh = overrides?.refresh ?? defaultRefresh
  return {
    getAccessToken: vi.fn(() => current),
    setAccessToken: vi.fn((t: string | null) => {
      current = t ?? ''
    }),
    refreshAccessToken: vi.fn(refresh),
    reset: vi.fn(() => {
      current = ''
    }),
  }
}

// Build an adapter that, for each URL it sees, returns 401 on the first
// invocation and 200 on the second. The 200 response echoes the
// Authorization header so we can assert the retry carries the new bearer.
function make401ThenAdapter(): { adapter: AxiosAdapter; calls: { url: string; auth: string | undefined }[] } {
  const seen = new Map<string, number>()
  const calls: { url: string; auth: string | undefined }[] = []
  const adapter: AxiosAdapter = (config: InternalAxiosRequestConfig) => {
    const url = config.url ?? ''
    const auth = (config.headers?.Authorization ?? config.headers?.authorization) as string | undefined
    calls.push({ url, auth })
    const n = (seen.get(url) ?? 0) + 1
    seen.set(url, n)
    if (n === 1) {
      const err: any = new Error('Request failed with status code 401')
      err.config = config
      err.response = {
        status: 401,
        statusText: 'Unauthorized',
        headers: {},
        config,
        data: { error: 'unauthorized' },
      } satisfies AxiosResponse
      err.isAxiosError = true
      return Promise.reject(err)
    }
    const ok: AxiosResponse = {
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
      data: { url, auth },
    }
    return Promise.resolve(ok)
  }
  return { adapter, calls }
}

describe('apiClient refresh single-flight', () => {
  beforeEach(() => {
    // Register a no-op router-shaped stub so the 403 email-not-verified
    // branch in the interceptor cannot accidentally fire and the refresh
    // failure branch's `routerRef.push('/login')` is a no-op.
    const stubRouter = {
      currentRoute: { value: { path: '/' } },
      push: vi.fn(() => Promise.resolve()),
    } as any
    registerRouter(stubRouter)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('coalesces concurrent 401s into a single refresh call', async () => {
    const bridge = makeBridge()
    registerAuthBridge(bridge)
    const { adapter, calls } = make401ThenAdapter()

    // Fire 5 concurrent requests against distinct URLs so each gets its
    // own 401-then-200 cycle but all share the single refreshPromise.
    const urls = ['/a', '/b', '/c', '/d', '/e']
    const results = await Promise.all(
      urls.map((u) => apiClient.get(u, { adapter })),
    )

    expect(bridge.refreshAccessToken).toHaveBeenCalledTimes(1)
    // Each URL must have been hit exactly twice: once with 401, once with 200.
    for (const u of urls) {
      const hits = calls.filter((c) => c.url === u)
      expect(hits).toHaveLength(2)
      expect(hits[0].auth).toBe(`Bearer ${STALE_TOKEN}`)
      expect(hits[1].auth).toBe(`Bearer ${FRESH_TOKEN}`)
    }
    // All 5 originals must have resolved with 200.
    for (let i = 0; i < urls.length; i++) {
      expect(results[i].status).toBe(200)
      expect(results[i].data.url).toBe(urls[i])
    }
  })

  it('starts a fresh refresh after the previous one settled (sequential 401s)', async () => {
    const bridge = makeBridge()
    registerAuthBridge(bridge)

    // First window
    const first = make401ThenAdapter()
    const r1 = await apiClient.get('/x', { adapter: first.adapter })
    expect(r1.status).toBe(200)
    expect(bridge.refreshAccessToken).toHaveBeenCalledTimes(1)

    // Allow the .finally() that clears refreshPromise to run.
    await Promise.resolve()
    await Promise.resolve()

    // Second window — same URL, fresh adapter so it 401s again.
    const second = make401ThenAdapter()
    const r2 = await apiClient.get('/x', { adapter: second.adapter })
    expect(r2.status).toBe(200)
    expect(bridge.refreshAccessToken).toHaveBeenCalledTimes(2)
  })

  it('rejects all originals and resets the bridge when refresh fails', async () => {
    const refreshErr = new Error('refresh blew up')
    const bridge = makeBridge({ refresh: () => Promise.reject(refreshErr) })
    registerAuthBridge(bridge)
    const { adapter } = make401ThenAdapter()

    const urls = ['/p', '/q', '/r', '/s', '/t']
    const settled = await Promise.allSettled(
      urls.map((u) => apiClient.get(u, { adapter })),
    )

    expect(bridge.refreshAccessToken).toHaveBeenCalledTimes(1)
    for (const s of settled) {
      expect(s.status).toBe('rejected')
      // Each rejection should carry the refresh error (the interceptor
      // re-throws the refresh failure, not the original 401).
      if (s.status === 'rejected') {
        expect(s.reason).toBe(refreshErr)
      }
    }
    expect(bridge.reset).toHaveBeenCalled()
  })
})
