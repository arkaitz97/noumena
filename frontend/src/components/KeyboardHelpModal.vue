<script setup lang="ts">
import { X } from 'lucide-vue-next'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

const shortcuts = [
  { key: 's', description: 'Star/unstar focused comment' },
  { key: 'j', description: 'Move focus to next comment' },
  { key: 'k', description: 'Move focus to previous comment' },
  { key: '/', description: 'Focus search input' },
  { key: '?', description: 'Toggle this help modal' },
]
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible"
      class="fixed z-50 inset-0 overflow-y-auto"
      role="dialog"
      aria-modal="true"
      aria-labelledby="keyboard-help-title"
    >
      <div class="flex items-center justify-center min-h-screen px-4">
        <div class="fixed inset-0 bg-zinc-900/60 backdrop-blur-sm" @click="emit('close')"></div>

        <div class="relative bg-white rounded-2xl shadow-xl max-w-md w-full p-6 sm:p-8">
          <div class="flex items-start justify-between">
            <h3 id="keyboard-help-title" class="text-lg font-semibold text-zinc-900">Keyboard shortcuts</h3>
            <button
              type="button"
              aria-label="Close"
              @click="emit('close')"
              class="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-700 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-zinc-900"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          <div class="mt-5 space-y-3">
            <div
              v-for="s in shortcuts"
              :key="s.key"
              class="flex items-center justify-between"
            >
              <span class="text-sm text-zinc-700">{{ s.description }}</span>
              <kbd class="inline-flex items-center justify-center min-w-[2rem] px-2 py-1 text-xs font-mono font-semibold text-zinc-700 bg-zinc-100 border border-zinc-200 rounded-lg">
                {{ s.key }}
              </kbd>
            </div>
          </div>

          <p class="mt-5 text-xs text-zinc-400">Shortcuts are disabled when focus is inside an input field.</p>
        </div>
      </div>
    </div>
  </Teleport>
</template>
