// Workspace-role store regression (fix-frontend-plumbing §1): fetchRole
// resolves the role from the server; canExport / canManageMembers derive
// from it; a failed fetch degrades to viewer.

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useWorkspaceRoleStore } from '../workspaceRole'
import apiClient from '../../services/api'

vi.mock('../../services/api', () => ({
  default: { get: vi.fn() },
}))

describe('workspaceRole store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('analyst role enables export but not member management', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ data: { role: 'analyst' } })
    const store = useWorkspaceRoleStore()
    await store.fetchRole()
    expect(store.role).toBe('analyst')
    expect(store.canExport).toBe(true)
    expect(store.canManageMembers).toBe(false)
  })

  it('admin role enables both export and member management', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ data: { role: 'admin' } })
    const store = useWorkspaceRoleStore()
    await store.fetchRole()
    expect(store.canExport).toBe(true)
    expect(store.canManageMembers).toBe(true)
  })

  it('failed fetch degrades to viewer (gates closed)', async () => {
    vi.mocked(apiClient.get).mockRejectedValue(new Error('403'))
    const store = useWorkspaceRoleStore()
    await store.fetchRole()
    expect(store.role).toBe('viewer')
    expect(store.canExport).toBe(false)
    expect(store.canManageMembers).toBe(false)
  })
})