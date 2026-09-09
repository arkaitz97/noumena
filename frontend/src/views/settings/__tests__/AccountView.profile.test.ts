import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as authApi from '../../../services/auth'
import * as commentsApi from '../../../services/comments'
import { useAccountStore } from '../../../stores/account'
import AccountView from '../AccountView.vue'

vi.mock('../../../services/auth')
vi.mock('../../../services/comments')
vi.mock('../../../stores/account')
vi.mock('../../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))

describe('AccountView creator profile editor', () => {
  let mockAccountStore: any

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(authApi.forgotPassword).mockResolvedValue({ message: 'sent' })
    vi.mocked(commentsApi.recentExports).mockResolvedValue([])

    mockAccountStore = {
      profile: null,
      creatorProfile: {
        display_name: 'Jane Doe',
        user_type: 'creator',
        role: 'owner',
        primary_goal: 'understand_audience',
        primary_platform: 'youtube',
        current_workflow: 'native_tools',
        biggest_pain: 'too_many_comments',
        content_topics: ['tech', 'gadgets'],
      },
      creatorProfileLoading: false,
      savingCreatorProfile: false,
      updatingEmail: false,
      exportJob: null,
      exportLoading: false,
      isVerified: false,
      supportsDigestOptIn: false,
      fetchProfile: vi.fn().mockResolvedValue(undefined),
      fetchCreatorProfile: vi.fn().mockResolvedValue(undefined),
      saveCreatorProfile: vi.fn().mockResolvedValue(true),
      updateEmail: vi.fn().mockResolvedValue(null),
      setDigestOptIn: vi.fn().mockResolvedValue(undefined),
      requestExport: vi.fn().mockResolvedValue(undefined),
      pollExport: vi.fn().mockResolvedValue(null),
      clearExport: vi.fn(),
      requestDeletion: vi.fn().mockResolvedValue({ ok: true }),
    }
  })

  function makeRouter() {
    return createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/settings/account', name: 'account', component: AccountView },
        { path: '/login', name: 'login', component: { template: '<div />' } },
      ],
    })
  }

  async function mountView(storeOverrides = {}) {
    const pinia = createPinia()
    setActivePinia(pinia)
    const router = makeRouter()

    // Provide the store
    const store = { ...mockAccountStore, ...storeOverrides }
    vi.mocked(useAccountStore).mockReturnValue(store as any)

    const wrapper = mount(AccountView, {
      global: { plugins: [pinia, router] },
    })
    await flushPromises()
    return { wrapper, router, store }
  }

  it('calls fetchCreatorProfile on mount and renders existing profile data', async () => {
    const { wrapper, store } = await mountView()

    expect(store.fetchCreatorProfile).toHaveBeenCalled()
    expect(wrapper.text()).toContain('Work Profile')

    const nameInput = wrapper.find<HTMLInputElement>('#profile-display-name')
    expect(nameInput.element.value).toBe('Jane Doe')

    const typeSelect = wrapper.find<HTMLSelectElement>('#profile-user-type')
    expect(typeSelect.element.value).toBe('creator')

    const roleSelect = wrapper.find<HTMLSelectElement>('#profile-role')
    expect(roleSelect.element.value).toBe('owner')

    const goalSelect = wrapper.find<HTMLSelectElement>('#profile-primary-goal')
    expect(goalSelect.element.value).toBe('understand_audience')

    const topicsInput = wrapper.find<HTMLInputElement>('#profile-topics')
    expect(topicsInput.element.value).toBe('tech, gadgets')
  })

  it('submits updated profile when form is saved', async () => {
    const { wrapper, store } = await mountView()

    const nameInput = wrapper.find<HTMLInputElement>('#profile-display-name')
    await nameInput.setValue('Jane Manager')

    const typeSelect = wrapper.find<HTMLSelectElement>('#profile-user-type')
    await typeSelect.setValue('agency')

    const roleSelect = wrapper.find<HTMLSelectElement>('#profile-role')
    await roleSelect.setValue('strategist')

    const topicsInput = wrapper.find<HTMLInputElement>('#profile-topics')
    await topicsInput.setValue('marketing, b2b, growth')

    const saveBtn = wrapper.findAll('button').find((b) => b.text().includes('Save work profile'))
    expect(saveBtn).toBeDefined()

    await saveBtn!.trigger('submit')
    await flushPromises()

    expect(store.saveCreatorProfile).toHaveBeenCalledWith(
      expect.objectContaining({
        display_name: 'Jane Manager',
        user_type: 'agency',
        role: 'strategist',
        content_topics: ['marketing', 'b2b', 'growth'],
      }),
    )
  })

  it('validates topic character length (>40 chars) before saving', async () => {
    const { wrapper, store } = await mountView()

    const longTopic = 'a'.repeat(45)
    const topicsInput = wrapper.find<HTMLInputElement>('#profile-topics')
    await topicsInput.setValue(`valid-topic, ${longTopic}`)

    const saveBtn = wrapper.findAll('button').find((b) => b.text().includes('Save work profile'))
    await saveBtn!.trigger('submit')
    await flushPromises()

    expect(store.saveCreatorProfile).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('exceeds 40 characters')
  })

  it('validates display_name character length (>100 chars) before saving', async () => {
    const { wrapper, store } = await mountView()

    const longName = 'a'.repeat(101)
    const nameInput = wrapper.find<HTMLInputElement>('#profile-display-name')
    await nameInput.setValue(longName)

    const saveBtn = wrapper.findAll('button').find((b) => b.text().includes('Save work profile'))
    await saveBtn!.trigger('submit')
    await flushPromises()

    expect(store.saveCreatorProfile).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Display name cannot exceed 100 characters')
  })
})
