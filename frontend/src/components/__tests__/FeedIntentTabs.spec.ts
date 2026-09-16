import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedIntentTabs from '../FeedIntentTabs.vue'

describe('FeedIntentTabs', () => {
  it('emits selected preset and exposes active state', async () => {
    const wrapper = mount(FeedIntentTabs, { props: { active: 'all' } })
    await wrapper.get('button[role="tab"]:nth-child(3)').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual(['questions'])
    expect(wrapper.get('button[aria-selected="true"]').text()).toContain('All')
  })
})
