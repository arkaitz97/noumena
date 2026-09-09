<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { AlertTriangle, X } from 'lucide-vue-next'

const props = defineProps<{
  visible: boolean
  retryAfterSeconds: number
}>()

const emit = defineEmits<{ (e: 'dismiss'): void }>()

const countdown = ref(props.retryAfterSeconds)
let timer: number | null = null

watch(
  () => props.visible,
  (v) => {
    if (v) {
      countdown.value = props.retryAfterSeconds
      startCountdown()
    } else {
      stopCountdown()
    }
  },
)

watch(
  () => props.retryAfterSeconds,
  (v) => {
    if (props.visible) {
      countdown.value = v
      startCountdown()
    }
  },
)

function startCountdown() {
  stopCountdown()
  timer = window.setInterval(() => {
    countdown.value--
    if (countdown.value <= 0) {
      stopCountdown()
      emit('dismiss')
    }
  }, 1000)
}

function stopCountdown() {
  if (timer !== null) {
    clearInterval(timer)
    timer = null
  }
}

onBeforeUnmount(stopCountdown)

const message = computed(() => {
  if (countdown.value > 0) {
    return `Rate limit exceeded. Retry available in ${countdown.value}s.`
  }
  return 'Rate limit exceeded.'
})
</script>

<template>
  <div
    v-if="visible"
    class="bg-amber-50 dark:bg-amber-400/10 ring-1 ring-amber-500/20 dark:ring-amber-400/30 rounded-xl px-4 py-3 flex items-center gap-3" data-unslop-ignore
    role="alert"
  >
    <AlertTriangle class="w-5 h-5 text-amber-500 flex-shrink-0" />
    <p class="text-sm text-amber-800 dark:text-amber-200 flex-1">{{ message }}</p>
    <button
      type="button"
      aria-label="Dismiss"
      @click="emit('dismiss')"
      class="flex items-center justify-center h-11 w-11 rounded-lg text-amber-500 dark:text-amber-400 hover:text-amber-700 dark:hover:text-amber-200 hover:bg-amber-100 dark:hover:bg-amber-400/20 transition-all duration-200 active:scale-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-500" data-unslop-ignore
    >
      <X class="w-4 h-4" />
    </button>
  </div>
</template>
