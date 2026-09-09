// Automated axe scans on key views (fix-frontend-ux-a11y §7.11 / §10.1).
// Mounts the dashboard shell, the insights feed, and the account/settings
// view with stubbed stores and asserts zero WCAG A/AA violations.
//
// `color-contrast` is disabled here: happy-dom does not load Tailwind CSS,
// so computed colors are meaningless — contrast is verified visually (task
// 2.4) and in the browser axe pass (10.2). `target-size` needs layout.

import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import axe from 'axe-core'

import DashboardLayout from '../views/dashboard/DashboardLayout.vue'
import InsightsFeed from '../views/dashboard/InsightsFeed.vue'
import AccountView from '../views/settings/AccountView.vue'

vi.mock('../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
vi.mock('../services/api', () => ({
  default: { get: vi.fn().mockResolvedValue({ data: [] }), put: vi.fn().mockResolvedValue({}) },
}))
vi.mock('../stores/auth', () => ({
  useAuthStore: () => ({
    user: { email: 'user@example.com', id: 'u1' },
    logout: vi.fn().mockResolvedValue(undefined),
    logoutAll: vi.fn().mockResolvedValue(undefined),
  }),
}))
vi.mock('../stores/rateLimit', () => ({
  useRateLimitStore: () => ({ visible: false, retryAfterSeconds: 0, dismiss: vi.fn() }),
}))
vi.mock('../composables/useWorkspaceRole', () => ({
  useWorkspaceRole: () => ({ canExport: false, isAdmin: false }),
}))
vi.mock('../composables/useDarkMode', () => ({
  useDarkMode: () => ({ isDark: false, toggleDarkMode: vi.fn() }),
}))
vi.mock('../stores/insights', () => ({
  useInsightsStore: () => ({
    actionableInsights: null,
    toxicity: null,
    trends: null,
    latestDigest: null,
    loadingDigest: false,
    fetchActionableInsights: vi.fn().mockResolvedValue(undefined),
    fetchToxicity: vi.fn().mockResolvedValue(undefined),
    fetchTrends: vi.fn().mockResolvedValue(undefined),
    fetchLatestDigest: vi.fn().mockResolvedValue(undefined),
  }),
}))
vi.mock('../stores/channels', () => ({
  useChannelsStore: () => ({ channels: [], fetchAll: vi.fn().mockResolvedValue(undefined) }),
}))
vi.mock('../stores/billing', () => ({
  useBillingStore: () => ({ status: null, nearLimit: false, fetchStatus: vi.fn().mockResolvedValue(undefined) }),
}))
vi.mock('../services/comments', () => ({
  recentExports: vi.fn().mockResolvedValue([]),
  feed: vi.fn().mockResolvedValue({ comments: [], total: 0, has_more: false }),
}))
vi.mock('../services/auth', () => ({ forgotPassword: vi.fn().mockResolvedValue({}) }))
vi.mock('../stores/account', () => ({
  useAccountStore: () => ({
    profile: null,
    isVerified: false,
    supportsDigestOptIn: false,
    exportJob: null,
    exportLoading: false,
    updatingEmail: false,
    fetchProfile: vi.fn().mockResolvedValue(undefined),
    requestExport: vi.fn().mockResolvedValue(undefined),
    pollExport: vi.fn().mockResolvedValue(null),
    clearExport: vi.fn(),
    updateEmail: vi.fn().mockResolvedValue(null),
    setDigestOptIn: vi.fn().mockResolvedValue(undefined),
    requestDeletion: vi.fn().mockResolvedValue({ ok: false }),
  }),
}))

const AXE_CONFIG = {
  rules: {
    'color-contrast': { enabled: false }, // needs real CSS (see file header)
    'target-size': { enabled: false }, // needs layout
  },
}

async function scan(element: Element): Promise<axe.Results> {
  return axe.run(element, AXE_CONFIG)
}

function makeRouter(component: unknown) {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', name: 'stub', component: component as never }],
  })
}

function formatViolations(results: axe.Results): string {
  return results.violations
    .map((v) => `${v.id}: ${v.nodes.length} node(s) — ${v.help}`)
    .join('\n')
}

describe('axe scans on key views', () => {
  it('dashboard shell (DashboardLayout) has no WCAG A/AA violations', async () => {
    const router = makeRouter({ template: '<div />' })
    await router.push('/')
    await router.isReady()
    const wrapper = mount(DashboardLayout, { attachTo: document.body, global: { plugins: [router] } })
    await flushPromises()

    const results = await scan(wrapper.element)
    expect(results.violations, formatViolations(results)).toHaveLength(0)
  })

  it('insights feed has no WCAG A/AA violations', async () => {
    const router = makeRouter(InsightsFeed)
    await router.push('/')
    await router.isReady()
    const wrapper = mount(InsightsFeed, {
      attachTo: document.body,
      global: {
        plugins: [createPinia(), router],
        stubs: { KeyboardHelpModal: { template: '<div />' } },
      },
    })
    await flushPromises()

    const results = await scan(wrapper.element)
    expect(results.violations, formatViolations(results)).toHaveLength(0)
  })

  it('account settings view has no WCAG A/AA violations', async () => {
    const router = makeRouter(AccountView)
    await router.push('/')
    await router.isReady()
    const wrapper = mount(AccountView, { attachTo: document.body, global: { plugins: [router] } })
    await flushPromises()

    const results = await scan(wrapper.element)
    expect(results.violations, formatViolations(results)).toHaveLength(0)
  })
})