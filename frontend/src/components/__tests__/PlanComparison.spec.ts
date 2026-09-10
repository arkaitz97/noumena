// PlanComparison.spec.ts — disabled current plan + update:interval emit.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PlanComparison from '../PlanComparison.vue'
import type { PlanOption } from '../../lib/plans'

const plans: PlanOption[] = [
  { id: 'free', name: 'Free', monthlyUsd: 0, annualUsd: 0, features: ['Free tier'] },
  { id: 'creator', name: 'Creator', monthlyUsd: 19, annualUsd: 190, features: ['More channels'] },
  { id: 'studio', name: 'Studio', monthlyUsd: 49, annualUsd: 490, features: ['Even more'] },
]

describe('PlanComparison', () => {
  it('disables the CTA on the row matching currentPlan', () => {
    const wrapper = mount(PlanComparison, {
      props: { plans, currentPlan: 'creator', interval: 'monthly' },
    })
    const buttons = wrapper.findAll('button')
    // Find any button labelled "Current plan" — that is the disabled one.
    const current = buttons.find((b) => b.text().includes('Current plan'))
    expect(current).toBeTruthy()
    expect(current!.attributes('disabled')).toBeDefined()
  })

  it('emits update:interval when the annual toggle is clicked while monthly', async () => {
    const wrapper = mount(PlanComparison, {
      props: { plans, currentPlan: 'free', interval: 'monthly' },
    })
    const tabs = wrapper.findAll('button[role="tab"]')
    const annual = tabs.find((t) => t.text() === 'Annual')
    expect(annual).toBeTruthy()
    await annual!.trigger('click')
    const events = wrapper.emitted('update:interval')
    expect(events).toBeTruthy()
    expect(events![0][0]).toBe('annual')
  })
})
