// SentimentChart.spec.ts — window-toggle emit + empty-series safety.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SentimentChart from '../SentimentChart.vue'

describe('SentimentChart', () => {
  it('renders without throwing on empty series', () => {
    const wrapper = mount(SentimentChart, {
      props: { series: [], windowKey: '7d' },
    })
    expect(wrapper.html()).toContain('No data yet')
  })

  it('emits update:windowKey when a window toggle is clicked', async () => {
    const wrapper = mount(SentimentChart, {
      props: {
        series: [
          { bucket: '2026-05-01', avg: 0.1, count: 10 },
          { bucket: '2026-05-02', avg: 0.2, count: 12 },
        ],
        windowKey: '7d',
      },
    })
    const buttons = wrapper.findAll('button[role="tab"]')
    const thirty = buttons.find((b) => b.text() === '30d')
    expect(thirty).toBeTruthy()
    await thirty!.trigger('click')
    const events = wrapper.emitted('update:windowKey')
    expect(events).toBeTruthy()
    expect(events![0][0]).toBe('30d')
  })
})
