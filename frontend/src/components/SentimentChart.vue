<script setup lang="ts">
// SentimentChart — per-day negative/neutral/positive counts + net score
// trend for one tracked channel. Spec 10-frontend-dashboard §4.3: props
// `data: SentimentData`, line chart with 7d/30d/90d toggle, colour
// gradient based on sentiment. The window toggle emits `update:days` so
// the owning view re-fetches; the active option reflects the `days` of
// the data currently shown. Rendered as plain SVG — no chart lib in
// package.json (mirrors InsightTypeDistributionChart's dependency-free
// approach).

import { computed } from 'vue'
import type { SentimentData } from '../services/insights'
import SegmentedControl from './SegmentedControl.vue'

const props = defineProps<{
  data: SentimentData | null
}>()

const emit = defineEmits<{
  'update:days': [days: number]
}>()

const DAYS_OPTIONS = [7, 30, 90] as const

const series = computed(() => props.data?.series ?? [])
const activeDays = computed(() => props.data?.days ?? 7)

const totals = computed(() =>
  series.value.reduce(
    (acc, d) => ({
      negative: acc.negative + d.negative,
      neutral: acc.neutral + d.neutral,
      positive: acc.positive + d.positive,
      score: acc.score + d.score,
    }),
    { negative: 0, neutral: 0, positive: 0, score: 0 },
  ),
)

// Long windows make a per-day table unwieldy — show the last 14 rows.
const visibleRows = computed(() => series.value.slice(-14))

const WIDTH = 320
const HEIGHT = 96
const PAD = 6

const scoreRange = computed(() => {
  const scores = series.value.map((d) => d.score)
  if (scores.length === 0) return { min: -1, max: 1 }
  const max = Math.max(...scores, 0)
  const min = Math.min(...scores, 0)
  return { min, max: min === max ? min + 1 : max }
})

const points = computed(() => {
  const n = series.value.length
  if (n === 0) return ''
  const { min, max } = scoreRange.value
  const span = max - min
  return series.value
    .map((d, i) => {
      const x = n === 1 ? WIDTH / 2 : PAD + (i / (n - 1)) * (WIDTH - 2 * PAD)
      const y = HEIGHT - PAD - ((d.score - min) / span) * (HEIGHT - 2 * PAD)
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
})

const areaPoints = computed(() => {
  if (points.value === '') return ''
  const n = series.value.length
  const firstX = n === 1 ? WIDTH / 2 : PAD
  const lastX = n === 1 ? WIDTH / 2 : WIDTH - PAD
  return `${firstX},${HEIGHT - PAD} ${points.value} ${lastX},${HEIGHT - PAD}`
})

// Dashed zero baseline when the range straddles 0.
const zeroY = computed(() => {
  const { min, max } = scoreRange.value
  if (min >= 0 || max <= 0) return null
  return HEIGHT - PAD - (0 - min) / (max - min) * (HEIGHT - 2 * PAD)
})

function scoreColor(score: number): string {
  if (score > 0) return 'text-emerald-600'
  if (score < 0) return 'text-rose-600'
  return 'text-zinc-500'
}
</script>

<template>
  <div class="bg-white dark:bg-zinc-900 shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-5">
    <div class="flex items-center justify-between gap-3 mb-3">
      <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider">Sentiment trend</p>
      <SegmentedControl
        :model-value="activeDays"
        :options="DAYS_OPTIONS.map((d) => ({ value: d, label: `${d}d` }))"
        label="Sentiment window"
        @update:model-value="(d) => emit('update:days', d as number)"
      />
    </div>

    <div v-if="series.length === 0" class="h-32 flex items-center justify-center text-xs text-zinc-400">
      No sentiment data yet
    </div>

    <template v-else>
      <svg
        viewBox="0 0 320 96"
        class="w-full h-24"
        role="img"
        :aria-label="`Sentiment score trend over the last ${activeDays} days`"
      >
        <defs>
          <linearGradient id="sentiment-score-stroke" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stop-color="#22d3ee" />
            <stop offset="100%" stop-color="#06b6d4" />
          </linearGradient>
          <linearGradient id="sentiment-score-fill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#22d3ee" stop-opacity="0.18" />
            <stop offset="100%" stop-color="#22d3ee" stop-opacity="0" />
          </linearGradient>
        </defs>
        <line
          v-if="zeroY !== null"
          :y1="zeroY"
          :y2="zeroY"
          x1="6"
          x2="314"
          class="stroke-zinc-200 dark:stroke-zinc-700"
          stroke-dasharray="3 3"
          stroke-width="1"
        />
        <polygon :points="areaPoints" fill="url(#sentiment-score-fill)" />
        <polyline
          :points="points"
          fill="none"
          stroke="url(#sentiment-score-stroke)"
          stroke-width="2"
          stroke-linejoin="round"
          stroke-linecap="round"
        />
      </svg>

      <div class="mt-3">
        <div class="flex items-center gap-2 text-[10px] font-medium text-zinc-400 uppercase tracking-wider px-1 pb-1">
          <span class="w-24">Date</span>
          <span class="w-8 text-right">Neg</span>
          <span class="w-8 text-right">Neu</span>
          <span class="w-8 text-right">Pos</span>
          <span class="ml-auto">Score</span>
        </div>
        <div
          v-for="row in visibleRows"
          :key="row.date"
          class="flex items-center gap-2 px-1 py-1 rounded-md text-xs odd:bg-zinc-50/60 dark:odd:bg-zinc-800/40"
        >
          <span class="w-24 text-zinc-600 dark:text-zinc-400 font-mono tabular-nums">{{ row.date }}</span>
          <span class="w-8 text-right text-rose-600 font-mono tabular-nums">{{ row.negative }}</span>
          <span class="w-8 text-right text-zinc-500 font-mono tabular-nums">{{ row.neutral }}</span>
          <span class="w-8 text-right text-emerald-600 font-mono tabular-nums">{{ row.positive }}</span>
          <span class="ml-auto font-medium font-mono tabular-nums" :class="scoreColor(row.score)">
            {{ row.score > 0 ? `+${row.score}` : row.score }}
          </span>
        </div>
        <p v-if="series.length > visibleRows.length" class="text-[10px] text-zinc-400 mt-1 px-1">
          Showing last {{ visibleRows.length }} of {{ series.length }} days
        </p>
      </div>

      <div class="mt-3 pt-3 border-t border-zinc-100 dark:border-zinc-800 flex items-center gap-3 text-xs text-zinc-500 dark:text-zinc-400">
        <span><span class="font-semibold text-rose-600 font-mono tabular-nums">{{ totals.negative }}</span> negative</span>
        <span><span class="font-semibold text-zinc-600 dark:text-zinc-300 font-mono tabular-nums">{{ totals.neutral }}</span> neutral</span>
        <span><span class="font-semibold text-emerald-600 font-mono tabular-nums">{{ totals.positive }}</span> positive</span>
        <span class="ml-auto">Net score <span class="font-semibold font-mono tabular-nums" :class="scoreColor(totals.score)">{{ totals.score }}</span></span>
      </div>
    </template>
  </div>
</template>