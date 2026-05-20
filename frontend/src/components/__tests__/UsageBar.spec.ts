// UsageBar.spec.ts — threshold transitions + zero-limit safety.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UsageBar from '../UsageBar.vue'

describe('UsageBar', () => {
  it('does not divide by zero when limit is 0 and renders 0% bar', () => {
    const wrapper = mount(UsageBar, {
      props: { used: 0, limit: 0, unit: 'channels', label: 'Channels' },
    })
    // No NaN; width computed from clamped ratio of 0.
    expect(wrapper.html()).toContain('width: 0.0%')
    expect(wrapper.html()).not.toContain('NaN')
  })

  it('uses the green token under 75% utilisation', () => {
    const wrapper = mount(UsageBar, { props: { used: 10, limit: 100 } })
    expect(wrapper.html()).toContain('bg-emerald-500')
  })

  it('switches to amber between 75% and 95%', () => {
    const wrapper = mount(UsageBar, { props: { used: 80, limit: 100 } })
    expect(wrapper.html()).toContain('bg-amber-500')
  })

  it('switches to red at or above 95%', () => {
    const wrapper = mount(UsageBar, { props: { used: 95, limit: 100 } })
    expect(wrapper.html()).toContain('bg-rose-500')
  })
})
