<script setup lang="ts">
import { computed } from 'vue'
import { dismiss, useToastState } from '../lib/toast'

const state = useToastState()
const items = computed(() => state.items)

const variantClass = (v: string) => {
  switch (v) {
    case 'success':
      return 'bg-emerald-50 dark:bg-emerald-400/10 text-emerald-800 dark:text-emerald-200 ring-emerald-600/20 dark:ring-emerald-400/30'
    case 'error':
      return 'bg-rose-50 dark:bg-rose-400/10 text-rose-800 dark:text-rose-200 ring-rose-600/20 dark:ring-rose-400/30'
    default:
      return 'bg-white dark:bg-zinc-900 text-zinc-800 dark:text-zinc-200 ring-zinc-950/5 dark:ring-white/10'
  }
}
</script>

<template>
  <div
    aria-live="polite"
    class="fixed bottom-4 right-4 z-[100] flex flex-col gap-2 max-w-sm w-[calc(100%-2rem)] sm:w-auto pointer-events-none"
  >
    <transition-group name="toast">
      <div
        v-for="t in items"
        :key="t.id"
        class="pointer-events-auto shadow-pop ring-1 rounded-xl px-4 py-3 text-sm font-medium flex items-start gap-3"
        :class="variantClass(t.variant)"
      >
        <span class="flex-1 leading-relaxed">{{ t.message }}</span>
        <button
          @click="dismiss(t.id)"
          class="text-current opacity-60 hover:opacity-100 transition-all duration-200 active:scale-95 shrink-0 p-1 -m-1 rounded-md focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
          aria-label="Dismiss"
        >
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </transition-group>
  </div>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: opacity 0.25s cubic-bezier(0.16, 1, 0.3, 1), transform 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
