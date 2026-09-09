import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FilterBar from '../FilterBar.vue'
import type { FeedFilters, WindowKey } from '../../services/types'

function makeFilters(overrides: Partial<FeedFilters> = {}): FeedFilters {
  return {
    channels: [],
    categories: [],
    insight_type: null,
    date_from: null,
    date_to: null,
    search: '',
    include_toxic: false,
    sort: 'newest',
    ...overrides,
  }
}

describe('FilterBar', () => {
  async function openFiltersPanel(wrapper: ReturnType<typeof mount>) {
    await wrapper.get('button[aria-label="Toggle filters"]').trigger('click')
  }

  async function openDateMenu(wrapper: ReturnType<typeof mount>) {
    await wrapper.get('button[aria-label="Select date range"]').trigger('click')
  }

  it('renders the insight type filter in the feed filters panel', async () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'feed' },
    })
    await openFiltersPanel(wrapper)
    expect(wrapper.text()).toContain('Insight type')
  })

  it('hides the insight type filter in saved variant', async () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'saved' },
    })
    await openFiltersPanel(wrapper)
    expect(wrapper.text()).not.toContain('Insight type')
  })

  it('emits reset when the Reset button is clicked', async () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'feed' },
    })
    await wrapper.get('button[aria-label="Reset filters"]').trigger('click')
    expect(wrapper.emitted('reset')).toBeTruthy()
  })

  it('emits update:range when a date preset is picked in the range popover (screen-wide insights range)', async () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'feed', range: '7d' as WindowKey },
    })
    // Trigger labels the current range (Stripe-style).
    expect(wrapper.get('button[aria-label="Select date range"]').text()).toContain('Last 7 days')
    await openDateMenu(wrapper)
    const preset = wrapper.findAll('button[role="menuitemradio"]').find((b) => b.text() === 'Last 30 days')
    expect(preset).toBeTruthy()
    await preset!.trigger('click')
    expect(wrapper.emitted('update:range')).toEqual([['30d']])
    // Popover closes after picking.
    expect(wrapper.find('[aria-label="Date range options"]').exists()).toBe(false)
  })

  it('marks the bound range preset active in the popover, not the materialised custom dates', async () => {
    const wrapper = mount(FilterBar, {
      props: {
        filters: makeFilters({ date_from: '2026-08-10', date_to: '2026-09-09' }),
        channels: [],
        variant: 'feed',
        range: '30d' as WindowKey,
      },
    })
    await openDateMenu(wrapper)
    const items = wrapper.findAll('button[role="menuitemradio"]')
    const active = items.filter((b) => b.attributes('aria-pressed') === 'true')
    expect(active.map((b) => b.text())).toEqual(['Last 30 days'])
  })
})
