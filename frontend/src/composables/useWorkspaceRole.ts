import { useWorkspaceRoleStore } from '../stores/workspaceRole'

export type { WorkspaceRole } from '../stores/workspaceRole'

/**
 * Fetch the caller's role in their active workspace into the pinia store.
 * Call at bootstrap, after login, and after a workspace switch so role-gated
 * UI mirrors the server decision.
 */
export async function fetchCurrentWorkspaceRole(): Promise<void> {
  const store = useWorkspaceRoleStore()
  await store.fetchRole()
}

export function useWorkspaceRole() {
  const store = useWorkspaceRoleStore()
  return {
    role: store.role,
    isAdmin: store.isAdmin,
    isAnalyst: store.isAnalyst,
    isViewer: store.isViewer,
    canManageMembers: store.canManageMembers,
    canExport: store.canExport,
  }
}