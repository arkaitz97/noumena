import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DashboardLayout from '../DashboardLayout.vue'
import { useAuthStore } from '../../../stores/auth'
import { useAccountStore } from '../../../stores/account'

vi.mock('../../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
vi.mock('../../../services/api', () => ({
  default: { get: vi.fn().mockResolvedValue({ data: [] }), put: vi.fn().mockResolvedValue({}) },
}))
vi.mock('../../../stores/auth')
vi.mock('../../../stores/account')
vi.mock('../../../stores/rateLimit', () => ({
  useRateLimitStore: () => ({ visible: false, retryAfterSeconds: 0, dismiss: vi.fn() }),
}))
vi.mock('../../../composables/useWorkspaceRole', () => ({
  useWorkspaceRole: () => ({ isAdmin: false }),
}))
vi.mock('../../../composables/useDarkMode', () => ({
  useDarkMode: () => ({ isDark: false, toggleDarkMode: vi.fn() }),
}))
vi.mock('../../../services/tokenExpiry', () => ({
  getTokenExpiry: vi.fn().mockResolvedValue({ channels: [] }),
}))

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', name: 'dashboard', component: { template: '<div />' } },
      { path: '/settings/account', name: 'account', component: { template: '<div />' } },
    ],
  })
}

describe('DashboardLayout profile display in sidebar', () => {
  let wrapper: any

  beforeEach(() => {
    vi.clearAllMocks()
    const pinia = createPinia()
    setActivePinia(pinia)
  })

  it('falls back to email when display_name is not set', async () => {
    vi.mocked(useAuthStore).mockReturnValue({
      user: { id: 'u1', email: 'creator@example.com' },
      logout: vi.fn(),
    } as any)

    vi.mocked(useAccountStore).mockReturnValue({
      creatorProfile: null,
      fetchCreatorProfile: vi.fn().mockResolvedValue(undefined),
    } as any)

    const router = makeRouter()
    router.push('/dashboard')
    await router.isReady()

    wrapper = mount(DashboardLayout, {
      global: { plugins: [router] },
    })
    await flushPromises()

    // Trigger button text contains email
    expect(wrapper.text()).toContain('creator@example.com')
    // Avatar initial is C
    expect(wrapper.text()).toContain('C')
  })

  it('displays display_name and its initial when display_name is set', async () => {
    vi.mocked(useAuthStore).mockReturnValue({
      user: { id: 'u1', email: 'creator@example.com' },
      logout: vi.fn(),
    } as any)

    vi.mocked(useAccountStore).mockReturnValue({
      creatorProfile: { display_name: 'Alex Rivera' },
      fetchCreatorProfile: vi.fn().mockResolvedValue(undefined),
    } as any)

    const router = makeRouter()
    router.push('/dashboard')
    await router.isReady()

    wrapper = mount(DashboardLayout, {
      attachTo: document.body,
      global: { plugins: [router] },
    })
    await flushPromises()

    // Trigger button displays Alex Rivera
    expect(wrapper.text()).toContain('Alex Rivera')
    // Avatar initial is A
    expect(wrapper.text()).toContain('A')

    // Find the user menu trigger button (which contains user's display name)
    const trigger = wrapper.findAll('button').find((b: any) => b.text().includes('Alex Rivera'))
    expect(trigger).toBeDefined()
    await trigger.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Signed in as')
    expect(wrapper.text()).toContain('Alex Rivera')
    expect(wrapper.text()).toContain('creator@example.com')

    wrapper.unmount()
  })
})
