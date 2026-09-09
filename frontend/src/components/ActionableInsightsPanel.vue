<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { InsightAggregatesResponse, WindowKey } from '../services/types'
import { insightTypeLabel } from '../lib/insightTypes'
import Card from './Card.vue'
import SegmentedControl from './SegmentedControl.vue'

const props = defineProps<{
  data: InsightAggregatesResponse | null
  windowKey: WindowKey
  loading?: boolean
}>()

const emit = defineEmits<{ (e: 'update:windowKey', w: WindowKey): void }>()

const totalInsights = computed(() => props.data?.total_insights ?? 0)
const topInsights = computed(() => props.data?.top_insights ?? [])

function drillQuery(type: string): Record<string, string> {
  const q: Record<string, string> = { insight_type: type }
  // When the aggregation is scoped to a single channel, carry it through so
  // the feed opens filtered by that channel too.
  if (props.data?.channel_id) q.channels = props.data.channel_id
  return q
}
</script>

<template>
  <Card>
    <div class="flex items-center justify-between mb-3">
      <div>
        <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider">Actionable insights</p>
        <p class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mt-0.5">{{ totalInsights }} total</p>
      </div>
      <SegmentedControl
        :model-value="windowKey"
        :options="(['7d', '30d', '90d'] as WindowKey[]).map((w) => ({ value: w, label: w }))"
        label="Insights window"
        @update:model-value="(w) => emit('update:windowKey', w as WindowKey)"
      />
    </div>

    <div v-if="loading" class="space-y-2">
      <div class="h-3 rounded bg-zinc-100 dark:bg-zinc-800 animate-pulse"></div>
      <div class="h-3 rounded bg-zinc-100 dark:bg-zinc-800 w-2/3 animate-pulse"></div>
    </div>

    <div v-else-if="topInsights.length === 0" class="h-24 flex items-center justify-center text-xs text-zinc-500 dark:text-zinc-400">
      No insights yet
    </div>

    <ul v-else class="space-y-2">
      <li v-for="(insight, i) in topInsights.slice(0, 8)" :key="i">
        <RouterLink
          :to="{ name: 'dashboard', query: drillQuery(insight.type) }"
          class="flex items-start gap-2 text-xs rounded-lg -m-1 p-1 hover:bg-zinc-50 dark:hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-brand-500"
        >
          <span class="flex-shrink-0 w-5 h-5 rounded-full bg-brand-50 dark:bg-brand-950/40 text-brand-600 dark:text-brand-300 flex items-center justify-center text-[10px] font-semibold">
            {{ i + 1 }}
          </span>
          <span class="flex-1 min-w-0">
            <span class="block text-zinc-700 dark:text-zinc-300 truncate">{{ insight.description }}</span>
            <span class="block text-zinc-500 dark:text-zinc-400 text-[10px] mt-0.5">
              {{ insightTypeLabel(insight.type) }} · {{ insight.count }} mentions
            </span>
          </span>
        </RouterLink>
      </li>
    </ul>
  </Card>
</template>
