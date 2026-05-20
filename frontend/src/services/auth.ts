// Typed wrappers for the /auth/* surface plus the verified-gated /api/user/me.
import apiClient, { authClient, markRefreshCall } from './api'
import type {
  LoginResponse,
  RefreshResponse,
  RegisterResponse,
  User,
} from './types'

// ---------------------------------------------------------------------------
// Password
// ---------------------------------------------------------------------------

export async function register(email: string, password: string): Promise<RegisterResponse> {
  const { data } = await authClient.post<RegisterResponse>('/register', { email, password })
  return data
}

export async function login(
  email: string,
  password: string,
  rememberMe = false,
): Promise<LoginResponse> {
  const { data } = await authClient.post<LoginResponse>('/login', {
    email,
    password,
    remember_me: rememberMe,
  })
  return data
}

export async function refresh(): Promise<RefreshResponse> {
  // Mark so the response interceptor doesn't try to refresh on a refresh.
  const cfg = markRefreshCall({})
  const { data } = await authClient.post<RefreshResponse>('/refresh', undefined, cfg)
  return data
}

export async function logout(): Promise<void> {
  await authClient.post('/logout')
}

export async function logoutAll(): Promise<void> {
  await authClient.post('/logout-all')
}

// ---------------------------------------------------------------------------
// Email verification
// ---------------------------------------------------------------------------

export async function verifyEmail(token: string): Promise<{ message: string }> {
  // Backend mounts POST /auth/verify-email with the token in the query string.
  const { data } = await authClient.post<{ message: string }>(
    `/verify-email?token=${encodeURIComponent(token)}`,
  )
  return data
}

export async function resendVerification(): Promise<{ message: string }> {
  const { data } = await authClient.post<{ message: string }>('/resend-verification')
  return data
}

// ---------------------------------------------------------------------------
// Password reset
// ---------------------------------------------------------------------------

export async function forgotPassword(email: string): Promise<{ message: string }> {
  const { data } = await authClient.post<{ message: string }>('/forgot-password', { email })
  return data
}

export async function resetPassword(
  token: string,
  password: string,
): Promise<{ message: string }> {
  const { data } = await authClient.post<{ message: string }>(
    `/reset-password?token=${encodeURIComponent(token)}`,
    { password },
  )
  return data
}

// ---------------------------------------------------------------------------
// Current user
// ---------------------------------------------------------------------------

interface RawProfileResponse {
  id: string
  email: string
  email_verified?: boolean
  email_verified_at?: string | null
  subscription_plan?: string
  subscription_status?: string
  created_at?: string
}

/**
 * Fetch the calling user. The backend exposes this as `GET /api/user/me`,
 * but only when the `ACCOUNT_LIFECYCLE_API_ENABLED` flag is on. When the
 * endpoint is unavailable we fall back to a minimal `User` derived from
 * the JWT subject — callers should treat `email_verified_at` as null in
 * that case and rely on the 403 email_not_verified interceptor instead.
 *
 * BLOCKER: a dedicated `/auth/me` endpoint is requested by the design
 * but not yet implemented in the backend. See tasks.md §12.
 */
export async function me(): Promise<User> {
  const { data } = await apiClient.get<RawProfileResponse>('/user/me')
  const verifiedAt =
    data.email_verified_at ??
    (data.email_verified ? new Date().toISOString() : null)
  return {
    id: data.id,
    email: data.email,
    email_verified_at: verifiedAt,
    subscription_plan: data.subscription_plan,
    subscription_status: data.subscription_status,
    created_at: data.created_at,
  }
}

// ---------------------------------------------------------------------------
// Google SSO (popup flow)
// ---------------------------------------------------------------------------

/**
 * Returns the Google authorization URL the popup should navigate to.
 *
 * BLOCKER: The current backend implements Google SSO as a 302-redirect
 * flow (`GET /auth/google` → 302 to Google → `GET /auth/google/callback`
 * → 302 to /dashboard or /onboarding with the refresh cookie + an
 * `X-Access-Token` response header). It does NOT expose a JSON
 * `auth_url` endpoint, nor a `POST /auth/google/exchange` for the SPA.
 *
 * Until the backend exposes those, the SPA falls back to a top-level
 * navigation: callers can call `loginWithGoogleRedirect()` instead of
 * the popup-based `loginWithGoogle(code, state)`.
 */
export async function googleAuthUrl(): Promise<{ auth_url: string; state: string }> {
  // Best-effort: try the JSON endpoint first; fall back to the static URL.
  try {
    const { data } = await authClient.get<{ auth_url: string; state: string }>('/google?json=1')
    if (data && typeof data.auth_url === 'string') return data
  } catch {
    /* fall through */
  }
  // Fallback: the backend's GET /auth/google itself triggers a 302; the
  // popup will follow it normally.
  return { auth_url: `${authClient.defaults.baseURL || '/auth'}/google`, state: '' }
}

export async function googleExchange(
  code: string,
  state: string,
): Promise<LoginResponse> {
  const { data } = await authClient.post<LoginResponse>('/google/callback', { code, state })
  return data
}

/**
 * Top-level redirect fallback for environments where the backend does not
 * yet support a popup-based code exchange. The browser navigates to
 * `/auth/google` and the backend handles the entire dance.
 */
export function loginWithGoogleRedirect(): void {
  window.location.href = `${authClient.defaults.baseURL || '/auth'}/google`
}
