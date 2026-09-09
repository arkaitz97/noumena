<script setup lang="ts">
import { ChevronDown, Building2 } from 'lucide-vue-next'
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import apiClient from '../services/api'
import { toast } from '../lib/toast'
import { fetchCurrentWorkspaceRole } from '../composables/useWorkspaceRole'

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
    await fetchCurrentWorkspaceRole()
    return true
  } catch {
    return false
  }
}

const open = ref(false)
const triggerRef = ref<HTMLButtonElement | null>(null)
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

function menuItems(): HTMLElement[] {
  if (!menuRef.value) return []
  return Array.from(menuRef.value.querySelectorAll<HTMLElement>('[role="menuitem"]:not([aria-disabled="true"])'))
}

function focusItemAt(index: number): void {
  const items = menuItems()
  if (items.length === 0) return
  items[(index + items.length) % items.length].focus()
}

function onMenuKeydown(e: KeyboardEvent): void {
  if (!open.value) return
  const items = menuItems()
  if (items.length === 0) return
  const idx = items.indexOf(document.activeElement as HTMLElement)
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    focusItemAt(idx + 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    focusItemAt(idx - 1)
  } else if (e.key === 'Home') {
    e.preventDefault()
    focusItemAt(0)
  } else if (e.key === 'End') {
    e.preventDefault()
    focusItemAt(items.length - 1)
  } else if (e.key === 'Escape') {
    e.preventDefault()
    open.value = false
    triggerRef.value?.focus()
  }
}

watch(open, async (v) => {
  if (v) {
    await nextTick()
    focusItemAt(0)
  }
})

const label = computed(() => currentWorkspace.value?.name ?? 'Workspace')
</script>

<template>
  <div class="relative w-full" @click.outside="open = false">
    <button
      ref="triggerRef"
      @click="toggle"
      class="w-full flex items-center gap-2 px-3 py-1.5 rounded-lg border border-zinc-200 dark:border-white/10 hover:border-zinc-300 dark:hover:border-white/20 hover:bg-zinc-50 dark:hover:bg-zinc-800/60 transition-all duration-200 ease-snappy active:scale-[0.98] text-sm shadow-card dark:shadow-none"
      :aria-expanded="open"
      aria-haspopup="menu"
      :aria-label="'Switch workspace, current: ' + label"
    >
      <Building2 class="w-4 h-4 shrink-0 text-zinc-500 dark:text-zinc-400" />
      <span class="flex-1 min-w-0 truncate font-medium text-zinc-700 dark:text-zinc-300 text-left">{{ label }}</span>
      <ChevronDown class="w-3.5 h-3.5 shrink-0 text-zinc-400 dark:text-zinc-500" />
    </button>

    <transition name="ws">
      <div
        v-if="open"
        ref="menuRef"
        role="menu"
        aria-label="Switch workspace"
        class="absolute left-0 mt-2 w-full min-w-56 bg-white dark:bg-zinc-900 shadow-pop ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl py-1 z-50 origin-top-left"
        @keydown="onMenuKeydown"
      >
        <button
          v-for="w in workspaces"
          :key="w.id"
          type="button"
          role="menuitem"
          :aria-disabled="w.is_active ? 'true' : undefined"
          @click="onWorkspaceClick(w)"
          class="w-full flex items-center gap-2 px-3 py-2 text-sm text-left transition-colors"
          :class="w.is_active ? 'bg-zinc-100/70 dark:bg-white/5 text-zinc-900 dark:text-white font-medium' : 'text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100/70 dark:hover:bg-white/5'"
        >
          <span class="flex-1 truncate">{{ w.name }}</span>
          <span class="text-[10px] uppercase tracking-wider text-zinc-500 dark:text-zinc-400">{{ w.role }}</span>
          <svg v-if="w.is_active" class="w-4 h-4 text-zinc-900 dark:text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
          </svg>
        </button>
        <div v-if="loading" class="px-3 py-2 text-xs text-zinc-500 dark:text-zinc-400">Loading…</div>
        <div v-if="workspaces.length === 0 && !loading" class="px-3 py-2 text-xs text-zinc-500 dark:text-zinc-400">No workspaces</div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.ws-enter-active,
.ws-leave-active {
  transition: opacity 0.2s cubic-bezier(0.16, 1, 0.3, 1), transform 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}
.ws-enter-from,
.ws-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(0.98);
}
</style>
