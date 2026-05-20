// SentimentSparkline.spec.ts — svg-only / no controls.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SentimentSparkline from '../SentimentSparkline.vue'

describe('SentimentSparkline', () => {
  it('renders exactly one <svg> and no interactive controls', () => {
    const wrapper = mount(SentimentSparkline, {
      props: {
        series: [
          { bucket: '2026-05-01', avg: 0.1, count: 1 },
          { bucket: '2026-05-02', avg: 0.4, count: 2 },
        ],
      },
    })
    expect(wrapper.findAll('svg')).toHaveLength(1)
    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.findAll('select')).toHaveLength(0)
  })

  it('renders without throwing on empty series', () => {
    const wrapper = mount(SentimentSparkline, { props: { series: [] } })
    expect(wrapper.find('svg').exists()).toBe(true)
  })
})
