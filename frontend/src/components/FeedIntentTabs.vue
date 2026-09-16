<script setup lang="ts">
import { INTENT_PRESETS, type FeedIntent } from '../lib/intentTabs'

defineProps<{ active: FeedIntent; counts?: Partial<Record<FeedIntent, number>> }>()
const emit = defineEmits<{ (event: 'select', intent: FeedIntent): void }>()

function onKeydown(e: KeyboardEvent, index: number): void {
  if (e.key === 'ArrowRight') {
    e.preventDefault()
    const next = (index + 1) % INTENT_PRESETS.length
    emit('select', INTENT_PRESETS[next].id)
  } else if (e.key === 'ArrowLeft') {
    e.preventDefault()
    const prev = (index - 1 + INTENT_PRESETS.length) % INTENT_PRESETS.length
    emit('select', INTENT_PRESETS[prev].id)
  }
}
</script>

<template>
  <nav aria-label="Feed intent" class="flex gap-1 overflow-x-auto" role="tablist">
    <button
      v-for="(preset, idx) in INTENT_PRESETS"
      :key="preset.id"
      type="button"
      role="tab"
      :tabindex="active === preset.id ? 0 : -1"
      :aria-selected="active === preset.id"
      :class="active === preset.id ? 'border-brand-500 text-brand-700 dark:text-brand-300' : 'border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'"
      class="inline-flex shrink-0 items-center gap-1.5 border-b-2 -mb-px px-3 py-2.5 text-xs font-semibold transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/70 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
      @click="emit('select', preset.id)"
      @keydown="onKeydown($event, idx)"
    >
      {{ preset.label }}
      <span
        v-if="counts?.[preset.id] !== undefined"
        class="rounded-full px-1.5 py-0.5 text-[10px] font-semibold tabular-nums transition-colors"
        :class="active === preset.id ? 'bg-brand-50 text-brand-700 dark:bg-brand-950/60 dark:text-brand-300' : 'bg-zinc-100 text-zinc-600 dark:bg-white/10 dark:text-zinc-400'"
      >
        {{ counts[preset.id] }}
      </span>
    </button>
  </nav>
</template>
