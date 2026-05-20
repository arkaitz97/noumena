// Pinia auth store — single source of truth for the in-memory access token,
// the current user, and the channel count used by the onboarding gate.
//
// IMPORTANT: this file MUST NEVER write the access token to localStorage or
// sessionStorage. Refresh tokens live in an HttpOnly cookie set by the
// backend; the access token is rehydrated on hard reload via a silent call
// to POST /auth/refresh during `bootstrap()`.

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as authApi from '../services/auth'
import * as channelsApi from '../services/channels'
import type { User } from '../services/types'

export const useAuthStore = defineStore('auth', () => {
  // ---- state ----
  const accessToken = ref<string | null>(null)
  const user = ref<User | null>(null)
  const channelsCount = ref<number>(0)
  // bootstrapping flag so the router guard can defensively await it.
  const isBootstrapped = ref<boolean>(false)

  // ---- getters ----
  const isAuthenticated = computed(() => accessToken.value !== null)
  const isEmailVerified = computed(
    () => user.value !== null && user.value.email_verified_at !== null,
  )
  const hasChannels = computed(() => channelsCount.value > 0)

  // ---- internal helpers ----
  function setAccessToken(token: string | null): void {
    accessToken.value = token
  }

  function reset(): void {
    accessToken.value = null
    user.value = null
    channelsCount.value = 0
  }

  async function refreshChannelsCount(): Promise<void> {
    try {
      const list = await channelsApi.list()
      channelsCount.value = list.length
    } catch {
      // Forbidden (verified-email gate) or transient failure — leave count at 0.
      channelsCount.value = 0
    }
  }

  async function fetchUser(): Promise<User | null> {
    try {
      const u = await authApi.me()
      user.value = u
      return u
    } catch {
      // /api/user/me may be unavailable (lifecycle flag off) or return 403
      // when the email isn't verified yet. Leave `user` as null in that
      // case; the verify-email view drives the next step.
      return null
    }
  }

  // ---- actions ----

  async function login(email: string, password: string, rememberMe = false): Promise<void> {
    const res = await authApi.login(email, password, rememberMe)
    accessToken.value = res.access_token
    await fetchUser()
    await refreshChannelsCount()
  }

  async function register(email: string, password: string): Promise<void> {
    const res = await authApi.register(email, password)
    accessToken.value = res.access_token
    // New accounts have no verified email yet — me() will likely 403; that's
    // fine, the verify-email view doesn't depend on it. We still try.
    await fetchUser()
    await refreshChannelsCount()
  }

  async function logout(): Promise<void> {
    try {
      await authApi.logout()
    } finally {
      reset()
    }
  }

  async function logoutAll(): Promise<void> {
    try {
      await authApi.logoutAll()
    } finally {
      reset()
    }
  }

  /**
   * Single-flight refresh entry-point used by the Axios interceptor and by
   * `bootstrap()`. Callers are responsible for sharing one in-flight
   * promise (see `services/api.ts#getRefreshPromise`).
   */
  async function refreshAccessToken(): Promise<string> {
    const res = await authApi.refresh()
    accessToken.value = res.access_token
    return res.access_token
  }

  async function verifyEmail(token: string): Promise<void> {
    await authApi.verifyEmail(token)
    // Re-fetch user so isEmailVerified flips true.
    await fetchUser()
    await refreshChannelsCount()
  }

  async function resendVerification(): Promise<void> {
    await authApi.resendVerification()
  }

  async function forgotPassword(email: string): Promise<void> {
    await authApi.forgotPassword(email)
  }

  async function resetPassword(token: string, password: string): Promise<void> {
    await authApi.resetPassword(token, password)
  }

  async function loginWithGoogle(code: string, state: string): Promise<void> {
    const res = await authApi.googleExchange(code, state)
    accessToken.value = res.access_token
    await fetchUser()
    await refreshChannelsCount()
  }

  /**
   * Called once at app startup. Attempts a silent refresh to recover an
   * existing session, then loads the user and channels count. Never throws
   * — public routes must still render when unauthenticated.
   */
  async function bootstrap(): Promise<void> {
    try {
      await refreshAccessToken()
      await fetchUser()
      await refreshChannelsCount()
    } catch {
      // No valid session — leave the store in its logged-out state.
      reset()
    } finally {
      isBootstrapped.value = true
    }
  }

  return {
    // state
    accessToken,
    user,
    channelsCount,
    isBootstrapped,
    // getters
    isAuthenticated,
    isEmailVerified,
    hasChannels,
    // actions
    setAccessToken,
    reset,
    login,
    register,
    logout,
    logoutAll,
    refreshAccessToken,
    verifyEmail,
    resendVerification,
    forgotPassword,
    resetPassword,
    fetchUser,
    refreshChannelsCount,
    loginWithGoogle,
    bootstrap,
  }
})
