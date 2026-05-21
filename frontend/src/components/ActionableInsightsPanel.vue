<script setup lang="ts">
import { computed } from 'vue'
import type { InsightAggregatesResponse, WindowKey } from '../services/types'

const props = defineProps<{
  data: InsightAggregatesResponse | null
  windowKey: WindowKey
  loading?: boolean
}>()

const emit = defineEmits<{ (e: 'update:windowKey', w: WindowKey): void }>()

const totalInsights = computed(() => props.data?.total_insights ?? 0)
const topInsights = computed(() => props.data?.top_insights ?? [])
</script>

<template>
  <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5">
    <div class="flex items-center justify-between mb-3">
      <div>
        <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider">Actionable insights</p>
        <p class="text-sm font-semibold text-zinc-900 mt-0.5">{{ totalInsights }} total</p>
      </div>
      <div class="inline-flex rounded-lg bg-zinc-100 p-0.5" role="tablist" aria-label="Insights window">
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

    <div v-if="loading" class="space-y-2">
      <div class="h-3 rounded bg-zinc-100 animate-pulse"></div>
      <div class="h-3 rounded bg-zinc-100 w-2/3 animate-pulse"></div>
    </div>

    <div v-else-if="topInsights.length === 0" class="h-24 flex items-center justify-center text-xs text-zinc-400">
      No insights yet
    </div>

    <ul v-else class="space-y-2">
      <li
        v-for="(insight, i) in topInsights.slice(0, 8)"
        :key="i"
        class="flex items-start gap-2 text-xs"
      >
        <span class="flex-shrink-0 w-5 h-5 rounded-full bg-indigo-50 text-indigo-600 flex items-center justify-center text-[10px] font-semibold">
          {{ i + 1 }}
        </span>
        <div class="flex-1 min-w-0">
          <p class="text-zinc-700 truncate">{{ insight.description }}</p>
          <p class="text-zinc-400 text-[10px] mt-0.5">{{ insight.type }} · {{ insight.count }} mentions</p>
        </div>
      </li>
    </ul>
  </div>
</template>
