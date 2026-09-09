// Onboarding backfill polling regression (fix-frontend-ux-a11y §6.1): the
// sync must keep polling past 10% until it completes (100%) or fails
// (recent_error) — previously the ≥10% gate froze the bar mid-backfill.

import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as channelsApi from '../../../services/channels'
import * as oauthPopup from '../../../lib/oauthPopup'
import OnboardingView from '../OnboardingView.vue'

vi.mock('../../../services/channels')
vi.mock('../../../lib/oauthPopup', () => ({
  generateState: vi.fn(() => 'st'),
  openOAuthPopup: vi.fn(),
}))
vi.mock('../../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
vi.mock('../../../stores/auth', () => ({
  useAuthStore: () => ({ user: null, refreshChannelsCount: vi.fn().mockResolvedValue(undefined) }),
}))

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', name: 'dashboard', component: { template: '<div />' } },
      { path: '/settings/channels', name: 'channels', component: { template: '<div />' } },
    ],
  })
}

async function mountAndConnect(router: ReturnType<typeof makeRouter>) {
  const wrapper = mount(OnboardingView, {
    global: {
      plugins: [router],
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        PrimaryButton: { template: '<button type="button"><slot /></button>' },
        SecondaryButton: { template: '<button type="button"><slot /></button>' },
      },
    },
  })
  await router.isReady()
  // Step 1 → step 2.
  await wrapper.findAll('button')[0].trigger('click')
  // Step 2 → connect YouTube (opens step 3 and starts polling).
  await wrapper.findAll('button')[0].trigger('click')
  return wrapper
}

describe('onboarding backfill polling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.mocked(channelsApi.connectYouTubeUrl).mockResolvedValue({ auth_url: 'https://example.test/auth', state: 'st' })
    vi.mocked(oauthPopup.openOAuthPopup).mockResolvedValue({ code: 'code', state: 'st' } as never)
    vi.mocked(channelsApi.exchangeYouTube).mockResolvedValue({
      id: 'ch1',
      platform_channel_id: 'yt1',
      display_name: 'My Channel',
      tracking_status: 'active',
      token_priority: 0,
      backfill_days: 90,
    })
  })

  it('polls past 10%, completes at 100%, fires toast, and navigates', async () => {
    vi.mocked(channelsApi.syncStatus)
      .mockResolvedValueOnce({ last_synced_at: null, next_sync_at: null, backfill_progress_percent: 15, recent_error: null })
      .mockResolvedValueOnce({ last_synced_at: null, next_sync_at: null, backfill_progress_percent: 55, recent_error: null })
      .mockResolvedValueOnce({ last_synced_at: null, next_sync_at: null, backfill_progress_percent: 100, recent_error: null })

    const router = makeRouter()
    const pushSpy = vi.spyOn(router, 'push')
    await mountAndConnect(router)

    // Immediate first poll (15%) — old code would have stopped here.
    await vi.advanceTimersByTimeAsync(0)
    expect(channelsApi.syncStatus).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(10_000)
    expect(channelsApi.syncStatus).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(10_000)
    expect(channelsApi.syncStatus).toHaveBeenCalledTimes(3)

    const { toast } = await import('../../../lib/toast')
    expect(toast.success).toHaveBeenCalledWith('Sync complete! Your insights are ready.')

    await vi.advanceTimersByTimeAsync(1_500)
    expect(pushSpy).toHaveBeenCalledWith('/dashboard')
  })

  it('stops polling and shows the escape hatch when the backfill fails', async () => {
    vi.mocked(channelsApi.syncStatus)
      .mockResolvedValueOnce({ last_synced_at: null, next_sync_at: null, backfill_progress_percent: 5, recent_error: null })
      .mockResolvedValueOnce({
        last_synced_at: null,
        next_sync_at: null,
        backfill_progress_percent: 5,
        recent_error: { at: '2026-09-08T00:00:00Z', message: 'sync failed' },
      })

    const router = makeRouter()
    const pushSpy = vi.spyOn(router, 'push')
    const wrapper = await mountAndConnect(router)

    await vi.advanceTimersByTimeAsync(10_000)
    expect(channelsApi.syncStatus).toHaveBeenCalledTimes(2)

    // No further polls; escape hatch visible with a Channels link.
    await vi.advanceTimersByTimeAsync(60_000)
    expect(channelsApi.syncStatus).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('syncing in the background')
    expect(wrapper.text()).toContain('Manage channels')
    expect(pushSpy).not.toHaveBeenCalled()
  })
})