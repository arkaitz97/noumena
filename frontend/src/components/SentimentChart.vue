<script setup lang="ts">
// SentimentChart — per-day negative/neutral/positive counts + net score
// trend for one tracked channel. Spec 10-frontend-dashboard §4.3: props
// `data: SentimentData`, line chart, colour gradient based on sentiment.
// Display-only: the active window comes from `data.days`; the range
// selector lives at the view level (InsightsFeed) and re-fetches with a
// new `days` value. Rendered as plain SVG — no chart lib in
// package.json (mirrors InsightTypeDistributionChart's dependency-free
// approach).

import { computed } from 'vue'
import type { SentimentData } from '../services/insights'

const props = defineProps<{
  data: SentimentData | null
}>()

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
  <div class="@container bg-white dark:bg-zinc-900 shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-4 @sm:p-5">
    <div class="flex items-center justify-between gap-3 mb-3">
      <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider">Sentiment trend</p>
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
            <stop offset="0%" stop-color="#7cc3da" />
            <stop offset="100%" stop-color="#57acc8" />
          </linearGradient>
          <linearGradient id="sentiment-score-fill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#7cc3da" stop-opacity="0.18" />
            <stop offset="100%" stop-color="#7cc3da" stop-opacity="0" />
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
        <div class="grid grid-cols-[minmax(0,1fr)_1.5rem_1.5rem_1.5rem_2.5rem] @xs:grid-cols-[minmax(0,1fr)_1.75rem_1.75rem_1.75rem_2.75rem] @sm:grid-cols-[minmax(0,1fr)_2rem_2rem_2rem_3.25rem] gap-x-1 @xs:gap-x-1.5 @sm:gap-x-2 items-center text-[9px] @sm:text-[10px] font-medium text-zinc-400 uppercase tracking-wider px-1 pb-1">
          <span>Date</span>
          <span class="text-right">Neg</span>
          <span class="text-right">Neu</span>
          <span class="text-right">Pos</span>
          <span class="text-right">Score</span>
        </div>
        <div
          v-for="row in visibleRows"
          :key="row.date"
          class="grid grid-cols-[minmax(0,1fr)_1.5rem_1.5rem_1.5rem_2.5rem] @xs:grid-cols-[minmax(0,1fr)_1.75rem_1.75rem_1.75rem_2.75rem] @sm:grid-cols-[minmax(0,1fr)_2rem_2rem_2rem_3.25rem] gap-x-1 @xs:gap-x-1.5 @sm:gap-x-2 items-center px-1 py-1 rounded-md text-[11px] @sm:text-xs odd:bg-zinc-50/60 dark:odd:bg-zinc-800/40"
        >
          <span class="min-w-0 truncate text-zinc-600 dark:text-zinc-400 font-mono tabular-nums">{{ row.date }}</span>
          <span class="text-right text-rose-600 font-mono tabular-nums">{{ row.negative }}</span>
          <span class="text-right text-zinc-500 font-mono tabular-nums">{{ row.neutral }}</span>
          <span class="text-right text-emerald-600 font-mono tabular-nums">{{ row.positive }}</span>
          <span class="text-right font-medium font-mono tabular-nums" :class="scoreColor(row.score)">
            {{ row.score > 0 ? `+${row.score}` : row.score }}
          </span>
        </div>
        <p v-if="series.length > visibleRows.length" class="text-[10px] text-zinc-400 mt-1 px-1">
          Showing last {{ visibleRows.length }} of {{ series.length }} days
        </p>
      </div>

      <div class="mt-3 pt-3 border-t border-zinc-100 dark:border-zinc-800 flex flex-wrap items-center gap-x-2 @sm:gap-x-3 gap-y-1 text-[11px] @xs:text-xs text-zinc-500 dark:text-zinc-400">
        <span><span class="font-semibold text-rose-600 font-mono tabular-nums">{{ totals.negative }}</span> negative</span>
        <span><span class="font-semibold text-zinc-600 dark:text-zinc-300 font-mono tabular-nums">{{ totals.neutral }}</span> neutral</span>
        <span><span class="font-semibold text-emerald-600 font-mono tabular-nums">{{ totals.positive }}</span> positive</span>
        <span class="w-full @xs:w-auto @xs:ml-auto">Net score <span class="font-semibold font-mono tabular-nums" :class="scoreColor(totals.score)">{{ totals.score }}</span></span>
      </div>
    </template>
  </div>
</template>