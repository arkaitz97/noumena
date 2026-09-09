<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Search, Calendar, Check, ChevronDown, Download, RefreshCw, SlidersHorizontal } from 'lucide-vue-next'
import type { Channel, FeedFilters, InsightType, WindowKey } from '../services/types'
import {
  CATEGORY_COLORS,
  CATEGORY_LABELS,
  HIGH_SIGNAL_CATEGORIES,
  TOXIC_CATEGORIES,
  type Category,
} from '../lib/categories'
import { INSIGHT_TYPES, INSIGHT_TYPE_LABELS } from '../lib/insightTypes'
import Card from './Card.vue'

const props = defineProps<{
  filters: FeedFilters
  channels: Channel[]
  variant?: 'feed' | 'saved'
  // When set (feed view), the 7d/30d/90d date presets also drive the
  // screen-wide insights range — one selector for every widget.
  range?: WindowKey
}>()

const emit = defineEmits<{
  (e: 'update:filters', partial: Partial<FeedFilters>): void
  (e: 'update:range', range: WindowKey): void
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

function setSort(sort: 'newest' | 'oldest'): void {
  emit('update:filters', { sort })
}

type DatePreset = '7d' | '30d' | '90d' | 'custom' | null
const datePreset = computed<DatePreset>(() => {
  if (!props.filters.date_from && !props.filters.date_to) return null
  return 'custom'
})
// With a bound `range` the preset highlight follows it (the insights
// range), since preset clicks also materialise date_from/date_to.
function isPresetActive(p: DatePreset): boolean {
  if (props.range) return p !== 'custom' && p !== null && props.range === p
  return datePreset.value === p
}
function setDatePreset(p: DatePreset): void {
  const today = new Date()
  // Local-timezone YYYY-MM-DD. `toISOString()` shifts by UTC offset and
  // produced an off-by-one day for `date_to`.
  const localIso = (d: Date) => {
    const m = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    return `${d.getFullYear()}-${m}-${day}`
  }
  if (p === '7d' || p === '30d' || p === '90d') {
    emit('update:range', p)
    const days = p === '7d' ? 7 : p === '30d' ? 30 : 90
    const from = new Date(today)
    from.setDate(from.getDate() - days)
    emit('update:filters', { date_from: localIso(from), date_to: localIso(today) })
  } else if (p === 'custom') {
    const from = new Date(today)
    from.setDate(from.getDate() - 30)
    emit('update:filters', {
      date_from: props.filters.date_from ?? localIso(from),
      date_to: props.filters.date_to ?? localIso(today),
    })
  } else {
    emit('update:filters', { date_from: null, date_to: null })
  }
}

// Stripe-style range picker: one trigger labelling the active range,
// presets picked from a popover.
const DATE_PRESETS: { key: Exclude<DatePreset, null | 'custom'>; label: string }[] = [
  { key: '7d', label: 'Last 7 days' },
  { key: '30d', label: 'Last 30 days' },
  { key: '90d', label: 'Last 90 days' },
]
const dateLabel = computed(() => {
  const active = DATE_PRESETS.find((p) => isPresetActive(p.key))
  if (active) return active.label
  if (datePreset.value === 'custom') return 'Custom range'
  return 'All time'
})

// Number of filter selections living behind the "Filters" popover that
// deviate from the defaults. Date presets and search stay on the main row,
// so they are not counted here.
const activeCount = computed(() => {
  const f = props.filters
  let n = f.channels.length
  const def = HIGH_SIGNAL_CATEGORIES
  n += f.categories.filter((c) => !def.includes(c)).length
  n += def.filter((c) => !f.categories.includes(c)).length
  if (isFeed.value) {
    if (f.insight_type) n += 1
    if (f.sort !== 'newest') n += 1
    if (f.include_toxic) n += 1
  }
  return n
})

const filtersOpen = ref(false)
const exportOpen = ref(false)
const dateOpen = ref(false)
const filtersRef = ref<HTMLElement | null>(null)
const exportRef = ref<HTMLElement | null>(null)
const dateRef = ref<HTMLElement | null>(null)

function toggleFiltersMenu(): void {
  filtersOpen.value = !filtersOpen.value
  if (filtersOpen.value) { exportOpen.value = false; dateOpen.value = false }
}
function toggleExportMenu(): void {
  exportOpen.value = !exportOpen.value
  if (exportOpen.value) { filtersOpen.value = false; dateOpen.value = false }
}
function toggleDateMenu(): void {
  dateOpen.value = !dateOpen.value
  if (dateOpen.value) { filtersOpen.value = false; exportOpen.value = false }
}
function pickDatePreset(p: DatePreset): void {
  setDatePreset(p)
  dateOpen.value = false
}
// Clicking a date input body (not just the tiny calendar glyph) opens the
// native picker via showPicker(). Safari lacks it; throw = no-op fallback.
function openDatePicker(e: Event): void {
  const el = e.target as HTMLInputElement
  try { el.showPicker?.() } catch { /* gesture denied or unsupported */ }
}
function closeMenus(): void {
  filtersOpen.value = false
  exportOpen.value = false
  dateOpen.value = false
}
function onDocPointerDown(e: PointerEvent): void {
  const t = e.target as Node
  if (filtersOpen.value && !filtersRef.value?.contains(t)) filtersOpen.value = false
  if (exportOpen.value && !exportRef.value?.contains(t)) exportOpen.value = false
  if (dateOpen.value && !dateRef.value?.contains(t)) dateOpen.value = false
}
function onDocKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') closeMenus()
}
onMounted(() => {
  document.addEventListener('pointerdown', onDocPointerDown)
  document.addEventListener('keydown', onDocKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointerDown)
  document.removeEventListener('keydown', onDocKeydown)
})

function doReset(): void {
  closeMenus()
  emit('reset')
}
</script>

<template>
  <Card padding="none" class="p-2.5 sm:p-3">
    <div class="flex flex-wrap items-center gap-2">
      <div class="relative min-w-0 flex-1 basis-48">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-400 dark:text-zinc-500" />
        <input
          type="search"
          :value="searchInput"
          @input="onSearchInput"
          placeholder="Search comments…"
          aria-label="Search comments"
          class="appearance-none block w-full pl-9 pr-4 py-2 border border-zinc-200 dark:border-white/10 rounded-xl shadow-sm placeholder-zinc-400 dark:placeholder-zinc-500 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500/30 focus:border-brand-500/70 transition-all duration-200 ease-snappy bg-zinc-50/50 focus:bg-white dark:bg-zinc-800 dark:focus:bg-zinc-800 text-zinc-900 dark:text-zinc-100"
        />
      </div>

      <div ref="dateRef" class="relative shrink-0" role="group" aria-label="Date range">
        <button
          type="button"
          @click="toggleDateMenu"
          :aria-expanded="dateOpen"
          class="inline-flex items-center gap-1.5 px-3 py-2 rounded-xl text-sm font-medium text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
          aria-label="Select date range"
        >
          <Calendar class="w-4 h-4 text-zinc-400 dark:text-zinc-500" />
          <span>{{ dateLabel }}</span>
          <ChevronDown class="w-3.5 h-3.5 opacity-60" />
        </button>
        <div
          v-if="dateOpen"
          class="absolute left-0 sm:left-auto sm:right-0 mt-2 w-72 sm:w-80 bg-white dark:bg-zinc-900 shadow-pop ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-1.5 z-30 origin-top-right"
          role="menu"
          aria-label="Date range options"
        >
          <button
            v-for="preset in DATE_PRESETS"
            :key="preset.key"
            type="button"
            role="menuitemradio"
            @click="pickDatePreset(preset.key)"
            :aria-pressed="isPresetActive(preset.key)"
            :class="[
              'relative w-full text-left px-3 py-2 rounded-lg text-sm transition-colors duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60',
              isPresetActive(preset.key)
                ? 'bg-zinc-100/80 dark:bg-white/5 text-zinc-900 dark:text-zinc-100 font-medium'
                : 'text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100/70 dark:hover:bg-white/5',
            ]"
          >
            <Check v-if="isPresetActive(preset.key)" class="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-brand-600" />
            {{ preset.label }}
          </button>
          <div class="my-1 border-t border-zinc-100 dark:border-white/5" />
          <button
            type="button"
            role="menuitemradio"
            @click="pickDatePreset(null)"
            :aria-pressed="!filters.date_from && !filters.date_to && !isPresetActive('7d') && !isPresetActive('30d') && !isPresetActive('90d')"
            :class="[
              'w-full text-left px-3 py-2 rounded-lg text-sm transition-colors duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60',
              (!filters.date_from && !filters.date_to && !isPresetActive('7d') && !isPresetActive('30d') && !isPresetActive('90d'))
                ? 'bg-zinc-100/80 dark:bg-white/5 text-zinc-900 dark:text-zinc-100 font-medium'
                : 'text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100/70 dark:hover:bg-white/5',
            ]"
          >
            All time
          </button>
          <div class="my-1 border-t border-zinc-100 dark:border-white/5" />
          <p class="px-3 pt-1 pb-1.5 text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider">Custom range</p>
          <div class="px-3 pb-2 flex flex-wrap items-center gap-2">
            <input
              type="date"
              :value="filters.date_from ?? ''"
              @change="emit('update:filters', { date_from: ($event.target as HTMLInputElement).value || null })"
              @click="openDatePicker"
              aria-label="Date from"
              class="min-w-[7.5rem] flex-1 appearance-none px-2 py-1.5 text-xs border border-zinc-200 dark:border-white/10 rounded-lg bg-zinc-50/50 focus:bg-white dark:bg-zinc-800 dark:focus:bg-zinc-800 dark:text-zinc-100 focus:outline-none focus:ring-2 focus:ring-brand-500/30 focus:border-brand-500/70"
            />
            <span class="text-xs text-zinc-400 dark:text-zinc-500">to</span>
            <input
              type="date"
              :value="filters.date_to ?? ''"
              @change="emit('update:filters', { date_to: ($event.target as HTMLInputElement).value || null })"
              @click="openDatePicker"
              aria-label="Date to"
              class="min-w-[7.5rem] flex-1 appearance-none px-2 py-1.5 text-xs border border-zinc-200 dark:border-white/10 rounded-lg bg-zinc-50/50 focus:bg-white dark:bg-zinc-800 dark:focus:bg-zinc-800 dark:text-zinc-100 focus:outline-none focus:ring-2 focus:ring-brand-500/30 focus:border-brand-500/70"
            />
          </div>
        </div>
      </div>

      <div ref="filtersRef" class="relative shrink-0">
        <button
          type="button"
          @click="toggleFiltersMenu"
          :aria-expanded="filtersOpen"
          class="inline-flex items-center gap-1.5 px-3 py-2 rounded-xl text-sm font-medium text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
          aria-label="Toggle filters"
        >
          <SlidersHorizontal class="w-4 h-4" />
          <span class="hidden sm:inline">Filters</span>
          <span
            v-if="activeCount > 0"
            class="inline-flex items-center justify-center min-w-5 h-5 px-1 rounded-full text-[11px] font-semibold bg-brand-600 text-white"
            aria-hidden="true"
          >{{ activeCount }}</span>
        </button>
        <div
          v-if="filtersOpen"
          class="fixed inset-x-3 bottom-3 max-h-[65vh] overflow-y-auto rounded-xl bg-white dark:bg-zinc-900 shadow-pop ring-1 ring-zinc-950/5 dark:ring-white/10 p-4 space-y-4 z-30 sm:absolute sm:inset-auto sm:right-0 sm:top-full sm:mt-2 sm:w-80 sm:max-h-none sm:overflow-visible sm:origin-top-right"
        >
          <div v-if="channels.length > 0">
            <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider mb-2">Channels</p>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="c in channels"
                :key="c.id"
                type="button"
                @click="toggleChannel(c.id)"
                :aria-pressed="filters.channels.includes(c.id)"
                :class="[
                  'inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium ring-1 ring-inset transition-all duration-200 ease-snappy',
                  filters.channels.includes(c.id)
                    ? 'bg-zinc-900 text-white ring-zinc-900 dark:bg-zinc-100 dark:text-zinc-900 dark:ring-zinc-100'
                    : 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-100/70 dark:bg-zinc-800 dark:text-zinc-300 dark:ring-white/10 dark:hover:bg-white/5',
                ]"
              >
                {{ c.display_name || c.platform_channel_id }}
              </button>
            </div>
          </div>

          <div>
            <div class="flex items-center justify-between mb-2">
              <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider">Categories</p>
              <label v-if="isFeed" class="inline-flex items-center gap-1.5 text-xs text-zinc-600 dark:text-zinc-300 cursor-pointer">
                <input
                  type="checkbox"
                  :checked="filters.include_toxic"
                  @change="toggleIncludeToxic"
                  class="h-3.5 w-3.5 rounded border-zinc-300 text-brand-600 dark:border-white/10 focus-visible:ring-2 focus-visible:ring-brand-500/60"
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
                  'inline-flex items-center px-2.5 py-1 rounded-md text-xs font-semibold tracking-wide ring-1 ring-inset transition-all duration-200 ease-snappy',
                  isCategoryActive(c) ? CATEGORY_COLORS[c] : 'bg-white text-zinc-500 ring-zinc-200 hover:bg-zinc-100/70 dark:bg-zinc-800 dark:text-zinc-400 dark:ring-white/10 dark:hover:bg-white/5',
                ]"
              >
                {{ CATEGORY_LABELS[c] }}
              </button>
            </div>
          </div>

          <div v-if="isFeed">
            <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider mb-2">Insight type</p>
            <div class="flex flex-wrap gap-1.5">
              <button
                type="button"
                @click="setInsightType(null)"
                :aria-pressed="filters.insight_type === null"
                :class="[
                  'inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium ring-1 ring-inset transition-all duration-200 ease-snappy',
                  filters.insight_type === null
                    ? 'bg-zinc-900 text-white ring-zinc-900 dark:bg-zinc-100 dark:text-zinc-900 dark:ring-zinc-100'
                    : 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-100/70 dark:bg-zinc-800 dark:text-zinc-300 dark:ring-white/10 dark:hover:bg-white/5',
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
                  'inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium ring-1 ring-inset transition-all duration-200 ease-snappy',
                  filters.insight_type === t
                    ? 'bg-brand-600 text-white ring-brand-600'
                    : 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-100/70 dark:bg-zinc-800 dark:text-zinc-300 dark:ring-white/10 dark:hover:bg-white/5',
                ]"
              >
                {{ INSIGHT_TYPE_LABELS[t] }}
              </button>
            </div>
          </div>

          <div v-if="isFeed">
            <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider mb-2">Sort order</p>
            <div class="inline-flex rounded-lg border border-zinc-200/70 dark:border-white/5 overflow-hidden" role="group" aria-label="Sort order">
              <button
                v-for="s in (['newest', 'oldest'] as const)"
                :key="s"
                type="button"
                @click="setSort(s)"
                :aria-pressed="filters.sort === s"
                class="px-3 py-1.5 text-xs font-medium transition-all duration-200 ease-snappy focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
                :class="filters.sort === s
                  ? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900'
                  : 'bg-white text-zinc-600 hover:bg-zinc-100/70 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-white/5'"
              >
                {{ s === 'newest' ? 'Newest' : 'Oldest' }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <button
        type="button"
        @click="doReset"
        class="inline-flex items-center justify-center w-9 h-9 shrink-0 rounded-xl text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 shadow-card dark:shadow-none"
        aria-label="Reset filters"
        title="Reset filters"
      >
        <RefreshCw class="w-4 h-4" />
      </button>

      <div v-if="isFeed" ref="exportRef" class="relative shrink-0">
        <button
          type="button"
          @click="toggleExportMenu"
          :aria-expanded="exportOpen"
          class="inline-flex items-center justify-center w-9 h-9 rounded-xl text-white bg-zinc-900 hover:bg-zinc-700 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200 shadow-card dark:shadow-none transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
          aria-label="Export"
          title="Export"
        >
          <Download class="w-4 h-4" />
        </button>
        <div
          v-if="exportOpen"
          class="absolute right-0 mt-2 w-40 bg-white dark:bg-zinc-900 shadow-pop ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl py-1 z-30 origin-top-right"
          role="menu"
        >
          <button
            type="button"
            role="menuitem"
            @click="exportOpen = false; emit('export')"
            class="w-full text-left px-3 py-2 text-sm text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100/70 dark:hover:bg-white/5 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
          >
            Open export…
          </button>
        </div>
      </div>
    </div>

  </Card>
</template>
