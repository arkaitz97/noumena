// Logout server-confirm regression (fix-frontend-plumbing §8.2): local
// state must only clear AFTER POST /auth/logout succeeds. A failed logout
// keeps the user authenticated (the HttpOnly refresh cookie is still valid).

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as authApi from '../../services/auth'
import * as channelsApi from '../../services/channels'
import { useAuthStore } from '../auth'

vi.mock('../../services/auth')
vi.mock('../../services/channels')
vi.mock('../../composables/useWorkspaceRole', () => ({
  fetchCurrentWorkspaceRole: vi.fn(async () => undefined),
}))

describe('auth store logout (server-confirmed)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.mocked(authApi.login).mockResolvedValue({ access_token: 'tok', message: 'ok' })
    vi.mocked(authApi.me).mockResolvedValue({
      id: 'u1',
      email: 'a@b.com',
      email_verified_at: new Date().toISOString(),
    })
    vi.mocked(channelsApi.list).mockResolvedValue([])
  })

  it('keeps the session when the server rejects logout', async () => {
    const auth = useAuthStore()
    await auth.login('a@b.com', 'pw')
    expect(auth.isAuthenticated).toBe(true)

    vi.mocked(authApi.logout).mockRejectedValue(new Error('network down'))
    await expect(auth.logout()).rejects.toBeTruthy()
    // No fake logout: access token + user survive, cookie stays valid.
    expect(auth.isAuthenticated).toBe(true)
    expect(auth.user?.email).toBe('a@b.com')
  })

  it('clears state only after the server confirms logout', async () => {
    const auth = useAuthStore()
    await auth.login('a@b.com', 'pw')
    vi.mocked(authApi.logout).mockResolvedValue(undefined)

    await auth.logout()
    expect(auth.isAuthenticated).toBe(false)
    expect(auth.user).toBeNull()
  })
})