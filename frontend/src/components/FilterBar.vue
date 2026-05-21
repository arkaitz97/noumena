<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Search, X, Download, RefreshCw } from 'lucide-vue-next'
import type { Channel, FeedFilters, InsightType } from '../services/types'
import {
  CATEGORY_COLORS,
  CATEGORY_LABELS,
  HIGH_SIGNAL_CATEGORIES,
  TOXIC_CATEGORIES,
  type Category,
} from '../lib/categories'

const INSIGHT_TYPE_LABELS: Record<InsightType, string> = {
  video_idea: 'Video Ideas',
  editing_tip: 'Editing Tips',
  feature_request: 'Feature Requests',
  collaboration_ask: 'Collaboration Asks',
  improvement_suggestion: 'Improvements',
  content_format_request: 'Format Requests',
  other: 'Other',
}

const INSIGHT_TYPES: InsightType[] = [
  'video_idea',
  'editing_tip',
  'feature_request',
  'collaboration_ask',
  'improvement_suggestion',
  'content_format_request',
  'other',
]

const props = defineProps<{
  filters: FeedFilters
  channels: Channel[]
  variant?: 'feed' | 'saved'
}>()

const emit = defineEmits<{
  (e: 'update:filters', partial: Partial<FeedFilters>): void
  (e: 'export'): void
  (e: 'reset'): void
}>()

const variant = computed(() => props.variant ?? 'feed')
const isFeed = computed(() => variant.value === 'feed')

const searchInput = ref<string>(props.filters.search)
let searchTimer: number | null = null
watch(
  () => props.filters.search,
  (v) => {
    if (v !== searchInput.value) searchInput.value = v
  },
)
function onSearchInput(e: Event): void {
  searchInput.value = (e.target as HTMLInputElement).value
  if (searchTimer !== null) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    emit('update:filters', { search: searchInput.value })
  }, 300)
}

function toggleChannel(id: string): void {
  const set = new Set(props.filters.channels)
  if (set.has(id)) set.delete(id)
  else set.add(id)
  emit('update:filters', { channels: Array.from(set) })
}

const visibleCategories = computed<Category[]>(() => {
  if (!isFeed.value) return HIGH_SIGNAL_CATEGORIES
  return props.filters.include_toxic
    ? [...HIGH_SIGNAL_CATEGORIES, ...TOXIC_CATEGORIES]
    : HIGH_SIGNAL_CATEGORIES
})
function toggleCategory(c: Category): void {
  const set = new Set(props.filters.categories)
  if (set.has(c)) set.delete(c)
  else set.add(c)
  emit('update:filters', { categories: Array.from(set) })
}
function isCategoryActive(c: Category): boolean {
  return props.filters.categories.includes(c)
}

function toggleIncludeToxic(): void {
  const next = !props.filters.include_toxic
  const partial: Partial<FeedFilters> = { include_toxic: next }
  if (!next) {
    partial.categories = props.filters.categories.filter(
      (c) => !TOXIC_CATEGORIES.includes(c),
    )
  }
  emit('update:filters', partial)
}

function setInsightType(t: InsightType | null): void {
  emit('update:filters', { insight_type: t })
}

type DatePreset = '7d' | '30d' | '90d' | 'custom' | null
const datePreset = computed<DatePreset>(() => {
  if (!props.filters.date_from && !props.filters.date_to) return null
  return 'custom'
})
function setDatePreset(p: DatePreset): void {
  const today = new Date()
  const iso = (d: Date) => d.toISOString().slice(0, 10)
  if (p === '7d' || p === '30d' || p === '90d') {
    const days = p === '7d' ? 7 : p === '30d' ? 30 : 90
    const from = new Date(today)
    from.setDate(from.getDate() - days)
    emit('update:filters', { date_from: iso(from), date_to: iso(today) })
  } else if (p === 'custom') {
    emit('update:filters', {
      date_from: props.filters.date_from ?? iso(new Date(today.getTime() - 30 * 86400000)),
      date_to: props.filters.date_to ?? iso(today),
    })
  } else {
    emit('update:filters', { date_from: null, date_to: null })
  }
}

const showCustom = ref(false)
watch(
  () => datePreset.value,
  (p) => {
    if (p === 'custom') showCustom.value = true
  },
  { immediate: true },
)

const exportOpen = ref(false)
function toggleExportMenu(): void {
  exportOpen.value = !exportOpen.value
}
function doReset(): void {
  emit('reset')
}
</script>

<template>
  <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-4 sm:p-5 space-y-4">
    <div class="flex flex-col sm:flex-row gap-3">
      <div class="relative flex-1">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-400" />
        <input
          type="search"
          :value="searchInput"
          @input="onSearchInput"
          placeholder="Search comments…"
          aria-label="Search comments"
          class="appearance-none block w-full pl-9 pr-4 py-2.5 border border-zinc-200 rounded-xl shadow-sm placeholder-zinc-400 text-sm focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 transition-colors bg-zinc-50/50 focus:bg-white text-zinc-900"
        />
      </div>
      <div class="flex items-center gap-2">
        <button
          type="button"
          @click="doReset"
          class="inline-flex items-center gap-1.5 px-3 py-2.5 rounded-xl text-sm font-medium text-zinc-600 border border-zinc-200 hover:bg-zinc-50 transition-colors focus:outline-none focus:ring-2 focus:ring-zinc-900"
          aria-label="Reset filters"
        >
          <RefreshCw class="w-4 h-4" /> Reset
        </button>
        <div v-if="isFeed" class="relative">
          <button
            type="button"
            @click="toggleExportMenu"
            :aria-expanded="exportOpen"
            class="inline-flex items-center gap-1.5 px-3 py-2.5 rounded-xl text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-800 transition-colors focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:ring-offset-2"
          >
            <Download class="w-4 h-4" /> Export
          </button>
          <div
            v-if="exportOpen"
            class="absolute right-0 mt-2 w-40 bg-white shadow-lg ring-1 ring-zinc-950/5 rounded-xl py-1 z-20"
            role="menu"
          >
            <button
              type="button"
              role="menuitem"
              @click="exportOpen = false; emit('export')"
              class="w-full text-left px-3 py-2 text-sm text-zinc-700 hover:bg-zinc-50"
            >
              Open export…
            </button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="channels.length > 0">
      <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">Channels</p>
      <div class="flex flex-wrap gap-1.5">
        <button
          v-for="c in channels"
          :key="c.id"
          type="button"
          @click="toggleChannel(c.id)"
          :aria-pressed="filters.channels.includes(c.id)"
          :class="[
            'inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium ring-1 ring-inset transition-colors',
            filters.channels.includes(c.id)
              ? 'bg-zinc-900 text-white ring-zinc-900'
              : 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-50',
          ]"
        >
          {{ c.display_name || c.platform_channel_id }}
        </button>
      </div>
    </div>

    <div>
      <div class="flex items-center justify-between mb-2">
        <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider">Categories</p>
        <label v-if="isFeed" class="inline-flex items-center gap-1.5 text-xs text-zinc-600 cursor-pointer">
          <input
            type="checkbox"
            :checked="filters.include_toxic"
            @change="toggleIncludeToxic"
            class="h-3.5 w-3.5 rounded border-zinc-300 text-zinc-900 focus:ring-zinc-900"
          />
          Include toxic / spam
        </label>
      </div>
      <div class="flex flex-wrap gap-1.5">
        <button
          v-for="c in visibleCategories"
          :key="c"
          type="button"
          @click="toggleCategory(c)"
          :aria-pressed="isCategoryActive(c)"
          :class="[
            'inline-flex items-center px-2.5 py-1 rounded-md text-xs font-semibold tracking-wide ring-1 ring-inset transition-colors',
            isCategoryActive(c) ? CATEGORY_COLORS[c] : 'bg-white text-zinc-500 ring-zinc-200 hover:bg-zinc-50',
          ]"
        >
          {{ CATEGORY_LABELS[c] }}
        </button>
      </div>
    </div>

    <div v-if="isFeed">
      <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">Insight type</p>
      <div class="flex flex-wrap gap-1.5">
        <button
          type="button"
          @click="setInsightType(null)"
          :aria-pressed="filters.insight_type === null"
          :class="[
            'inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium ring-1 ring-inset transition-colors',
            filters.insight_type === null
              ? 'bg-zinc-900 text-white ring-zinc-900'
              : 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-50',
          ]"
        >
          All
        </button>
        <button
          v-for="t in INSIGHT_TYPES"
          :key="t"
          type="button"
          @click="setInsightType(t)"
          :aria-pressed="filters.insight_type === t"
          :class="[
            'inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium ring-1 ring-inset transition-colors',
            filters.insight_type === t
              ? 'bg-indigo-600 text-white ring-indigo-600'
              : 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-50',
          ]"
        >
          {{ INSIGHT_TYPE_LABELS[t] }}
        </button>
      </div>
    </div>

    <div>
      <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">Date range</p>
      <div class="flex flex-wrap items-center gap-1.5">
        <button
          v-for="p in (['7d', '30d', '90d', 'custom'] as DatePreset[])"
          :key="p ?? 'all'"
          type="button"
          @click="setDatePreset(p)"
          :aria-pressed="datePreset === p"
          :class="[
            'inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium ring-1 ring-inset transition-colors',
            datePreset === p
              ? 'bg-zinc-900 text-white ring-zinc-900'
              : 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-50',
          ]"
        >
          {{ p === 'custom' ? 'Custom' : p?.toUpperCase() }}
        </button>
        <button
          v-if="filters.date_from || filters.date_to"
          type="button"
          @click="setDatePreset(null)"
          class="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs text-zinc-500 hover:text-zinc-900 hover:bg-zinc-50 transition-colors"
        >
          <X class="w-3 h-3" /> clear
        </button>
      </div>
      <div v-if="datePreset === 'custom'" class="mt-2 flex flex-wrap items-center gap-2">
        <input
          type="date"
          :value="filters.date_from ?? ''"
          @change="emit('update:filters', { date_from: ($event.target as HTMLInputElement).value || null })"
          aria-label="Date from"
          class="appearance-none px-3 py-2 text-sm border border-zinc-200 rounded-xl bg-zinc-50/50 focus:bg-white focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900"
        />
        <span class="text-xs text-zinc-400">to</span>
        <input
          type="date"
          :value="filters.date_to ?? ''"
          @change="emit('update:filters', { date_to: ($event.target as HTMLInputElement).value || null })"
          aria-label="Date to"
          class="appearance-none px-3 py-2 text-sm border border-zinc-200 rounded-xl bg-zinc-50/50 focus:bg-white focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900"
        />
      </div>
    </div>
  </div>
</template>
