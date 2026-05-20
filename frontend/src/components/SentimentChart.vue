<script setup lang="ts">
import { computed } from 'vue'
import type { SentimentBucket, WindowKey } from '../services/types'

const props = defineProps<{
  series: SentimentBucket[]
  windowKey: WindowKey
  loading?: boolean
}>()

const emit = defineEmits<{ (e: 'update:windowKey', w: WindowKey): void }>()

const W = 600
const H = 160
const PAD = 24

const points = computed(() => {
  const s = props.series ?? []
  if (s.length === 0) return [] as Array<{ x: number; y: number; v: number; bucket: string }>
  const innerW = W - PAD * 2
  const innerH = H - PAD * 2
  return s.map((b, i) => {
    const x = PAD + (s.length === 1 ? innerW / 2 : (i / (s.length - 1)) * innerW)
    // map [-1, 1] → [H-PAD, PAD]
    const v = Math.max(-1, Math.min(1, b.avg ?? 0))
    const y = PAD + ((1 - (v + 1) / 2)) * innerH
    return { x, y, v, bucket: b.bucket }
  })
})

const polylinePoints = computed(() => points.value.map((p) => `${p.x},${p.y}`).join(' '))

const avg = computed(() => {
  const s = props.series ?? []
  if (s.length === 0) return 0
  const sum = s.reduce((acc, b) => acc + (b.avg ?? 0), 0)
  return sum / s.length
})

const lineColor = computed(() => {
  const a = avg.value
  if (a >= 0.3) return '#10b981' // emerald-500
  if (a <= -0.3) return '#f43f5e' // rose-500
  return '#71717a' // zinc-500
})

const ariaSummary = computed(() => {
  const a = avg.value
  return `Sentiment over ${props.windowKey}, average ${a.toFixed(2)}.`
})
</script>

<template>
  <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5">
    <div class="flex items-center justify-between mb-3">
      <div>
        <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider">Sentiment</p>
        <p class="text-sm font-semibold text-zinc-900 mt-0.5">avg {{ avg.toFixed(2) }}</p>
      </div>
      <div class="inline-flex rounded-lg bg-zinc-100 p-0.5" role="tablist" aria-label="Sentiment window">
        <button
          v-for="w in (['7d', '30d', '90d'] as WindowKey[])"
          :key="w"
          type="button"
          role="tab"
          :aria-selected="windowKey === w"
          @click="emit('update:windowKey', w)"
          :class="[
            'px-2.5 py-1 text-xs font-medium rounded-md transition-colors',
            windowKey === w ? 'bg-white text-zinc-900 shadow-sm' : 'text-zinc-500 hover:text-zinc-900',
          ]"
        >
          {{ w }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="h-40 rounded-xl bg-zinc-50 animate-pulse"></div>
    <div v-else-if="series.length === 0" class="h-40 rounded-xl bg-zinc-50 flex items-center justify-center text-xs text-zinc-400">
      No data yet
    </div>
    <svg
      v-else
      :viewBox="`0 0 ${W} ${H}`"
      class="w-full h-40"
      role="img"
      :aria-label="ariaSummary"
    >
      <line :x1="PAD" :x2="W - PAD" :y1="PAD + (H - PAD * 2) / 2" :y2="PAD + (H - PAD * 2) / 2" stroke="#e4e4e7" stroke-dasharray="4 4" />
      <polyline
        :points="polylinePoints"
        fill="none"
        :stroke="lineColor"
        stroke-width="2.5"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <circle
        v-for="(p, i) in points"
        :key="i"
        :cx="p.x"
        :cy="p.y"
        r="2.5"
        :fill="lineColor"
      />
    </svg>

    <table class="sr-only">
      <caption>Sentiment series for {{ windowKey }}</caption>
      <thead>
        <tr><th>Bucket</th><th>Average</th><th>Count</th></tr>
      </thead>
      <tbody>
        <tr v-for="b in series" :key="b.bucket">
          <td>{{ b.bucket }}</td>
          <td>{{ (b.avg ?? 0).toFixed(2) }}</td>
          <td>{{ b.count }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
