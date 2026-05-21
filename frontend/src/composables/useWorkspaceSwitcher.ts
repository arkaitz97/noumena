import { computed, ref } from 'vue'
import apiClient from '../services/api'

export interface Workspace {
  id: string
  name: string
  role: 'admin' | 'analyst' | 'viewer'
  is_active: boolean
}

const workspaces = ref<Workspace[]>([])
const loading = ref(false)

export async function fetchWorkspaces(): Promise<Workspace[]> {
  loading.value = true
  try {
    const { data } = await apiClient.get<Workspace[]>('/workspaces/mine')
    workspaces.value = data ?? []
    return workspaces.value
  } catch {
    return []
  } finally {
    loading.value = false
  }
}

export async function activateWorkspace(id: string): Promise<boolean> {
  try {
    await apiClient.put(`/workspaces/${encodeURIComponent(id)}/activate`)
    await fetchWorkspaces()
    return true
  } catch {
    return false
  }
}

export function useWorkspaceSwitcher() {
  const currentWorkspace = computed(() => workspaces.value.find((w) => w.is_active) ?? null)

  return {
    workspaces: computed(() => workspaces.value),
    currentWorkspace,
    loading: computed(() => loading.value),
    fetchWorkspaces,
    activateWorkspace,
  }
}
