<script setup lang="ts">
import { computed } from 'vue'
import type { ToxicityResponse } from '../services/types'

const props = defineProps<{
  data: ToxicityResponse | null
  windowKey?: '7d' | '30d'
  compact?: boolean
}>()

const SIZE = computed(() => (props.compact ? 96 : 140))
const STROKE = computed(() => (props.compact ? 10 : 14))

const pct = computed(() => {
  const d = props.data
  if (!d) return 0
  if (!d.total_comments || d.total_comments === 0) return 0
  return Math.max(0, Math.min(100, d.filtered_pct))
})

const colorClass = computed(() => {
  const p = pct.value
  if (p < 5) return '#10b981' // green
  if (p < 15) return '#f59e0b' // amber
  return '#f43f5e' // rose
})

const radius = computed(() => (SIZE.value - STROKE.value) / 2)
const circumference = computed(() => 2 * Math.PI * radius.value)
const dashOffset = computed(() => circumference.value * (1 - pct.value / 100))

const display = computed(() => `${pct.value.toFixed(1)}%`)
</script>

<template>
  <div class="flex flex-col items-center justify-center" role="img" :aria-label="`Toxicity shield ${display}`">
    <svg :width="SIZE" :height="SIZE" :viewBox="`0 0 ${SIZE} ${SIZE}`">
      <circle
        :cx="SIZE / 2"
        :cy="SIZE / 2"
        :r="radius"
        fill="none"
        stroke="#e4e4e7"
        :stroke-width="STROKE"
      />
      <circle
        :cx="SIZE / 2"
        :cy="SIZE / 2"
        :r="radius"
        fill="none"
        :stroke="colorClass"
        :stroke-width="STROKE"
        stroke-linecap="round"
        :stroke-dasharray="circumference"
        :stroke-dashoffset="dashOffset"
        :transform="`rotate(-90 ${SIZE / 2} ${SIZE / 2})`"
      />
      <text
        :x="SIZE / 2"
        :y="SIZE / 2"
        text-anchor="middle"
        dominant-baseline="central"
        :class="compact ? 'fill-zinc-900 text-sm font-semibold' : 'fill-zinc-900 text-lg font-semibold'"
      >
        {{ display }}
      </text>
    </svg>
    <p v-if="!compact" class="text-[11px] text-zinc-500 mt-1">filtered ({{ windowKey || '7d' }})</p>
  </div>
</template>
