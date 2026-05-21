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
    class="bg-amber-50 border border-amber-200 rounded-xl px-4 py-3 flex items-center gap-3"
    role="alert"
  >
    <AlertTriangle class="w-5 h-5 text-amber-500 flex-shrink-0" />
    <p class="text-sm text-amber-800 flex-1">{{ message }}</p>
    <button
      type="button"
      aria-label="Dismiss"
      @click="emit('dismiss')"
      class="p-1 rounded-lg text-amber-500 hover:text-amber-700 hover:bg-amber-100 focus:outline-none focus:ring-2 focus:ring-amber-500"
    >
      <X class="w-4 h-4" />
    </button>
  </div>
</template>
