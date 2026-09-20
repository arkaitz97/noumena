// Invite acceptance SPA flow (fix-workspace-invite-acceptance):
// public /invite route, pending-token persistence + login handoff,
// authenticated acceptance, and the 403/409/410 error states.

import { mount, flushPromises } from '@vue/test-utils'
import { AxiosError } from 'axios'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import * as workspacesApi from '../../../services/workspaces'
import * as authApi from '../../../services/auth'
import * as channelsApi from '../../../services/channels'
import { useAuthStore } from '../../../stores/auth'
import { PENDING_INVITE_TOKEN_KEY } from '../../../lib/inviteToken'
import router from '../../../router'
import AcceptInviteView from '../AcceptInviteView.vue'
import VerifyEmailView from '../VerifyEmailView.vue'

vi.mock('../../../services/workspaces', () => ({
  previewInvite: vi.fn(),
  acceptInvite: vi.fn(),
}))
vi.mock('../../../services/auth', () => ({
  me: vi.fn(),
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
  logoutAll: vi.fn(),
  refresh: vi.fn(),
  verifyEmail: vi.fn(),
  resendVerification: vi.fn(),
  forgotPassword: vi.fn(),
  resetPassword: vi.fn(),
  changePassword: vi.fn(),
  googleExchange: vi.fn(),
  linkGoogle: vi.fn(),
  googleAuthUrl: vi.fn(),
}))
vi.mock('../../../services/channels', () => ({ list: vi.fn() }))
vi.mock('../../../services/api', () => ({
  default: { get: vi.fn().mockResolvedValue({ data: { role: 'viewer' } }) },
}))

function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/invite', name: 'invite', component: AcceptInviteView },
      { path: '/', name: 'landing', component: { template: '<div>landing</div>' } },
      { path: '/verify-email', name: 'verify', component: VerifyEmailView },
      { path: '/login', name: 'login', component: { template: '<div>login</div>' } },
      { path: '/dashboard', name: 'dashboard', component: { template: '<div>dashboard</div>' } },
      { path: '/onboarding', name: 'onboarding', component: { template: '<div>onboarding</div>' } },
    ],
  })
}

async function mountInvite(authenticated: boolean, path = '/invite?token=raw-token') {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  if (authenticated) {
    auth.accessToken = 'access'
    auth.user = {
      id: 'u-1',
      email: 'me@example.com',
      email_verified_at: '2026-01-01T00:00:00Z',
    } as never
  }
  const testRouter = makeRouter()
  await testRouter.push(path)
  await testRouter.isReady()
  const wrapper = mount(AcceptInviteView, { global: { plugins: [pinia, testRouter] } })
  await flushPromises()
  await flushPromises()
  return { wrapper, router: testRouter, auth }
}

function httpError(status: number, data: Record<string, unknown> = {}): AxiosError {
  return new AxiosError('Request failed', 'ERR_BAD_REQUEST', undefined, undefined, {
    status,
    data,
  } as never)
}

describe('AcceptInviteView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
    vi.mocked(workspacesApi.previewInvite).mockResolvedValue({
      workspace_name: 'Studio',
      email: 'me@example.com',
      role: 'viewer',
    })
    vi.mocked(channelsApi.list).mockResolvedValue([{ id: 'c-1' }] as never)
    vi.mocked(authApi.me).mockResolvedValue({
      id: 'u-1',
      email: 'me@example.com',
      email_verified_at: '2026-01-01T00:00:00Z',
    } as never)
  })

  afterEach(() => {
    sessionStorage.clear()
  })

  it('routes /invite publicly, never to the catch-all', () => {
    const resolved = router.resolve({ path: '/invite', query: { token: 'abc' } })
    expect(resolved.name).toBe('invite')
    const meta = resolved.matched.map((m) => m.meta)
    expect(meta.some((m) => m.requiresAuth)).toBe(false)
    expect(meta.some((m) => m.requiresVerified)).toBe(false)
    expect(meta.some((m) => m.requiresOnboarded)).toBe(false)
  })

  it('stores the token and hands an unauthenticated visitor to login with next', async () => {
    const { router: testRouter } = await mountInvite(false)

    expect(sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBe('raw-token')
    expect(testRouter.currentRoute.value.path).toBe('/login')
    expect(testRouter.currentRoute.value.query.next).toBe('/invite?token=raw-token')
    expect(workspacesApi.acceptInvite).not.toHaveBeenCalled()
  })

  it('accepts for an authenticated visitor, clears the token, and navigates on', async () => {
    vi.mocked(workspacesApi.acceptInvite).mockResolvedValue({
      workspace_id: 'ws-1',
      workspace_name: 'Studio',
      role: 'viewer',
    })
    const { router: testRouter } = await mountInvite(true)

    expect(workspacesApi.acceptInvite).toHaveBeenCalledWith('raw-token')
    expect(sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBeNull()
    expect(testRouter.currentRoute.value.path).toBe('/dashboard')
  })

  it('keeps the token and offers an account switch on 403', async () => {
    vi.mocked(workspacesApi.acceptInvite).mockRejectedValue(
      httpError(403, { error: 'invite email does not match your account email' }),
    )
    const { wrapper } = await mountInvite(true)

    expect(wrapper.text()).toContain('different account')
    expect(wrapper.text()).toContain('me@example.com')
    expect(sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBe('raw-token')
  })

  it('explains the seat limit and clears the token on 409', async () => {
    vi.mocked(workspacesApi.acceptInvite).mockRejectedValue(
      httpError(409, { error: 'seat limit reached, upgrade your plan' }),
    )
    const { wrapper } = await mountInvite(true)

    expect(wrapper.text()).toContain('No free seats')
    expect(sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBeNull()
  })

  it('explains an expired invite and clears the token on 410', async () => {
    vi.mocked(workspacesApi.acceptInvite).mockRejectedValue(
      httpError(410, { error: 'invite expired or already used' }),
    )
    const { wrapper } = await mountInvite(true)

    expect(wrapper.text()).toContain('expired or already used')
    expect(sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBeNull()
  })

  it('routes an unverified session to verification while keeping the token', async () => {
    vi.mocked(workspacesApi.acceptInvite).mockRejectedValue(
      httpError(403, { code: 'email_not_verified' }),
    )
    const { router: testRouter } = await mountInvite(true)

    expect(testRouter.currentRoute.value.path).toBe('/verify-email')
    expect(sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBe('raw-token')
  })
})

describe('VerifyEmailView pending invite', () => {
  it('returns to /invite after verification when a token is pending', async () => {
    vi.useFakeTimers()
    try {
      sessionStorage.setItem(PENDING_INVITE_TOKEN_KEY, 'raw-token')
      const pinia = createPinia()
      setActivePinia(pinia)
      const auth = useAuthStore()
      auth.accessToken = 'access'
      vi.mocked(authApi.verifyEmail).mockResolvedValue(undefined as never)
      vi.mocked(channelsApi.list).mockResolvedValue([] as never)

      const testRouter = makeRouter()
      await testRouter.push('/verify-email?token=verify-token')
      await testRouter.isReady()
      mount(VerifyEmailView, { global: { plugins: [pinia, testRouter] } })
      await flushPromises()

      vi.advanceTimersByTime(1300)
      await flushPromises()

      expect(testRouter.currentRoute.value.path).toBe('/invite')
    } finally {
      vi.useRealTimers()
    }
  })
})
