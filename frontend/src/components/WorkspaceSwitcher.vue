<script setup lang="ts">
import { ChevronDown, Building2 } from 'lucide-vue-next'
import { computed, onMounted, ref } from 'vue'
import apiClient from '../services/api'
import { toast } from '../lib/toast'

export interface Workspace {
  id: string
  name: string
  role: 'admin' | 'analyst' | 'viewer'
  is_active: boolean
}

const workspaces = ref<Workspace[]>([])
const loading = ref(false)
const currentWorkspace = computed(() => workspaces.value.find((w) => w.is_active) ?? null)

async function fetchWorkspaces(): Promise<Workspace[]> {
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

async function activateWorkspace(id: string): Promise<boolean> {
  try {
    await apiClient.put(`/workspaces/${encodeURIComponent(id)}/activate`)
    await fetchWorkspaces()
    return true
  } catch {
    return false
  }
}

const open = ref(false)
const menuRef = ref<HTMLElement | null>(null)

onMounted(() => {
  void fetchWorkspaces()
})

function toggle() {
  open.value = !open.value
}

function onWorkspaceClick(w: Workspace) {
  open.value = false
  if (w.is_active) return
  void activateWorkspace(w.id).then((ok) => {
    if (ok) {
      toast.success(`Switched to ${w.name}`)
    } else {
      toast.error('Failed to switch workspace')
    }
  })
}

const label = computed(() => currentWorkspace.value?.name ?? 'Workspace')
</script>

<template>
  <div ref="menuRef" class="relative" @click.outside="open = false">
    <button
      @click="toggle"
      class="flex items-center gap-2 px-3 py-1.5 rounded-lg border border-zinc-200 hover:border-zinc-300 hover:bg-zinc-50 transition-colors text-sm"
      :aria-expanded="open"
      :aria-label="'Switch workspace, current: ' + label"
    >
      <Building2 class="w-4 h-4 text-zinc-500" />
      <span class="max-w-[10rem] truncate font-medium text-zinc-700">{{ label }}</span>
      <ChevronDown class="w-3.5 h-3.5 text-zinc-400" />
    </button>

    <transition name="ws">
      <div
        v-if="open"
        class="absolute left-0 mt-2 w-64 bg-white shadow-lg ring-1 ring-zinc-950/5 rounded-xl py-1 z-50"
      >
        <div
          v-for="w in workspaces"
          :key="w.id"
          @click="onWorkspaceClick(w)"
          class="flex items-center gap-2 px-3 py-2 text-sm cursor-pointer transition-colors"
          :class="w.is_active ? 'bg-zinc-50 text-zinc-900 font-medium' : 'text-zinc-700 hover:bg-zinc-50'"
        >
          <span class="flex-1 truncate">{{ w.name }}</span>
          <span class="text-[10px] uppercase tracking-wider text-zinc-400">{{ w.role }}</span>
          <svg v-if="w.is_active" class="w-4 h-4 text-zinc-900" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <div v-if="loading" class="px-3 py-2 text-xs text-zinc-400">Loading…</div>
        <div v-if="workspaces.length === 0 && !loading" class="px-3 py-2 text-xs text-zinc-400">No workspaces</div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.ws-enter-active,
.ws-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.ws-enter-from,
.ws-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.98);
}
</style>
