<script setup lang="ts">
import { computed } from 'vue'
import type { InsightDistributionItem, InsightType } from '../services/types'

const props = defineProps<{
  distribution: InsightDistributionItem[]
}>()

const INSIGHT_TYPE_COLORS: Record<InsightType, string> = {
  video_idea: '#93b4e8',
  editing_tip: '#e8c47a',
  feature_request: '#7cc3da',
  collaboration_ask: '#8fd0a8',
  improvement_suggestion: '#f0a878',
  content_format_request: '#f0a6c8',
  other: '#a3a6bb',
}

const INSIGHT_TYPE_LABELS: Record<InsightType, string> = {
  video_idea: 'Video Ideas',
  editing_tip: 'Editing Tips',
  feature_request: 'Feature Requests',
  collaboration_ask: 'Collaboration Asks',
  improvement_suggestion: 'Improvements',
  content_format_request: 'Format Requests',
  other: 'Other',
}

const total = computed(() => props.distribution.reduce((sum, d) => sum + d.count, 0))

const sorted = computed(() => [...props.distribution].sort((a, b) => b.count - a.count))

const maxCount = computed(() => {
  if (sorted.value.length === 0) return 1
  return Math.max(...sorted.value.map(d => d.count))
})
</script>

<template>
  <div class="@container bg-white dark:bg-zinc-900 shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-4 @sm:p-5">
    <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-3">Insight distribution</p>

    <div v-if="sorted.length === 0" class="h-32 flex items-center justify-center text-xs text-zinc-400">
      No data yet
    </div>

    <div v-else class="space-y-2">
      <div
        v-for="item in sorted"
        :key="item.type"
        class="flex items-center gap-2 @xs:gap-2.5 @sm:gap-3"
      >
        <span class="text-[11px] @xs:text-xs text-zinc-600 dark:text-zinc-400 w-24 @xs:w-28 @sm:w-32 truncate text-right">{{ INSIGHT_TYPE_LABELS[item.type] }}</span>
        <div class="flex-1 h-4 bg-zinc-100 dark:bg-zinc-800 rounded-full overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-300"
            :style="{
              width: `${(item.count / maxCount) * 100}%`,
              backgroundColor: INSIGHT_TYPE_COLORS[item.type],
            }"
          ></div>
        </div>
        <span class="text-[11px] @xs:text-xs font-medium text-zinc-500 font-mono tabular-nums w-8 @xs:w-10 text-right">{{ item.count }}</span>
      </div>
    </div>

    <p v-if="total > 0" class="text-[10px] text-zinc-400 mt-3 text-right">{{ total }} total insights</p>
  </div>
</template>
