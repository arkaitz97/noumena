<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  used: number
  limit: number
  unit?: string
  label?: string
  unlimited?: boolean
}>()

const ratio = computed(() => {
  if (props.unlimited || !props.limit || props.limit <= 0) return 0
  return Math.min(1, Math.max(0, props.used / props.limit))
})

const colorClass = computed(() => {
  const r = ratio.value
  if (r >= 0.95) return 'bg-rose-500'
  if (r >= 0.75) return 'bg-amber-500'
  return 'bg-emerald-500'
})

const widthStyle = computed(() => ({ width: `${(ratio.value * 100).toFixed(1)}%` }))

function fmt(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return String(n)
}
</script>

<template>
  <div>
    <div v-if="label || unit" class="flex items-center justify-between text-xs mb-1.5 font-medium">
      <span class="text-zinc-700">{{ label }}</span>
      <span class="text-zinc-500 font-mono tabular-nums">{{ fmt(used) }} / {{ unlimited ? 'Unlimited' : fmt(limit) }}{{ unit ? ' ' + unit : '' }}</span>
    </div>
    <div
      class="w-full bg-zinc-100 rounded-full h-2.5 overflow-hidden border border-zinc-200/50"
      role="progressbar"
      :aria-valuenow="used"
      :aria-valuemin="0"
      :aria-valuemax="unlimited ? Math.max(1, used) : limit"
      :aria-label="label || 'Usage'"
    >
      <div :class="['h-full rounded-full transition-all duration-500', colorClass]" :style="widthStyle" data-unslop-ignore></div>
    </div>
  </div>
</template>
