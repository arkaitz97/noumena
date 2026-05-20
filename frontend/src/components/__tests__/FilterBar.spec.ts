// FilterBar.spec.ts — variant-based sort selector visibility + update:filters emit.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FilterBar from '../FilterBar.vue'
import type { FeedFilters } from '../../services/types'

function makeFilters(overrides: Partial<FeedFilters> = {}): FeedFilters {
  return {
    channels: [],
    categories: [],
    sentiment_min: null,
    sentiment_max: null,
    date_from: null,
    date_to: null,
    search: '',
    include_toxic: false,
    sort: 'newest',
    ...overrides,
  }
}

describe('FilterBar', () => {
  it('renders the sort selector in feed variant', () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'feed' },
    })
    expect(wrapper.find('#filterbar-sort').exists()).toBe(true)
  })

  it('hides the sort selector in saved variant', () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'saved' },
    })
    expect(wrapper.find('#filterbar-sort').exists()).toBe(false)
  })

  it('emits reset when the Reset button is clicked', async () => {
    const wrapper = mount(FilterBar, {
      props: { filters: makeFilters(), channels: [], variant: 'feed' },
    })
    await wrapper.get('button[aria-label="Reset filters"]').trigger('click')
    expect(wrapper.emitted('reset')).toBeTruthy()
  })
})
