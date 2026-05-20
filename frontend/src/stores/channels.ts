// useChannelsStore — owns the tracked-channel list and per-channel
// status. Mirrors mutations into `useAuthStore.refreshChannelsCount` so
// the router's `requiresOnboarded` gate and the billing limit-check stay
// honest.

import { ref } from 'vue'
import { defineStore } from 'pinia'
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
   * synchronously inside its click handler so `window.open` is not blocked.
   */
  async function connectYouTube(backfillDays = 90): Promise<ExchangeChannelResponse | null> {
    let connect: { auth_url: string; state: string }
    try {
      connect = await channelsApi.connectYouTubeUrl()
    } catch {
      toast.error('Could not start the YouTube connection.')
      return null
    }
    const state = connect.state || generateState('youtube')
    let popupResult
    try {
      popupResult = await openOAuthPopup('youtube', connect.auth_url, { state })
    } catch (err) {
      const reason = err instanceof Error ? err.message : 'unknown'
      if (reason === 'popup_blocked') {
        toast.error('Pop-up blocked. Please allow pop-ups for this site.')
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
  }
})
