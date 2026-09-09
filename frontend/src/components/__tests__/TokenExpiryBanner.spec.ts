// TokenExpiryBanner tests (implement-missing-product-features §7.3): an
// expiring channel renders the banner with title + days remaining and a
// reconnect link; no expiring channels → nothing rendered; dismiss emits.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import TokenExpiryBanner from '../TokenExpiryBanner.vue'
import type { TokenExpiryChannel } from '../../services/tokenExpiry'

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', component: { template: '<div />' } },
      { path: '/settings/channels', component: { template: '<div />' } },
    ],
  })
}

function channel(overrides: Partial<TokenExpiryChannel> = {}): TokenExpiryChannel {
  return {
    channel_id: 'ch_1',
    title: 'Tech Talk',
    expires_at: new Date(Date.now() + 5 * 86400000).toISOString(),
    days_remaining: 5,
    ...overrides,
  }
}

async function mountBanner(visible: boolean, channels: TokenExpiryChannel[]) {
  const router = makeRouter()
  await router.push('/dashboard')
  await router.isReady()
  const wrapper = mount(TokenExpiryBanner, {
    props: { visible, channels },
    global: { plugins: [router] },
  })
  return { wrapper, router }
}

describe('TokenExpiryBanner', () => {
  it('renders the title and days remaining when a channel is expiring', async () => {
    const { wrapper } = await mountBanner(true, [channel()])
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Tech Talk')
    expect(wrapper.text()).toContain('token expires in 5 days')
  })

  it('renders every channel when several are expiring', async () => {
    const { wrapper } = await mountBanner(true, [
      channel(),
      channel({ channel_id: 'ch_2', title: 'Cooking Weekly', days_remaining: 2 }),
    ])
    const text = wrapper.text()
    expect(text).toContain('Tech Talk')
    expect(text).toContain('Cooking Weekly')
    expect(text).toContain('2 days')
  })

  it('singularises "day" when exactly one day remains', async () => {
    const { wrapper } = await mountBanner(true, [channel({ days_remaining: 1 })])
    expect(wrapper.text()).toContain('token expires in 1 day.')
  })

  it('offers a reconnect link to the channels view', async () => {
    const { wrapper } = await mountBanner(true, [channel()])
    const link = wrapper.find('a[href="/settings/channels"]')
    expect(link.exists()).toBe(true)
    expect(link.text()).toContain('Reconnect now')
  })

  it('emits dismiss when the close button is clicked', async () => {
    const { wrapper } = await mountBanner(true, [channel()])
    await wrapper.find('button[aria-label="Dismiss"]').trigger('click')
    expect(wrapper.emitted('dismiss')).toBeTruthy()
  })

  it('renders nothing when there are no expiring channels', async () => {
    const { wrapper } = await mountBanner(true, [])
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('*').exists()).toBe(false)
  })

  it('renders nothing when hidden even if channels are expiring', async () => {
    const { wrapper } = await mountBanner(false, [channel()])
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })
})
