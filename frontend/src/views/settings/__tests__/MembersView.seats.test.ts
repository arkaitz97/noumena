// Members seat-usage header regression (implement-missing-product-features
// §4.3): the roster header renders "X of Y seats" from the member-list
// envelope, flips to a warning (+N over plan limit) instead of crashing on
// legacy over-cap workspaces, and surfaces the backend's seat-limit message
// when a role change bounces with 409.

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as workspacesApi from '../../../services/workspaces'
import apiClient from '../../../services/api'
import MembersView from '../MembersView.vue'

vi.mock('../../../services/api', () => ({
  default: { get: vi.fn() },
}))
vi.mock('../../../services/workspaces')
vi.mock('../../../lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))
vi.mock('../../../stores/auth', () => ({
  useAuthStore: () => ({ user: { id: 'me' } }),
}))
vi.mock('../../../composables/useWorkspaceRole', () => ({
  useWorkspaceRole: () => ({ isAdmin: true }),
}))

function makeMember(id: string, role: workspacesApi.MemberRole = 'viewer'): workspacesApi.WorkspaceMember {
  return { user_id: id, email: `${id}@example.com`, role, created_at: '2026-01-01T00:00:00Z' }
}

const memberAlice = makeMember('alice')

describe('MembersView seat usage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(apiClient.get).mockResolvedValue({
      data: [
        {
          id: 'ws1',
          name: 'Test Workspace',
          role: 'admin' as const,
          is_active: true,
          member_count: 0,
          seats_limit: 5,
        },
      ],
    })
    vi.mocked(workspacesApi.inviteMember).mockResolvedValue(undefined)
    vi.mocked(workspacesApi.changeMemberRole).mockResolvedValue(undefined)
    vi.mocked(workspacesApi.removeMember).mockResolvedValue(undefined)
  })

  async function mountWithMembers(res: workspacesApi.MembersList) {
    vi.mocked(workspacesApi.listMembers).mockResolvedValue(res)
    const wrapper = mount(MembersView)
    await flushPromises()
    return wrapper
  }

  it('renders "X of Y seats" from the members response envelope', async () => {
    const wrapper = await mountWithMembers({
      members: [memberAlice, makeMember('bob', 'analyst'), makeMember('carol', 'admin')],
      member_count: 3,
      seats_limit: 5,
    })

    expect(wrapper.text()).toContain('3 of 5 seats')
    expect(wrapper.text()).not.toContain('over plan limit')
    // The roster still lists every member.
    expect(wrapper.text()).toContain('alice@example.com')
    expect(wrapper.text()).toContain('bob@example.com')
    expect(wrapper.text()).toContain('carol@example.com')
    expect(wrapper.findAll('li')).toHaveLength(3)
  })

  it('renders a warning (not a crash) when a legacy workspace is over its seat cap', async () => {
    const six = [memberAlice]
    for (let i = 1; i < 6; i++) six.push(makeMember(`member${i}`))
    const wrapper = await mountWithMembers({
      members: six,
      member_count: 6,
      seats_limit: 5,
    })

    expect(wrapper.text()).toContain('6 of 5 seats')
    expect(wrapper.text()).toContain('+1 over plan limit')
    expect(wrapper.find('span.bg-amber-50').exists()).toBe(true) // unslop-ignore
    // Members still render — no crash on over-cap state.
    expect(wrapper.findAll('li')).toHaveLength(6)
    expect(wrapper.text()).toContain('member5@example.com')
  })

  it('falls back to the workspace summary when the members envelope omits seats_limit', async () => {
    const wrapper = await mountWithMembers({
      members: [memberAlice, makeMember('bob')],
      member_count: 2,
      seats_limit: null,
    })

    expect(wrapper.text()).toContain('2 of 5 seats')
    expect(wrapper.text()).not.toContain('over plan limit')
    expect(wrapper.findAll('li')).toHaveLength(2)
  })

  it('shows the plain member count when no seat info exists anywhere', async () => {
    // Workspace detail also lacks seat fields (pre-seat backend).
    vi.mocked(apiClient.get).mockResolvedValue({
      data: [{ id: 'ws1', name: 'Test Workspace', role: 'admin' as const, is_active: true }],
    })
    const wrapper = await mountWithMembers({
      members: [memberAlice, makeMember('bob')],
      member_count: 2,
      seats_limit: null,
    })

    expect(wrapper.text()).not.toContain('seats')
    expect(wrapper.text()).toContain('2 members')
    expect(wrapper.findAll('li')).toHaveLength(2)
  })

  it('surfaces the server seat-limit message when a role change bounces with 409', async () => {
    const wrapper = await mountWithMembers({
      members: [memberAlice],
      member_count: 1,
      seats_limit: 1,
    })
    vi.mocked(workspacesApi.changeMemberRole).mockRejectedValue({
      response: { status: 409, data: { error: 'seat limit reached, upgrade your plan' } },
    })

    await wrapper.find('select[aria-label="Role for alice@example.com"]').setValue('admin')
    await flushPromises()

    const { toast } = await import('../../../lib/toast')
    expect(toast.error).toHaveBeenCalledWith('seat limit reached, upgrade your plan')
    expect(toast.error).not.toHaveBeenCalledWith('Could not update the role.')
  })
})
