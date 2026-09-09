// Drill-down (fix-frontend-ux-a11y §4): clicking an actionable insight
// navigates to the feed filtered by that insight type (+ channel when the
// aggregation is channel-scoped).

import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'

import ActionableInsightsPanel from '../ActionableInsightsPanel.vue'
import type { InsightAggregatesResponse, InsightType } from '../../services/types'

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/dashboard', name: 'dashboard', component: { template: '<div />' } }],
  })
}

function data(overrides: Partial<InsightAggregatesResponse> = {}): InsightAggregatesResponse {
  return {
    window: '7d',
    channel_id: null,
    distribution: [],
    top_insights: [
      { type: 'video_idea', description: 'Viewers want more tutorials', count: 12 },
      { type: 'feature_request', description: 'Dark mode for the app', count: 5 },
    ],
    total_insights: 17,
    ...overrides,
  }
}

async function mountPanel(payload: InsightAggregatesResponse, typeFilter?: InsightType) {
  const router = makeRouter()
  await router.push('/dashboard')
  await router.isReady()
  const wrapper = mount(ActionableInsightsPanel, {
    props: { data: payload, typeFilter },
    global: { plugins: [router] },
  })
  return { wrapper, router }
}

describe('ActionableInsightsPanel drill-down', () => {
  it('clicking an insight navigates to the feed filtered by its type', async () => {
    const { wrapper, router } = await mountPanel(data())
    await wrapper.findAll('a')[0].trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('dashboard')
    expect(router.currentRoute.value.query.insight_type).toBe('video_idea')
  })

  it('carries the channel filter through when the aggregation is channel-scoped', async () => {
    const { wrapper, router } = await mountPanel(data({ channel_id: 'ch-42' }))
    await wrapper.findAll('a')[1].trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('dashboard')
    expect(router.currentRoute.value.query.insight_type).toBe('feature_request')
    expect(router.currentRoute.value.query.channels).toBe('ch-42')
  })

  it('narrows the visible list when a type filter is passed', async () => {
    const { wrapper } = await mountPanel(data(), 'feature_request')
    const links = wrapper.findAll('a')
    expect(links).toHaveLength(1)
    expect(links[0].text()).toContain('Dark mode for the app')
  })

  it('shows the filtered empty state when no insight matches the type filter', async () => {
    const { wrapper } = await mountPanel(data(), 'question')
    expect(wrapper.text()).toContain('No insights of this type in range')
  })
})