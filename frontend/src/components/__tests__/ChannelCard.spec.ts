// ChannelCard.spec.ts — pause/resume visibility + emits.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelCard from '../ChannelCard.vue'
import type { Channel } from '../../services/types'

function makeChannel(overrides: Partial<Channel> = {}): Channel {
  return {
    id: 'ch_1',
    user_id: 'u_1',
    platform: 'youtube',
    platform_channel_id: 'UCabc',
    display_name: 'Test channel',
    tracking_status: 'active',
    backfill_days: 30,
    last_synced_at: null,
    next_sync_at: null,
    token_expires_at: null,
    created_at: new Date().toISOString(),
    ...(overrides as Channel),
  } as Channel
}

describe('ChannelCard', () => {
  it('shows Pause when tracking_status is active and emits pause with the channel id', async () => {
    const wrapper = mount(ChannelCard, {
      props: { channel: makeChannel({ tracking_status: 'active' }), status: null },
    })
    const pauseBtn = wrapper.findAll('button').find((b) => b.text().includes('Pause'))
    expect(pauseBtn).toBeTruthy()
    await pauseBtn!.trigger('click')
    expect(wrapper.emitted('pause')).toBeTruthy()
    expect(wrapper.emitted('pause')![0][0]).toBe('ch_1')
  })

  it('shows Resume when tracking_status is paused and emits resume', async () => {
    const wrapper = mount(ChannelCard, {
      props: { channel: makeChannel({ tracking_status: 'paused' }), status: null },
    })
    const resumeBtn = wrapper.findAll('button').find((b) => b.text().includes('Resume'))
    expect(resumeBtn).toBeTruthy()
    expect(wrapper.findAll('button').find((b) => b.text().includes('Pause'))).toBeFalsy()
    await resumeBtn!.trigger('click')
    expect(wrapper.emitted('resume')).toBeTruthy()
    expect(wrapper.emitted('resume')![0][0]).toBe('ch_1')
  })

  it('emits remove with the channel id when the trash button is clicked', async () => {
    const wrapper = mount(ChannelCard, {
      props: { channel: makeChannel(), status: null },
    })
    await wrapper.get('button[aria-label="Remove channel"]').trigger('click')
    expect(wrapper.emitted('remove')).toBeTruthy()
    expect(wrapper.emitted('remove')![0][0]).toBe('ch_1')
  })
})
