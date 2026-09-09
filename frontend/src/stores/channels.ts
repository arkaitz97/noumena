// useChannelsStore — owns the tracked-channel list and per-channel
// status. Mirrors mutations into `useAuthStore.refreshChannelsCount` so
// the router's `requiresOnboarded` gate and the billing limit-check stay
// honest.

import { ref } from 'vue'
import { defineStore } from 'pinia'
import { AxiosError } from 'axios'
import * as channelsApi from '../services/channels'
import { openOAuthPopup, generateState } from '../lib/oauthPopup'
import { toast } from '../lib/toast'
import { useAuthStore } from './auth'
import type {
  Channel,
  ChannelStatus,
  ExchangeChannelResponse,
} from '../services/types'

export const useChannelsStore = defineStore('channels', () => {
  const channels = ref<Channel[]>([])
  const statusById = ref<Record<string, ChannelStatus>>({})
  const loading = ref(false)
  const fetchingStatusFor = ref<Record<string, boolean>>({})

  async function fetchAll(): Promise<void> {
    loading.value = true
    try {
      channels.value = await channelsApi.list()
    } catch {
      toast.error('Could not load your channels.')
    } finally {
      loading.value = false
    }
  }

  async function fetchStatus(id: string): Promise<void> {
    fetchingStatusFor.value[id] = true
    try {
      statusById.value[id] = await channelsApi.status(id)
    } catch {
      // Non-fatal — the card just renders without the status detail.
    } finally {
      fetchingStatusFor.value[id] = false
    }
  }

  async function pause(id: string): Promise<void> {
    try {
      await channelsApi.pause(id)
      const idx = channels.value.findIndex((c) => c.id === id)
      if (idx >= 0) channels.value[idx] = { ...channels.value[idx], tracking_status: 'paused' }
      const s = statusById.value[id]
      if (s) statusById.value[id] = { ...s, tracking_status: 'paused' }
      toast.success('Channel paused.')
    } catch {
      toast.error('Could not pause that channel.')
    }
  }

  async function resume(id: string): Promise<void> {
    try {
      await channelsApi.resume(id)
      const idx = channels.value.findIndex((c) => c.id === id)
      if (idx >= 0) channels.value[idx] = { ...channels.value[idx], tracking_status: 'active' }
      const s = statusById.value[id]
      if (s) statusById.value[id] = { ...s, tracking_status: 'active' }
      toast.success('Channel resumed.')
    } catch {
      toast.error('Could not resume that channel.')
    }
  }

  async function remove(id: string): Promise<void> {
    try {
      await channelsApi.remove(id)
      channels.value = channels.value.filter((c) => c.id !== id)
      delete statusById.value[id]
      const auth = useAuthStore()
      await auth.refreshChannelsCount()
      toast.success('Channel removed.')
    } catch {
      toast.error('Could not remove that channel.')
    }
  }

  async function updateBackfill(id: string, days: number): Promise<void> {
    try {
      await channelsApi.updateBackfill(id, days)
      const idx = channels.value.findIndex((c) => c.id === id)
      if (idx >= 0) channels.value[idx] = { ...channels.value[idx], backfill_days: days }
      toast.success('Backfill window updated.')
    } catch {
      toast.error('Could not update the backfill window.')
    }
  }

  /**
   * Connect-a-new-YouTube-channel flow. The opener element MUST call this
   * synchronously inside its click handler so `window.open` is not blocked:
   * the popup is pre-opened about:blank BEFORE the auth-URL fetch, then
   * navigated (PlanComparison pattern).
   */
  async function connectYouTube(backfillDays = 90): Promise<ExchangeChannelResponse | null> {
    // Pre-open synchronously — the URL fetch below must not gate window.open.
    const popup = window.open('', 'oauth_youtube', 'width=500,height=600,menubar=no,toolbar=no,location=yes,status=no,resizable=yes,scrollbars=yes')

    let connect: { auth_url: string; state: string }
    try {
      connect = await channelsApi.connectYouTubeUrl()
    } catch {
      popup?.close()
      toast.error('Could not start the YouTube connection.')
      return null
    }
    const state = connect.state || generateState('youtube')
    let popupResult
    try {
      popupResult = await openOAuthPopup('youtube', connect.auth_url, { state, popup })
    } catch (err) {
      const reason = err instanceof Error ? err.message : 'unknown'
      if (reason === 'popup_blocked') {
        toast.error('Pop-up blocked. Please allow pop-ups for this site, then try again.')
      } else if (reason === 'popup_closed') {
        toast.info('Connection cancelled.')
      } else {
        toast.error('YouTube connection failed.')
      }
      return null
    }
    try {
      const exchanged = await channelsApi.exchangeYouTube(
        popupResult.code,
        popupResult.state,
        backfillDays,
      )
      await fetchAll()
      const auth = useAuthStore()
      await auth.refreshChannelsCount()
      toast.success(`Connected ${exchanged.display_name}.`)
      return exchanged
    } catch {
      toast.error('Could not finalise the YouTube connection.')
      return null
    }
  }

  /**
   * Connect a public channel by @handle or URL (competitor tracking). No
   * OAuth popup: the server resolves the channel via its API key.
   */
  async function connectYouTubeByName(ref: string, backfillDays = 90): Promise<ExchangeChannelResponse | null> {
    try {
      const connected = await channelsApi.connectYouTubeByName(ref, backfillDays)
      await fetchAll()
      const auth = useAuthStore()
      await auth.refreshChannelsCount()
      toast.success(`Connected ${connected.display_name}.`)
      return connected
    } catch (err) {
      if (err instanceof AxiosError && err.response?.status === 402) {
        toast.error('Channel limit reached for your plan. Upgrade to add more channels.')
      } else if (err instanceof AxiosError && err.response?.status === 409) {
        toast.error('Channel already tracked.')
      } else if (err instanceof AxiosError && err.response?.status === 503) {
        toast.error('Public channel tracking is not configured on this server.')
      } else {
        toast.error('Could not connect that channel.')
      }
      return null
    }
  }

  /**
   * Reauthorize an existing channel whose token expired/was revoked. The
   * OAuth popup is pre-opened synchronously by the caller (ChannelCard
   * click handler); the exchange links fresh tokens to the SAME tracking
   * row — never a duplicate channel.
   */
  async function reauthorize(id: string, popup: Window | null): Promise<boolean> {
    let connect: { auth_url: string; state: string }
    try {
      connect = await channelsApi.reauthorizeUrl(id)
    } catch {
      popup?.close()
      toast.error('Could not start the reconnection.')
      return false
    }
    const state = connect.state || generateState('youtube')
    let popupResult
    try {
      popupResult = await openOAuthPopup('youtube', connect.auth_url, { state, popup })
    } catch (err) {
      const reason = err instanceof Error ? err.message : 'unknown'
      if (reason === 'popup_blocked') {
        toast.error('Pop-up blocked. Please allow pop-ups for this site, then try again.')
      } else if (reason === 'popup_closed') {
        toast.info('Reconnection cancelled.')
      } else {
        toast.error('Reconnection failed.')
      }
      return false
    }
    try {
      await channelsApi.reauthorizeExchange(id, popupResult.code, popupResult.state)
      // Refresh list + status so the expiry banner clears.
      await fetchAll()
      await fetchStatus(id)
      toast.success('Channel reconnected. Tracking resumes.')
      return true
    } catch (err) {
      if (err instanceof AxiosError && err.response?.status === 409) {
        toast.error('That YouTube account does not match this channel.')
      } else {
        toast.error('Could not reconnect the channel.')
      }
      return false
    }
  }

  return {
    channels,
    statusById,
    loading,
    fetchingStatusFor,
    fetchAll,
    fetchStatus,
    pause,
    resume,
    remove,
    updateBackfill,
    connectYouTube,
    connectYouTubeByName,
    reauthorize,
  }
})
