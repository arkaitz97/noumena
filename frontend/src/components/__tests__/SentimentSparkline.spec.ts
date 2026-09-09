// SentimentSparkline.spec.ts — compact sidebar variant (spec §4.4):
// renders the 7-point net-score sparkline, always slices to the trailing
// 7 points, and shows an empty state when there is no data.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SentimentSparkline from '../SentimentSparkline.vue'
import type { SentimentData } from '../../services/insights'

function makeData(overrides: Partial<SentimentData> = {}): SentimentData {
  return {
    days: 7,
    series: [
      { date: '2026-09-01', negative: 0, neutral: 5, positive: 5, score: -2 },
      { date: '2026-09-02', negative: 0, neutral: 5, positive: 5, score: 1 },
      { date: '2026-09-03', negative: 0, neutral: 5, positive: 5, score: 3 },
    ],
    ...overrides,
  }
}

describe('SentimentSparkline', () => {
  it('renders the empty state when data is null', () => {
    const wrapper = mount(SentimentSparkline, { props: { data: null } })
    expect(wrapper.text()).toContain('No data yet')
    expect(wrapper.find('svg').exists()).toBe(false)
  })

  it('renders the empty state for an empty series', () => {
    const wrapper = mount(SentimentSparkline, { props: { data: makeData({ series: [] }) } })
    expect(wrapper.text()).toContain('No data yet')
  })

  it('draws a polyline and shows the summed net score', () => {
    const wrapper = mount(SentimentSparkline, { props: { data: makeData() } })
    expect(wrapper.find('svg polyline').exists()).toBe(true)
    expect(wrapper.text()).toContain('Net score')
    // -2 + 1 + 3 = 2
    expect(wrapper.text()).toContain('2')
  })

  it('slices to the trailing 7 points regardless of window', () => {
    const series = []
    for (let i = 0; i < 12; i++) {
      series.push({ date: `2026-08-${String(i + 1).padStart(2, '0')}`, negative: 0, neutral: 1, positive: 1, score: i })
    }
    const wrapper = mount(SentimentSparkline, { props: { data: makeData({ days: 90, series }) } })
    // 12 scores summed = 66; trailing 7 (scores 5..11) sum = 56.
    expect(wrapper.text()).toContain('56')
    const points = wrapper.find('svg polyline').attributes('points')
    expect(points.split(' ')).toHaveLength(7)
  })

  it('shows an up trend badge when the last score exceeds the first', () => {
    const wrapper = mount(SentimentSparkline, { props: { data: makeData() } })
    expect(wrapper.text()).toContain('up')
  })
})