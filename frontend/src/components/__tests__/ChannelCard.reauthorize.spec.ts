// ChannelCard reconnect regression (fix-frontend-plumbing §4/§5): a token
// that is expired or expiring soon renders a Reconnect button; clicking it
// emits `reauthorize` synchronously (the popup pre-open happens inside the
// click handler, so `window.open` is not blocked).
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelCard from '../ChannelCard.vue'
import type { Channel } from '../../services/types'

function makeChannel(overrides: Partial<Channel> = {}): Channel {
  const past = new Date(Date.now() - 86400000).toISOString()
  return {
    id: 'ch_1',
    platform: 'youtube',
    platform_channel_id: 'UCabc',
    display_name: 'Test channel',
    tracking_status: 'active',
    token_priority: 0,
    backfill_days: 30,
    token_expires_at: past,
    ...overrides,
  } as Channel
}

describe('ChannelCard reauthorize', () => {
  it('shows Reconnect when the token has expired and emits reauthorize with the popup', async () => {
    const wrapper = mount(ChannelCard, {
      props: { channel: makeChannel(), status: null },
    })
    const btn = wrapper.findAll('button').find((b) => b.text().includes('Reconnect'))
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    const emitted = wrapper.emitted('reauthorize')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toBe('ch_1')
    // window.open ran synchronously inside the click handler (popup-blocker
    // contract); happy-dom hands back a Window object that flows through to
    // the store, which navigates it after the auth-URL fetch.
    expect(emitted![0][1]).not.toBeNull()
  })

  it('shows Reconnect when the token expires soon', async () => {
    const soon = new Date(Date.now() + 3 * 86400000).toISOString()
    const wrapper = mount(ChannelCard, {
      props: { channel: makeChannel({ token_expires_at: soon }), status: null },
    })
    const btn = wrapper.findAll('button').find((b) => b.text().includes('Reconnect'))
    expect(btn).toBeTruthy()
  })

  it('does not show Reconnect when the token is healthy', async () => {
    const far = new Date(Date.now() + 90 * 86400000).toISOString()
    const wrapper = mount(ChannelCard, {
      props: { channel: makeChannel({ token_expires_at: far }), status: null },
    })
    expect(wrapper.findAll('button').find((b) => b.text().includes('Reconnect'))).toBeFalsy()
  })
})