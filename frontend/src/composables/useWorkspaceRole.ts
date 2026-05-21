import { computed, ref } from 'vue'
import apiClient from '../services/api'

export type WorkspaceRole = 'admin' | 'analyst' | 'viewer'

const currentRole = ref<WorkspaceRole>('viewer')

export async function fetchCurrentWorkspaceRole(): Promise<void> {
  try {
    const { data } = await apiClient.get<{ role: WorkspaceRole }>('/workspaces/active/role')
    currentRole.value = data?.role ?? 'viewer'
  } catch {
    currentRole.value = 'viewer'
  }
}

export function useWorkspaceRole() {
  const role = computed(() => currentRole.value)
  const isAdmin = computed(() => role.value === 'admin')
  const isAnalyst = computed(() => role.value === 'analyst')
  const isViewer = computed(() => role.value === 'viewer')
  const canManageMembers = computed(() => isAdmin.value)
  const canExport = computed(() => isAdmin.value || isAnalyst.value)

  return { role, isAdmin, isAnalyst, isViewer, canManageMembers, canExport }
}
