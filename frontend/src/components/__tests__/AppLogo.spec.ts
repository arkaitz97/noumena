import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AppLogo from '../AppLogo.vue'

describe('AppLogo', () => {
  it('renders correctly with default props', () => {
    const wrapper = mount(AppLogo)
    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('Noumena')
  })

  it('renders wordmark when showText is true', () => {
    const wrapper = mount(AppLogo, {
      props: {
        showText: true,
      },
    })
    expect(wrapper.text()).toContain('Noumena')
  })

  it('renders badge when provided', () => {
    const wrapper = mount(AppLogo, {
      props: {
        showText: true,
        badge: 'AI',
      },
    })
    expect(wrapper.text()).toContain('AI')
  })

  it('renders markOnly without container', () => {
    const wrapper = mount(AppLogo, {
      props: {
        markOnly: true,
      },
    })
    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.classes()).toContain('inline-flex')
  })
})
