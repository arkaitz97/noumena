// Workspaces service unwrap regression (implement-missing-product-features
// §4.3): `GET /workspaces/{id}/members` changed from a bare array to an
// object envelope `{ members, member_count, seats_limit }` — listMembers
// must unwrap `.members` and expose the seat counters.

import { beforeEach, describe, expect, it, vi } from 'vitest'

import apiClient from '../../services/api'
import { listMembers, type WorkspaceMember } from '../workspaces'

vi.mock('../../services/api', () => ({
  default: { get: vi.fn() },
}))

function makeMember(id: string): WorkspaceMember {
  return { user_id: id, email: `${id}@example.com`, role: 'viewer', created_at: '2026-01-01T00:00:00Z' }
}

describe('listMembers unwrap', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('unwraps .members from the object envelope and keeps seat counters', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      data: {
        members: [makeMember('a'), makeMember('b')],
        member_count: 2,
        seats_limit: 5,
      },
    })

    const res = await listMembers('ws1')

    expect(apiClient.get).toHaveBeenCalledWith('/workspaces/ws1/members')
    expect(res.members).toHaveLength(2)
    expect(res.members[0].email).toBe('a@example.com')
    expect(res.member_count).toBe(2)
    expect(res.seats_limit).toBe(5)
  })

  it('derives member_count from the unwrapped list and tolerates a null envelope', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: null })
    const empty = await listMembers('ws1')
    expect(empty.members).toEqual([])
    expect(empty.member_count).toBe(0)
    expect(empty.seats_limit).toBeNull()

    vi.mocked(apiClient.get).mockResolvedValueOnce({
      data: { members: [makeMember('a')], seats_limit: 1 },
    })
    const res = await listMembers('ws1')
    expect(res.members).toHaveLength(1)
    expect(res.member_count).toBe(1)
  })
})
