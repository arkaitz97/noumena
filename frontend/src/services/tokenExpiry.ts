// Typed wrapper for GET /user/token-expiry: the current user's channels
// whose OAuth token expires within the server-configured warning window.
// Types live here (not services/types.ts) — this surface is only consumed
// by the TokenExpiryBanner mount logic.
import apiClient from './api'

export interface TokenExpiryChannel {
  channel_id: string
  title: string
  /** RFC3339 — the UTC instant the token expires. */
  expires_at: string
  days_remaining: number
}

export interface TokenExpiryResponse {
  /** Never null; empty when no channel is expiring within the window. */
  channels: TokenExpiryChannel[]
  warning_days: number
}

export async function getTokenExpiry(): Promise<TokenExpiryResponse> {
  const { data } = await apiClient.get<TokenExpiryResponse | null>('/user/token-expiry')
  return data ?? { channels: [], warning_days: 0 }
}
