<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    description?: string
    confirmLabel?: string
    cancelLabel?: string
    destructive?: boolean
    confirmDisabled?: boolean
    /** When true, omit the default confirm button so the slot can render its own footer. */
    customFooter?: boolean
  }>(),
  {
    confirmLabel: 'Confirm',
    cancelLabel: 'Cancel',
    destructive: false,
    confirmDisabled: false,
    customFooter: false,
  },
)

const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'cancel'): void
  (e: 'update:open', v: boolean): void
}>()

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
  if (!props.open) return
  if (e.key === 'Escape') {
    e.preventDefault()
    closeModal()
  } else if (e.key === 'Tab') {
    trapFocus(e)
  }
}

function closeModal(): void {
  emit('update:open', false)
  emit('cancel')
}

function confirm(): void {
  if (props.confirmDisabled) return
  emit('confirm')
}

watch(
  () => props.open,
  async (v) => {
    if (v) {
      previouslyFocused = (document.activeElement as HTMLElement) || null
      await nextTick()
      const items = focusable()
      // Focus the cancel (safer default) on destructive modals; otherwise the first focusable.
      const target = props.destructive ? items.find((i) => i.dataset.modalCancel) : items[0]
      ;(target ?? items[0])?.focus()
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
    <div
      v-if="open"
      class="fixed z-50 inset-0 overflow-y-auto"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="title"
    >
      <div class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
        <div
          class="fixed inset-0 bg-zinc-900/60 backdrop-blur-sm transition-opacity"
          aria-hidden="true"
          @click="closeModal"
        ></div>
        <span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>
        <div
          ref="dialogRef"
          class="inline-block align-bottom bg-white rounded-2xl px-4 pt-5 pb-6 text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full sm:p-8"
        >
          <div class="sm:flex sm:items-start">
            <div
              :class="[
                'mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full sm:mx-0 sm:h-10 sm:w-10',
                destructive ? 'bg-rose-100' : 'bg-zinc-100',
              ]"
            >
              <svg
                v-if="destructive"
                class="h-6 w-6 text-rose-600"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                />
              </svg>
              <svg
                v-else
                class="h-6 w-6 text-zinc-700"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01" />
              </svg>
            </div>
            <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left w-full">
              <h3 class="text-xl leading-6 font-semibold text-zinc-900 mb-2">{{ title }}</h3>
              <div class="mt-2 text-sm text-zinc-500 leading-relaxed">
                <p v-if="description">{{ description }}</p>
                <slot name="step" />
                <slot />
              </div>
            </div>
          </div>
          <div class="mt-8 sm:flex sm:flex-row-reverse gap-3">
            <slot name="footer">
              <button
                v-if="!customFooter"
                type="button"
                @click="confirm"
                :disabled="confirmDisabled"
                :class="[
                  'w-full inline-flex justify-center rounded-xl border border-transparent shadow-sm px-5 py-2.5 text-sm font-medium text-white sm:w-auto disabled:opacity-50 disabled:cursor-not-allowed transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2',
                  destructive
                    ? 'bg-rose-600 hover:bg-rose-700 focus:ring-rose-500'
                    : 'bg-zinc-900 hover:bg-zinc-800 focus:ring-zinc-900',
                ]"
              >
                {{ confirmLabel }}
              </button>
              <button
                v-if="!customFooter"
                type="button"
                data-modal-cancel="true"
                @click="closeModal"
                class="mt-3 sm:mt-0 w-full inline-flex justify-center rounded-xl border border-zinc-200 shadow-sm px-5 py-2.5 bg-white text-sm font-medium text-zinc-700 hover:bg-zinc-50 sm:w-auto transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900"
              >
                {{ cancelLabel }}
              </button>
            </slot>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
