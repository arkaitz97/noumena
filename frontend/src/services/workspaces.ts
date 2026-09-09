// /api/workspaces/* member-management surface (fix-frontend-plumbing).
// Admin-only on the backend; the UI mirrors that with role gates.

import apiClient from './api'

export interface WorkspaceMember {
  user_id: string
  email: string
  role: 'admin' | 'analyst' | 'viewer'
  created_at: string
}

export type MemberRole = 'admin' | 'analyst' | 'viewer'

/**
 * Seat counters returned by workspace + member endpoints
 * (implement-missing-product-features §4.3). `member_count` is active
 * members; `seats_limit` is the plan's seat cap (Free/Creator 1, Studio 5,
 * Agency 20). Legacy workspaces may sit over the cap — render, don't crash.
 */
export interface WorkspaceSeats {
  member_count: number
  seats_limit: number | null
}

/** `GET /workspaces/{id}/members` — object envelope (was a bare array). */
export interface MembersList extends WorkspaceSeats {
  members: WorkspaceMember[]
}

/** Workspace summary row (`GET /workspaces/mine`, `/workspaces`, `/workspaces/{id}`). */
export interface WorkspaceSummary extends WorkspaceSeats {
  id: string
  name: string
  role: MemberRole
  is_active: boolean
}

export async function listMembers(workspaceId: string): Promise<MembersList> {
  const { data } = await apiClient.get<Partial<MembersList> | null>(
    `/workspaces/${encodeURIComponent(workspaceId)}/members`,
  )
  const members = data?.members ?? []
  return {
    members,
    member_count: data?.member_count ?? members.length,
    seats_limit: data?.seats_limit ?? null,
  }
}

export async function inviteMember(
  workspaceId: string,
  email: string,
  role: MemberRole,
): Promise<void> {
  await apiClient.post(`/workspaces/${encodeURIComponent(workspaceId)}/invites`, { email, role })
}

export async function changeMemberRole(
  workspaceId: string,
  userId: string,
  role: MemberRole,
): Promise<void> {
  await apiClient.patch(`/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(userId)}`, {
    role,
  })
}

export async function removeMember(workspaceId: string, userId: string): Promise<void> {
  await apiClient.delete(
    `/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(userId)}`,
  )
}