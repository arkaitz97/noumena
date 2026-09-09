import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as accountApi from '../../../services/account'
import OnboardingView from '../OnboardingView.vue'

vi.mock('../../../services/account', () => ({
  getCreatorProfile: vi.fn().mockResolvedValue({}),
  patchCreatorProfile: vi.fn().mockResolvedValue({}),
}))
vi.mock('../../../services/channels')
vi.mock('../../../lib/oauthPopup', () => ({
  generateState: vi.fn(() => 'st'),
  openOAuthPopup: vi.fn(),
}))
vi.mock('../../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
vi.mock('../../../stores/auth', () => ({
  useAuthStore: () => ({ user: null, refreshChannelsCount: vi.fn().mockResolvedValue(undefined) }),
}))

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/onboarding', name: 'onboarding', component: OnboardingView },
      { path: '/dashboard', name: 'dashboard', component: { template: '<div />' } },
    ],
  })
}

async function mountView() {
  const router = makeRouter()
  router.push('/onboarding')
  await router.isReady()
  const wrapper = mount(OnboardingView, {
    global: {
      plugins: [router],
    },
  })
  await flushPromises()
  return { wrapper, router }
}

describe('OnboardingView creator profile step', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders "About your work" profile step by default (step 1)', async () => {
    const { wrapper } = await mountView()
    expect(wrapper.text()).toContain('Tell us a bit about your work')
    expect(wrapper.find('input#onboarding-name').exists()).toBe(true)
    expect(wrapper.find('select#onboarding-goal').exists()).toBe(true)
    expect(wrapper.find('input#onboarding-topics').exists()).toBe(true)
  })

  it('allows skipping profile step and navigates to step 2', async () => {
    const { wrapper } = await mountView()
    const skipBtn = wrapper.findAll('button').find((b) => b.text().includes('Skip for now'))
    expect(skipBtn).toBeDefined()

    await skipBtn!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Connect your YouTube channel')
    expect(accountApi.patchCreatorProfile).not.toHaveBeenCalled()
  })

  it('submits selected persona, name, and goal when clicking Get started', async () => {
    const { wrapper } = await mountView()

    // Enter name
    const nameInput = wrapper.find('input#onboarding-name')
    await nameInput.setValue('Alex Creator')

    // Select 'creator' radio
    const creatorRadio = wrapper.find('input[type="radio"][value="creator"]')
    await creatorRadio.setValue(true)

    // Select primary goal
    const goalSelect = wrapper.find('select#onboarding-goal')
    await goalSelect.setValue('generate_content_ideas')

    // Enter topics
    const topicsInput = wrapper.find('input#onboarding-topics')
    await topicsInput.setValue('tech, ai, coding')

    const getStartedBtn = wrapper.findAll('button').find((b) => b.text().includes('Get started'))
    expect(getStartedBtn).toBeDefined()

    await getStartedBtn!.trigger('click')
    await flushPromises()

    expect(accountApi.patchCreatorProfile).toHaveBeenCalledWith({
      display_name: 'Alex Creator',
      user_type: 'creator',
      primary_goal: 'generate_content_ideas',
      content_topics: ['tech', 'ai', 'coding'],
    })

    // Progresses to step 2
    expect(wrapper.text()).toContain('Connect your YouTube channel')
  })
})
