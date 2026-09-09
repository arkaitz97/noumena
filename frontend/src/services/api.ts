// Shared Axios client + interceptors.
//
// The interceptors talk to the auth store and the router via late-bound
// setters (registered from main.ts) to avoid circular imports between
// `services/api.ts` ↔ `stores/auth.ts` ↔ `router/index.ts`.
import axios, { AxiosError } from 'axios'
import type { AxiosInstance, AxiosRequestConfig, InternalAxiosRequestConfig } from 'axios'
import type { Router } from 'vue-router'
import { useRateLimitStore } from '../stores/rateLimit'

// `VITE_API_BASE_URL` defaults to `/api`. The /auth surface is computed
// relative to it so deployments where /api lives on a different origin
// can still reach /auth on that same origin (sibling path).
const API_BASE_URL: string = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api'

function deriveAuthBase(apiBase: string): string {
  if (apiBase.startsWith('http://') || apiBase.startsWith('https://')) {
    try {
      const u = new URL(apiBase)
      return `${u.origin}/auth`
    } catch {
      return '/auth'
    }
  }
  return '/auth'
}

export const apiBaseURL = API_BASE_URL
export const authBaseURL = deriveAuthBase(API_BASE_URL)

const apiClient: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  withCredentials: true,
  headers: { 'Content-Type': 'application/json' },
})

// A separate client for /auth/* endpoints. The refresh-token cookie is
// scoped to /auth on the backend, so all auth calls (including the
// silent refresh) need to hit this base.
export const authClient: AxiosInstance = axios.create({
  baseURL: authBaseURL,
  withCredentials: true,
  headers: { 'Content-Type': 'application/json' },
})

// ---------------------------------------------------------------------------
// Late-bound dependencies (registered from main.ts)
// ---------------------------------------------------------------------------

interface AuthBridge {
  getAccessToken: () => string | null
  setAccessToken: (token: string | null) => void
  refreshAccessToken: () => Promise<string>
  reset: () => void
}

let authBridge: AuthBridge | null = null
let routerRef: Router | null = null

export function registerAuthBridge(bridge: AuthBridge): void {
  authBridge = bridge
}

export function registerRouter(r: Router): void {
  routerRef = r
}

// ---------------------------------------------------------------------------
// Request interceptor: attach Authorization header.
// ---------------------------------------------------------------------------

function attachAuth(config: InternalAxiosRequestConfig): InternalAxiosRequestConfig {
  const token = authBridge?.getAccessToken()
  if (token) {
    config.headers = config.headers ?? ({} as any)
    ;(config.headers as any).Authorization = `Bearer ${token}`
  }
  return config
}

apiClient.interceptors.request.use(attachAuth)
authClient.interceptors.request.use(attachAuth)

// ---------------------------------------------------------------------------
// Response interceptor: single-flight refresh on 401, redirect on 403
// email_not_verified, retry once.
// ---------------------------------------------------------------------------

interface RetriableConfig extends AxiosRequestConfig {
  __retried?: boolean
  __isRefreshCall?: boolean
}

let refreshPromise: Promise<string> | null = null

function getRefreshPromise(): Promise<string> {
  if (refreshPromise) return refreshPromise
  if (!authBridge) return Promise.reject(new Error('auth bridge not registered'))
  refreshPromise = authBridge
    .refreshAccessToken()
    .finally(() => {
      // Clear after microtask so concurrent awaiters all attach to the same
      // promise but the next 401 starts a fresh one.
      refreshPromise = null
    })
  return refreshPromise
}

function isEmailNotVerified(err: AxiosError): boolean {
  if (err.response?.status !== 403) return false
  const body = err.response.data as { code?: string } | undefined
  return body?.code === 'email_not_verified'
}

// Retry-After may be seconds ("30") or an HTTP date. Fall back to 30s.
function parseRetryAfter(v: unknown): number {
  const raw = Array.isArray(v) ? v[0] : v
  if (typeof raw !== 'string' || raw === '') return 30
  const secs = Number(raw)
  if (Number.isFinite(secs) && secs > 0) return secs
  const t = Date.parse(raw)
  if (Number.isFinite(t)) return Math.max(1, Math.round((t - Date.now()) / 1000))
  return 30
}

function handleResponseError(err: AxiosError): Promise<never> {
  const config = err.config as RetriableConfig | undefined

  // 429 — surface the rate-limit banner (Retry-After countdown). The
  // request itself still rejects; callers handle it as a plain error.
  if (err.response?.status === 429) {
    useRateLimitStore().show(parseRetryAfter(err.response.headers['retry-after']))
    return Promise.reject(err)
  }

  // 403 email_not_verified → redirect, do not retry.
  if (config && isEmailNotVerified(err)) {
    if (routerRef && routerRef.currentRoute.value.path !== '/verify-email') {
      void routerRef.push('/verify-email')
    }
    return Promise.reject(err)
  }

  // 401 handling — single-flight refresh + retry once.
  if (
    err.response?.status === 401 &&
    config &&
    !config.__retried &&
    !config.__isRefreshCall &&
    authBridge &&
    Boolean((config.headers as Record<string, unknown> | undefined)?.['Authorization'])
  ) {
    return getRefreshPromise()
      .then((newToken) => {
        config.__retried = true
        config.headers = (config.headers ?? {}) as any
        ;(config.headers as any).Authorization = `Bearer ${newToken}`
        // Use the same client this request originated from. Both clients
        // share the same interceptors, so axios.request on either works.
        const client =
          (config.baseURL && config.baseURL.includes('/auth')) ? authClient : apiClient
        return client.request(config) as unknown as Promise<never>
      })
      .catch((refreshErr) => {
        // Refresh failed — clear store and bounce to /login, preserving the
        // destination so the user lands back where they were after signing
        // in again.
        authBridge?.reset()
        if (routerRef && routerRef.currentRoute.value.path !== '/login') {
          void routerRef.push({
            path: '/login',
            query: { next: routerRef.currentRoute.value.fullPath },
          })
        }
        return Promise.reject(refreshErr)
      })
  }

  return Promise.reject(err)
}

apiClient.interceptors.response.use((r) => r, handleResponseError)
authClient.interceptors.response.use((r) => r, handleResponseError)

// ---------------------------------------------------------------------------
// Helper: tag a config as the refresh call so the response interceptor
// won't try to refresh on top of itself.
// ---------------------------------------------------------------------------

export function markRefreshCall(config: AxiosRequestConfig): AxiosRequestConfig {
  ;(config as RetriableConfig).__isRefreshCall = true
  return config
}

export default apiClient
