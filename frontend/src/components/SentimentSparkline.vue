<script setup lang="ts">
import { computed } from 'vue'
import type { SentimentBucket } from '../services/types'

const props = defineProps<{ series: SentimentBucket[] }>()

const W = 200
const H = 48
const PAD = 4

const points = computed(() => {
  const s = props.series ?? []
  if (s.length === 0) return ''
  const innerW = W - PAD * 2
  const innerH = H - PAD * 2
  return s
    .map((b, i) => {
      const x = PAD + (s.length === 1 ? innerW / 2 : (i / (s.length - 1)) * innerW)
      const v = Math.max(-1, Math.min(1, b.avg ?? 0))
      const y = PAD + (1 - (v + 1) / 2) * innerH
      return `${x},${y}`
    })
    .join(' ')
})

const avg = computed(() => {
  const s = props.series ?? []
  if (s.length === 0) return 0
  return s.reduce((a, b) => a + (b.avg ?? 0), 0) / s.length
})
const lineColor = computed(() => {
  const a = avg.value
  if (a >= 0.3) return '#10b981'
  if (a <= -0.3) return '#f43f5e'
  return '#71717a'
})
</script>

<template>
  <svg :viewBox="`0 0 ${W} ${H}`" class="w-full h-12" aria-hidden="true">
    <polyline
      v-if="points"
      :points="points"
      fill="none"
      :stroke="lineColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
  </svg>
</template>
