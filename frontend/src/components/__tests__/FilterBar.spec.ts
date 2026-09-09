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
  it('renders the insight type filter in feed variant', () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'feed' },
    })
    expect(wrapper.text()).toContain('Insight type')
  })

  it('hides the insight type filter in saved variant', () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'saved' },
    })
    expect(wrapper.text()).not.toContain('Insight type')
  })

  it('emits reset when the Reset button is clicked', async () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'feed' },
    })
    await wrapper.get('button[aria-label="Reset filters"]').trigger('click')
    expect(wrapper.emitted('reset')).toBeTruthy()
  })

  it('emits update:range when a date preset is clicked (screen-wide insights range)', async () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'feed', range: '7d' as WindowKey },
    })
    const preset = wrapper.findAll('button').find((b) => b.text() === '30D')
    expect(preset).toBeTruthy()
    await preset!.trigger('click')
    expect(wrapper.emitted('update:range')).toEqual([['30d']])
  })

  it('highlights the bound range preset, not the materialised custom dates', () => {
    const wrapper = mount(FilterBar, {
      props: {
        filters: makeFilters({ date_from: '2026-08-10', date_to: '2026-09-09' }),
        channels: [],
        variant: 'feed',
        range: '30d' as WindowKey,
      },
    })
    const presets = wrapper
      .findAll('button')
      .filter((b) => ['7D', '30D', '90D', 'CUSTOM'].includes(b.text()))
    const active = presets.filter((b) => b.attributes('aria-pressed') === 'true')
    expect(active.map((b) => b.text())).toEqual(['30D'])
  })
})
