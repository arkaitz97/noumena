// Billing usage history + trial countdown (implement-missing-product-features
// §3.3 / §5.2 / §5.5): the usage table renders period/used/quota rows from
// the store, empties gracefully, the trial banner follows the backend
// `trial` block, and the UsageBar stays on the page.

import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as billingApi from '../../../services/billing'
import type { BillingStatusWithTrial, UsagePeriod } from '../../../services/billing'
import { useBillingStore } from '../../../stores/billing'
import BillingView from '../BillingView.vue'
import UsageBar from '../../../components/UsageBar.vue'

vi.mock('../../../services/billing', () => ({
  status: vi.fn(),
  getUsage: vi.fn(),
  checkout: vi.fn(),
  portal: vi.fn(),
  NoStripeCustomerError: class NoStripeCustomerError extends Error {
    name = 'NoStripeCustomerError'
  },
}))
vi.mock('../../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))

function makeStatus(trial?: BillingStatusWithTrial['trial']): BillingStatusWithTrial {
  return {
    plan: 'creator',
    interval: 'monthly',
    channels_used: 2,
    channels_limit: 5,
    comments_used: 12,
    comments_limit: 50,
    period_end: '2026-09-30T23:59:59Z',
    overage_cost_usd: 0,
    trial,
  }
}

function makePeriod(overrides: Partial<UsagePeriod> = {}): UsagePeriod {
  return {
    period_start: '2026-09-01T00:00:00Z',
    period_end: '2026-09-30T23:59:59Z',
    comments_used: 12,
    quota: 50,
    ...overrides,
  }
}

describe('BillingView usage history', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.mocked(billingApi.status).mockResolvedValue(makeStatus())
    vi.mocked(billingApi.getUsage).mockResolvedValue({ periods: [] })
  })

  it('renders one row per period with used/quota and keeps the UsageBar', async () => {
    vi.mocked(billingApi.getUsage).mockResolvedValue({
      periods: [
        makePeriod(),
        makePeriod({
          period_start: '2026-08-01T00:00:00Z',
          period_end: '2026-08-31T23:59:59Z',
          comments_used: 30,
        }),
      ],
    })

    const wrapper = mount(BillingView, { global: { plugins: [createPinia()] } })
    await flushPromises()

    expect(billingApi.getUsage).toHaveBeenCalledWith(6)
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(wrapper.text()).toContain('Usage history')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('30')
    expect(wrapper.text()).toContain('50')

    // The current-period UsageBars must stay on the page untouched.
    expect(wrapper.findComponent(UsageBar).exists()).toBe(true)
    expect(wrapper.text()).toContain('Comments this period')
    expect(wrapper.text()).toContain('Channels')
  })

  it('shows an empty state when the API returns no periods', async () => {
    const wrapper = mount(BillingView, { global: { plugins: [createPinia()] } })
    await flushPromises()

    expect(wrapper.findAll('tbody tr')).toHaveLength(0)
    expect(wrapper.text()).toContain('No usage history yet.')
  })

  it('renders the trial countdown with days remaining when the trial is active', async () => {
    vi.mocked(billingApi.status).mockResolvedValue(
      makeStatus({ active: true, ends_at: '2026-09-15T00:00:00Z', days_remaining: 7 }),
    )

    const wrapper = mount(BillingView, { global: { plugins: [createPinia()] } })
    await flushPromises()

    expect(wrapper.text()).toContain('Free trial ends in 7 days')
    expect(wrapper.text()).toContain('Upgrade')
  })

  it('hides the trial countdown when the trial is inactive', async () => {
    vi.mocked(billingApi.status).mockResolvedValue(
      makeStatus({ active: false, ends_at: '2026-09-15T00:00:00Z', days_remaining: 0 }),
    )

    const wrapper = mount(BillingView, { global: { plugins: [createPinia()] } })
    await flushPromises()

    expect(wrapper.text()).not.toContain('Free trial ends')
  })
})

describe('billing store usage/trial mapping', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('fetchUsage stores periods returned by getUsage (default 6)', async () => {
    const periods = [makePeriod(), makePeriod({ comments_used: 30 })]
    vi.mocked(billingApi.getUsage).mockResolvedValue({ periods })

    const store = useBillingStore()
    await store.fetchUsage()

    expect(billingApi.getUsage).toHaveBeenCalledWith(6)
    expect(store.usagePeriods).toEqual(periods)
    expect(store.usageLoading).toBe(false)
  })

  it('fetchUsage forwards an explicit periods count and clears loading on error', async () => {
    const { toast } = await import('../../../lib/toast')
    vi.mocked(billingApi.getUsage).mockRejectedValue(new Error('nope'))

    const store = useBillingStore()
    await store.fetchUsage(3)

    expect(billingApi.getUsage).toHaveBeenCalledWith(3)
    expect(store.usagePeriods).toEqual([])
    expect(store.usageLoading).toBe(false)
    expect(toast.error).toHaveBeenCalled()
  })

  it('fetchStatus passes the additive trial block through to the store', async () => {
    vi.mocked(billingApi.status).mockResolvedValue(
      makeStatus({ active: true, ends_at: '2026-09-15T00:00:00Z', days_remaining: 7 }),
    )

    const store = useBillingStore()
    await store.fetchStatus()

    expect(store.status?.trial).toEqual({
      active: true,
      ends_at: '2026-09-15T00:00:00Z',
      days_remaining: 7,
    })
  })
})
// llm-cost-analytics 3.2: the cost column + MTD line render when the
// additive API fields are present and disappear without error when an
// older backend omits them.
describe('BillingView LLM cost display', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.mocked(billingApi.getUsage).mockResolvedValue({ periods: [] })
  })

  it('renders the cost column and the MTD line when cost fields are present', async () => {
    vi.mocked(billingApi.status).mockResolvedValue({ ...makeStatus(), llm_cost_mtd_usd: 0.42 })
    vi.mocked(billingApi.getUsage).mockResolvedValue({
      periods: [makePeriod({ llm_cost_usd: 1.23 }), makePeriod({ llm_cost_usd: 0, period_start: '2026-08-01T00:00:00Z' })],
    })

    const wrapper = mount(BillingView, { global: { plugins: [createPinia()] } })
    await flushPromises()

    expect(wrapper.text()).toContain('AI cost')
    const cells = wrapper.findAll('[data-testid="llm-cost-cell"]')
    expect(cells).toHaveLength(2)
    expect(cells[0].text()).toBe('$1.23')
    expect(cells[1].text()).toBe('$0.00')
    const mtd = wrapper.find('[data-testid="llm-cost-mtd"]')
    expect(mtd.exists()).toBe(true)
    expect(mtd.text()).toContain('$0.42')
  })

  it('falls back to the plain table when the backend omits cost fields', async () => {
    vi.mocked(billingApi.status).mockResolvedValue(makeStatus())
    vi.mocked(billingApi.getUsage).mockResolvedValue({ periods: [makePeriod()] })

    const wrapper = mount(BillingView, { global: { plugins: [createPinia()] } })
    await flushPromises()

    expect(wrapper.text()).not.toContain('AI cost')
    expect(wrapper.find('[data-testid="llm-cost-cell"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="llm-cost-mtd"]').exists()).toBe(false)
    // The original columns must still render.
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.text()).toContain('12')
  })
})
