<script setup lang="ts">
// SentimentSparkline — compact net-score sparkline for the dashboard
// sidebar (spec 10-frontend-dashboard §4.4 / DEFERRED_TASKS 4.4).
// Always shows the trailing 7 points regardless of the chart's active
// window. Dependency-free SVG, matching the chart component's visual
// language.

import { computed } from 'vue'
import type { SentimentData } from '../services/insights'

const props = defineProps<{
  data: SentimentData | null
}>()

const WIDTH = 160
const HEIGHT = 40

const scores = computed(() => (props.data?.series ?? []).slice(-7).map((d) => d.score))

const range = computed(() => {
  if (scores.value.length === 0) return { min: -1, max: 1 }
  const max = Math.max(...scores.value, 0)
  const min = Math.min(...scores.value, 0)
  return { min, max: min === max ? min + 1 : max }
})

const points = computed(() => {
  const n = scores.value.length
  if (n === 0) return ''
  const { min, max } = range.value
  const span = max - min
  return scores.value
    .map((s, i) => {
      const x = n === 1 ? WIDTH / 2 : (i / (n - 1)) * WIDTH
      const y = HEIGHT - 2 - ((s - min) / span) * (HEIGHT - 4)
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
})

const netScore = computed(() => scores.value.reduce((a, b) => a + b, 0))

const trend = computed(() => {
  if (scores.value.length < 2) return 'flat'
  const last = scores.value[scores.value.length - 1]
  const first = scores.value[0]
  if (last > first) return 'up'
  if (last < first) return 'down'
  return 'flat'
})
</script>

<template>
  <div class="bg-white dark:bg-zinc-900 shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-5">
    <div class="flex items-center justify-between mb-2">
      <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider">Sentiment</p>
      <span
        v-if="scores.length > 0"
        class="inline-flex items-center px-1.5 py-0.5 rounded-md text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset"
        :class="
          trend === 'up'
            ? 'bg-emerald-50 text-emerald-700 ring-emerald-600/20'
            : trend === 'down'
              ? 'bg-rose-50 text-rose-700 ring-rose-600/20'
              : 'bg-zinc-50 text-zinc-600 ring-zinc-500/20'
        "
      >
        {{ trend }}
      </span>
    </div>

    <svg v-if="scores.length > 0" viewBox="0 0 160 40" class="w-full h-10" role="img" aria-label="7-day sentiment score sparkline">
      <defs>
        <linearGradient id="sentiment-spark-stroke" x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stop-color="#22d3ee" />
          <stop offset="100%" stop-color="#06b6d4" />
        </linearGradient>
      </defs>
      <polyline
        :points="points"
        fill="none"
        stroke="url(#sentiment-spark-stroke)"
        stroke-width="2"
        stroke-linejoin="round"
        stroke-linecap="round"
      />
    </svg>
    <div v-else class="h-10 flex items-center justify-center text-xs text-zinc-400">
      No data yet
    </div>

    <p class="mt-2 text-[10px] text-zinc-400">
      Net score
      <span class="font-semibold font-mono tabular-nums text-zinc-600 dark:text-zinc-300">{{ netScore }}</span>
    </p>
  </div>
</template>