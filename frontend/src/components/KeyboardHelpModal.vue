<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
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

const dialogRef = ref<HTMLElement | null>(null)
let previouslyFocused: HTMLElement | null = null

function focusable(): HTMLElement[] {
  if (!dialogRef.value) return []
  return Array.from(
    dialogRef.value.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])',
    ),
  )
}

function trapFocus(e: KeyboardEvent): void {
  if (e.key !== 'Tab') return
  const items = focusable()
  if (items.length === 0) return
  const first = items[0]
  const last = items[items.length - 1]
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault()
    first.focus()
  }
}

function onKeydown(e: KeyboardEvent): void {
  if (!props.visible) return
  if (e.key === 'Escape') {
    e.preventDefault()
    emit('close')
  } else if (e.key === 'Tab') {
    trapFocus(e)
  }
}

watch(
  () => props.visible,
  async (v) => {
    if (v) {
      previouslyFocused = (document.activeElement as HTMLElement) || null
      await nextTick()
      focusable()[0]?.focus()
    } else if (previouslyFocused) {
      previouslyFocused.focus()
      previouslyFocused = null
    }
  },
)

if (typeof document !== 'undefined') {
  document.addEventListener('keydown', onKeydown)
}
onBeforeUnmount(() => {
  if (typeof document !== 'undefined') {
    document.removeEventListener('keydown', onKeydown)
  }
})
</script>

<template>
  <Teleport to="body">
    <Transition name="help">
      <div
        v-if="visible"
        class="fixed z-50 inset-0 overflow-y-auto"
        role="dialog"
        aria-modal="true"
        aria-labelledby="keyboard-help-title"
      >
        <div class="flex items-center justify-center min-h-screen px-4">
          <div class="fixed inset-0 bg-zinc-950/50 backdrop-blur-sm dark:bg-black/60" @click="emit('close')"></div>

          <div ref="dialogRef" class="relative bg-white dark:bg-zinc-900 rounded-2xl shadow-pop ring-1 ring-zinc-950/5 dark:ring-white/10 max-w-md w-full p-6 sm:p-8">
            <div class="flex items-start justify-between">
              <h3 id="keyboard-help-title" class="text-lg font-bold tracking-tight text-zinc-900 dark:text-white">Keyboard shortcuts</h3>
              <button
                type="button"
                aria-label="Close"
                @click="emit('close')"
                class="flex items-center justify-center h-11 w-11 rounded-lg text-zinc-400 dark:text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-200 hover:bg-zinc-100/80 dark:hover:bg-white/5 transition-all duration-200 active:scale-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
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
                <span class="text-sm text-zinc-700 dark:text-zinc-300">{{ s.description }}</span>
                <kbd class="inline-flex items-center justify-center min-w-[2rem] px-2 py-1 text-xs font-mono font-semibold text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-white/10 shadow-card dark:shadow-none rounded-md">
                  {{ s.key }}
                </kbd>
              </div>
            </div>

            <p class="mt-5 text-xs text-zinc-500 dark:text-zinc-400">Shortcuts are disabled when focus is inside an input field.</p>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.help-enter-active,
.help-leave-active {
  transition: opacity 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.help-enter-from,
.help-leave-to {
  opacity: 0;
}
</style>
