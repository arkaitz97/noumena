// Typed wrappers for the /api/channels surface used by the auth/onboarding
// flow. The full channel-management surface (pause, resume, delete, …) lives
// elsewhere; only what the auth store + onboarding wizard needs is exposed
// here.
import apiClient from './api'
import type {
  Channel,
  ChannelStatus,
  ConnectChannelResponse,
  ExchangeChannelResponse,
  SyncStatus,
} from './types'

export async function list(): Promise<Channel[]> {
  const { data } = await apiClient.get<Channel[] | null>('/channels')
  return data ?? []
}

export async function connectYouTubeUrl(): Promise<ConnectChannelResponse> {
  const { data } = await apiClient.get<ConnectChannelResponse>('/channels/connect/youtube')
  return data
}

export async function exchangeYouTube(
  code: string,
  state: string,
  backfillDays = 90,
): Promise<ExchangeChannelResponse> {
  const { data } = await apiClient.post<ExchangeChannelResponse>('/channels/exchange/youtube', {
    code,
    state,
    backfill_days: backfillDays,
  })
  return data
}

/** Connect a public channel by @handle or full URL (competitor tracking, no OAuth). */
export async function connectYouTubeByName(
  ref: string,
  backfillDays = 90,
): Promise<ExchangeChannelResponse> {
  const { data } = await apiClient.post<ExchangeChannelResponse>('/channels/connect/youtube/by-name', {
    ref,
    backfill_days: backfillDays,
  })
  return data
}

export async function syncStatus(channelId: string): Promise<SyncStatus> {
  const { data } = await apiClient.get<SyncStatus>(
    `/channels/${encodeURIComponent(channelId)}/sync-status`,
  )
  return data
}

// ---------------------------------------------------------------------------
// Phase-10 management surface — pause / resume / delete / backfill / status.
// ---------------------------------------------------------------------------

export async function pause(id: string): Promise<void> {
  await apiClient.patch(`/channels/${encodeURIComponent(id)}/pause`)
}

export async function resume(id: string): Promise<void> {
  await apiClient.post(`/channels/${encodeURIComponent(id)}/resume`)
}

export async function remove(id: string): Promise<void> {
  await apiClient.delete(`/channels/${encodeURIComponent(id)}`)
}

export async function updateBackfill(id: string, days: number): Promise<void> {
  await apiClient.patch(`/channels/${encodeURIComponent(id)}/backfill-window`, { days })
}

export async function status(id: string): Promise<ChannelStatus> {
  const { data } = await apiClient.get<ChannelStatus>(
    `/channels/${encodeURIComponent(id)}/status`,
  )
  return data
}

// ---------------------------------------------------------------------------
// Reauthorize (fix-frontend-plumbing) — re-link an expired/revoked token to
// an existing tracking row. Same OAuth popup dance as connect; the exchange
// updates the existing row instead of creating a channel.
// ---------------------------------------------------------------------------

export async function reauthorizeUrl(id: string): Promise<ConnectChannelResponse> {
  const { data } = await apiClient.post<ConnectChannelResponse>(
    `/channels/${encodeURIComponent(id)}/reauthorize`,
  )
  return data
}

export async function reauthorizeExchange(
  channelId: string,
  code: string,
  state: string,
): Promise<ExchangeChannelResponse> {
  const { data } = await apiClient.post<ExchangeChannelResponse>('/channels/reauthorize/exchange', {
    code,
    state,
    channel_id: channelId,
  })
  return data
}
