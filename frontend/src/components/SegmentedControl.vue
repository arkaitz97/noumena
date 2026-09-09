<script setup lang="ts">
// Shared segmented control (7d/30d/90d windows, billing interval, …).
// Centralizes the duplicated tablist markup that drifted across
// SentimentChart, ActionableInsightsPanel and PlanComparison: active-tab
// dark background, padding, and roving-tabindex keyboard behaviour now
// live in one place (same pattern as Card.vue).

import { onRovingTablistKeydown } from '../lib/rovingTabs'

export interface SegmentedOption {
  value: string | number
  label: string
}

const props = defineProps<{
  options: SegmentedOption[]
  modelValue: string | number
  label: string
}>()

const emit = defineEmits<{ (e: 'update:modelValue', value: string | number): void }>()
</script>

<template>
  <div
    class="inline-flex rounded-lg bg-zinc-100 dark:bg-zinc-800 p-1 gap-1 shadow-inner shadow-zinc-950/5 dark:shadow-black/20 ring-1 ring-zinc-950/5 dark:ring-white/5"
    role="tablist"
    :aria-label="label"
    @keydown="onRovingTablistKeydown"
  >
    <button
      v-for="opt in options"
      :key="String(opt.value)"
      type="button"
      role="tab"
      :aria-selected="modelValue === opt.value"
      :tabindex="modelValue === opt.value ? 0 : -1"
      @click="emit('update:modelValue', opt.value)"
      :class="[
        'px-3 py-1.5 text-xs font-semibold rounded-md transition-all duration-200 ease-snappy focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60',
        modelValue === opt.value
          ? 'bg-white dark:bg-zinc-600 text-zinc-900 dark:text-white shadow-card dark:shadow-none'
          : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-white/60 dark:hover:bg-zinc-700/40',
      ]"
    >
      {{ opt.label }}
    </button>
  </div>
</template>
