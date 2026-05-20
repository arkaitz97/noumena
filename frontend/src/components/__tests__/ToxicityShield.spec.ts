// ToxicityShield.spec.ts — threshold colour switch + zero-total safety.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ToxicityShield from '../ToxicityShield.vue'
import type { ToxicityResponse } from '../../services/types'

function makeData(overrides: Partial<ToxicityResponse> = {}): ToxicityResponse {
  return {
    window: '7d',
    channel_id: null,
    filtered_pct: 0,
    filtered_comments: 0,
    total_comments: 100,
    ...overrides,
  }
}

describe('ToxicityShield', () => {
  it('renders 0.0% with empty ring when total_comments is 0', () => {
    const wrapper = mount(ToxicityShield, {
      props: {
        data: makeData({ total_comments: 0, filtered_pct: 0 }),
        windowKey: '7d',
      },
    })
    expect(wrapper.text()).toContain('0.0%')
  })

  it('uses the green stroke colour when filtered_pct < 5', () => {
    const wrapper = mount(ToxicityShield, {
      props: { data: makeData({ filtered_pct: 2 }), windowKey: '7d' },
    })
    // green token #10b981 — present on the foreground stroke circle
    expect(wrapper.html()).toContain('#10b981')
  })

  it('uses the red stroke colour when filtered_pct >= 15', () => {
    const wrapper = mount(ToxicityShield, {
      props: { data: makeData({ filtered_pct: 22 }), windowKey: '7d' },
    })
    expect(wrapper.html()).toContain('#f43f5e')
  })
})
