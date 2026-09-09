// AccountView password change (implement-missing-product-features §2.2):
// inline policy validation before submit (same rule strings as the reset
// form), confirm-match gating, 401 wrong-current error inline, success
// clears the form and swaps the in-memory access token (server reissues
// cookies; other sessions die silently).

import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { AxiosError } from 'axios'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as authApi from '../../../services/auth'
import * as commentsApi from '../../../services/comments'
import { useAuthStore } from '../../../stores/auth'
import AccountView from '../AccountView.vue'

vi.mock('../../../services/auth')
vi.mock('../../../services/comments')
vi.mock('../../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
vi.mock('../../../stores/account', () => ({
  useAccountStore: () => ({
    profile: null,
    loading: false,
    creatorProfile: null,
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
  }),
}))

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/settings/account', name: 'account', component: AccountView },
      { path: '/login', name: 'login', component: { template: '<div />' } },
    ],
  })
}

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = makeRouter()
  const wrapper = mount(AccountView, {
    global: { plugins: [pinia, router] },
  })
  await flushPromises()
  // Forms render in template order: [0] email change, [1] password change.
  const passwordForm = wrapper.findAll('form')[1]
  const inputs = passwordForm.findAll('input')
  return { wrapper, router, pinia, passwordForm, inputs }
}

function authError(status: number, data: Record<string, unknown>): AxiosError {
  return new AxiosError('Request failed', 'ERR_BAD_REQUEST', undefined, undefined, {
    status,
    data,
  } as never)
}

function updateButton(wrapper: VueWrapper) {
  return wrapper.findAll('button').find((b) => b.text() === 'Update password')
}

describe('AccountView password change', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(authApi.forgotPassword).mockResolvedValue({ message: 'sent' })
    vi.mocked(commentsApi.recentExports).mockResolvedValue([])
  })

  it('shows policy rule messages as the new password is typed, before any submit', async () => {
    const { wrapper, inputs } = await mountView()
    await inputs[1].setValue('weak')

    expect(wrapper.text()).toContain('At least 8 characters')
    expect(wrapper.text()).toContain('One uppercase letter')
    expect(wrapper.text()).toContain('One number')

    const update = updateButton(wrapper)
    expect(update!.attributes('disabled')).toBeDefined()
  })

  it('blocks submit on confirm mismatch without calling the service', async () => {
    const { wrapper, inputs, passwordForm } = await mountView()
    await inputs[0].setValue('OldPass1')
    await inputs[1].setValue('NewPass1')
    await inputs[2].setValue('NewPass2')

    expect(wrapper.text()).toContain('Passwords do not match')

    const update = updateButton(wrapper)
    await update!.trigger('click')
    await flushPromises()
    expect(authApi.changePassword).not.toHaveBeenCalled()

    // Defensive guard on the submit handler still reports the mismatch.
    await passwordForm.trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('Passwords do not match.')
    expect(authApi.changePassword).not.toHaveBeenCalled()
  })

  it('submits valid credentials, clears the form, shows success, and swaps the access token', async () => {
    vi.mocked(authApi.changePassword).mockResolvedValue({
      message: 'password updated',
      access_token: 'new-token',
    })
    const { wrapper, inputs, passwordForm, pinia } = await mountView()
    await inputs[0].setValue('OldPass1')
    await inputs[1].setValue('NewPass1')
    await inputs[2].setValue('NewPass1')

    await passwordForm.trigger('submit')
    await flushPromises()

    expect(authApi.changePassword).toHaveBeenCalledWith('OldPass1', 'NewPass1')
    expect(inputs[0].element.value).toBe('')
    expect(inputs[1].element.value).toBe('')
    expect(inputs[2].element.value).toBe('')

    const { toast } = await import('../../../lib/toast')
    expect(toast.success).toHaveBeenCalledWith('Password updated.')
    expect(useAuthStore(pinia).accessToken).toBe('new-token')
  })

  it('shows the wrong-current-password error inline on 401', async () => {
    vi.mocked(authApi.changePassword).mockRejectedValue(
      authError(401, { error: 'current password is incorrect' }),
    )
    const { wrapper, inputs, passwordForm } = await mountView()
    await inputs[0].setValue('WrongPass1')
    await inputs[1].setValue('NewPass1')
    await inputs[2].setValue('NewPass1')

    await passwordForm.trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('Current password is incorrect.')
    expect(inputs[0].element.value).toBe('WrongPass1')
  })

  it('shows the server policy message on 400 and a generic message on network failure', async () => {
    vi.mocked(authApi.changePassword).mockRejectedValue(authError(400, { error: 'password too weak' }))
    const { wrapper, inputs, passwordForm } = await mountView()
    await inputs[0].setValue('OldPass1')
    await inputs[1].setValue('NewPass1')
    await inputs[2].setValue('NewPass1')

    await passwordForm.trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('password too weak')

    vi.mocked(authApi.changePassword).mockRejectedValue(new Error('network down'))
    await passwordForm.trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('Could not update your password. Please try again.')
  })
})