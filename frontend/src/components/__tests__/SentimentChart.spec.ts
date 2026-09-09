// SentimentChart.spec.ts — prop/data mapping (implement-missing-product-
// features §1.6): empty series renders the empty state, populated series
// maps per-day negative/neutral/positive counts + net score into the
// rendered output. The chart is display-only; the range selector lives
// in InsightsFeed.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SentimentChart from '../SentimentChart.vue'
import type { SentimentData } from '../../services/insights'

function makeData(overrides: Partial<SentimentData> = {}): SentimentData {
  return {
    days: 7,
    series: [
      { date: '2026-09-01', negative: 2, neutral: 10, positive: 5, score: -2 },
      { date: '2026-09-02', negative: 1, neutral: 8, positive: 6, score: 1 },
    ],
    ...overrides,
  }
}

describe('SentimentChart', () => {
  it('renders the empty state when data is null', () => {
    const wrapper = mount(SentimentChart, { props: { data: null } })
    expect(wrapper.text()).toContain('No sentiment data yet')
    expect(wrapper.find('polyline').exists()).toBe(false)
  })

  it('renders the empty state when the series is empty (backend: series never null, may be [])', () => {
    const wrapper = mount(SentimentChart, { props: { data: makeData({ series: [] }) } })
    expect(wrapper.text()).toContain('No sentiment data yet')
  })

  it('maps per-day counts and net score into the rendered output', () => {
    const wrapper = mount(SentimentChart, { props: { data: makeData() } })
    const text = wrapper.text()

    // Per-day rows: date + negative/neutral/positive counts + signed score.
    expect(text).toContain('2026-09-01')
    expect(text).toContain('2026-09-02')
    expect(text).toContain('-2')
    expect(text).toContain('+1')

    // Totals footer: 3 negative / 18 neutral / 11 positive, net -1.
    expect(text).toContain('3 negative')
    expect(text).toContain('18 neutral')
    expect(text).toContain('11 positive')
    expect(text).toContain('Net score')
    expect(text).toContain('-1')
  })

  it('draws a score line (polyline) when the series has data', () => {
    const wrapper = mount(SentimentChart, { props: { data: makeData() } })
    const polyline = wrapper.find('svg polyline')
    expect(polyline.exists()).toBe(true)
    expect(polyline.attributes('points')).toContain(',')
  })

  it('handles a single-day series without producing NaN geometry', () => {
    const wrapper = mount(SentimentChart, {
      props: { data: makeData({ series: [{ date: '2026-09-01', negative: 0, neutral: 1, positive: 0, score: 0 }] }) },
    })
    expect(wrapper.find('svg polyline').attributes('points')).not.toContain('NaN')
    expect(wrapper.text()).toContain('0 negative')
    expect(wrapper.text()).toContain('1 neutral')
  })

  it('caps the per-day table at the last 14 rows with a note for long windows', () => {
    const long: SentimentDay[] = []
    for (let i = 0; i < 20; i++) {
      long.push({ date: `2026-08-${String(i + 1).padStart(2, '0')}`, negative: 0, neutral: 1, positive: 1, score: 1 })
    }
    const wrapper = mount(SentimentChart, { props: { data: makeData({ days: 90, series: long }) } })
    expect(wrapper.text()).toContain('Showing last 14 of 20 days')
  })
})