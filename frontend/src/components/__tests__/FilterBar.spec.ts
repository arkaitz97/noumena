import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FilterBar from '../FilterBar.vue'
import type { FeedFilters } from '../../services/types'

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
})
