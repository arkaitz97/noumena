import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import WeeklyDigestCard from '../WeeklyDigestCard.vue'
import type { WeeklyDigest } from '../../services/types'
import { trackEvent } from '../../services/events'

vi.mock('../../services/events', () => ({
  trackEvent: vi.fn(),
}))

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

describe('WeeklyDigestCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders empty-state copy and skips the sections when digest is null', () => {
    const wrapper = mount(WeeklyDigestCard, { props: { digest: null } })
    expect(wrapper.text()).toMatch(/Once you've collected a few days of comments/)
    expect(wrapper.text()).not.toMatch(/Top ideas/)
    expect(wrapper.text()).not.toMatch(/Top questions/)
  })

  it('renders the four collapsible sections when digest is provided', () => {
    const wrapper = mount(WeeklyDigestCard, { props: { digest } })
    expect(wrapper.text()).toContain('Top ideas')
    expect(wrapper.text()).toContain('Top questions')
    expect(wrapper.text()).toContain('Top actionable insights')
    expect(wrapper.text()).toContain('Channel health')
  })

  it('tracks a digest_section_toggled event per section toggle', async () => {
    const wrapper = mount(WeeklyDigestCard, { props: { digest } })
    const buttons = wrapper.findAll('button[aria-expanded]')
    expect(buttons).toHaveLength(4)

    for (const [i, section] of (['ideas', 'questions', 'insights', 'health'] as const).entries()) {
      await buttons[i].trigger('click')
      expect(trackEvent).toHaveBeenCalledWith('digest_section_toggled', { section })
    }
    expect(trackEvent).toHaveBeenCalledTimes(4)
  })
})
