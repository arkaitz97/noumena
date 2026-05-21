import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import WeeklyDigestCard from '../WeeklyDigestCard.vue'
import type { WeeklyDigest } from '../../services/types'

describe('WeeklyDigestCard', () => {
  it('renders empty-state copy and skips the sections when digest is null', () => {
    const wrapper = mount(WeeklyDigestCard, { props: { digest: null } })
    expect(wrapper.text()).toMatch(/Once you've collected a few days of comments/)
    expect(wrapper.text()).not.toMatch(/Top ideas/)
    expect(wrapper.text()).not.toMatch(/Top questions/)
  })

  it('renders the four collapsible sections when digest is provided', () => {
    const digest: WeeklyDigest = {
      week_start: '2026-04-27',
      week_end: '2026-05-03',
      top_ideas: [{ title: 'Idea A', source_count: 3 }],
      top_questions: [{ title: 'Q1', source_count: 2 }],
      top_actionable_insights: [
        { type: 'video_idea', description: 'Make a tutorial', count: 5 },
      ],
      health_score: 88,
    }
    const wrapper = mount(WeeklyDigestCard, { props: { digest } })
    expect(wrapper.text()).toContain('Top ideas')
    expect(wrapper.text()).toContain('Top questions')
    expect(wrapper.text()).toContain('Top actionable insights')
    expect(wrapper.text()).toContain('Channel health')
  })
})
