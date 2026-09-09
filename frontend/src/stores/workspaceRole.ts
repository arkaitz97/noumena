// Pinia workspace-role store (fix-frontend-plumbing). The composable
// `useWorkspaceRole` delegates here so role state lives in one reactive
// store fetched at bootstrap / login / workspace switch.

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import apiClient from '../services/api'

export type WorkspaceRole = 'admin' | 'analyst' | 'viewer'

export const useWorkspaceRoleStore = defineStore('workspaceRole', () => {
  const role = ref<WorkspaceRole>('viewer')

  async function fetchRole(): Promise<void> {
    try {
      const { data } = await apiClient.get<{ role: WorkspaceRole }>('/workspaces/active/role')
      role.value = data?.role ?? 'viewer'
    } catch {
      // Non-fatal: role gates stay closed (viewer) until the server
      // answers. Backend enforcement is the security boundary anyway.
      role.value = 'viewer'
    }
  }

  const isAdmin = computed(() => role.value === 'admin')
  const isAnalyst = computed(() => role.value === 'analyst')
  const isViewer = computed(() => role.value === 'viewer')
  const canManageMembers = computed(() => isAdmin.value)
  const canExport = computed(() => isAdmin.value || isAnalyst.value)

  return {
    role,
    isAdmin,
    isAnalyst,
    isViewer,
    canManageMembers,
    canExport,
    fetchRole,
  }
})