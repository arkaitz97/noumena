<script setup lang="ts">
import { computed, ref } from 'vue'
import { Star } from 'lucide-vue-next'
import type { Comment, Insight } from '../services/types'
import { categoryColor, categoryLabel } from '../lib/categories'
import { relative } from '../lib/relativeTime'

const props = defineProps<{
  comment: Comment
  isStarred?: boolean
  isFocused?: boolean
}>()

const emit = defineEmits<{ (e: 'toggle-star', id: string, next: boolean): void }>()

const TRUNCATE = 280
const expanded = ref(false)
const insightsExpanded = ref(false)

const isStarred = computed(() => props.isStarred ?? props.comment.is_starred)
const body = computed(() => props.comment.body || '')
const truncated = computed(() => body.value.length > TRUNCATE)
const visibleBody = computed(() =>
  truncated.value && !expanded.value ? body.value.slice(0, TRUNCATE).trimEnd() + '…' : body.value,
)

const insights = computed<Insight[]>(() => props.comment.actionable_insights ?? [])
const hasInsights = computed(() => insights.value.length > 0)

const INSIGHT_TYPE_COLORS: Record<string, string> = {
  video_idea: 'bg-blue-50 text-blue-700 ring-blue-600/20',
  editing_tip: 'bg-amber-50 text-amber-700 ring-amber-600/20',
  feature_request: 'bg-purple-50 text-purple-700 ring-purple-600/20',
  collaboration_ask: 'bg-green-50 text-green-700 ring-green-600/20',
  improvement_suggestion: 'bg-orange-50 text-orange-700 ring-orange-600/20',
  content_format_request: 'bg-pink-50 text-pink-700 ring-pink-600/20',
  other: 'bg-zinc-50 text-zinc-700 ring-zinc-600/20',
}

const INSIGHT_TYPE_LABELS: Record<string, string> = {
  video_idea: 'Video Idea',
  editing_tip: 'Editing Tip',
  feature_request: 'Feature Request',
  collaboration_ask: 'Collaboration Ask',
  improvement_suggestion: 'Improvement',
  content_format_request: 'Format Request',
  other: 'Other',
}

const author = computed(() =>
  props.comment.author_handle || props.comment.author_display_name || 'anonymous',
)
const channelLabel = computed(() => props.comment.channel_display_name || 'Channel')
const postedAt = computed(() => relative(props.comment.posted_at))
const isProcessing = computed(() => props.comment.processed_at === null)

function onToggleStar(): void {
  emit('toggle-star', props.comment.id, !isStarred.value)
}
</script>

<template>
  <article
    class="bg-white dark:bg-zinc-900 shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-2xl p-5 hover:shadow-md transition-shadow duration-200 relative overflow-hidden"
    :class="isFocused ? 'ring-2 ring-indigo-500 dark:ring-indigo-400 shadow-md' : ''"
  >
    <div class="absolute left-0 top-0 bottom-0 w-1 bg-indigo-400" aria-hidden="true"></div>

    <div class="pl-3">
      <div class="flex items-start justify-between gap-2">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <p class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 truncate">{{ author }}</p>
          <span class="text-zinc-300 dark:text-zinc-600">•</span>
          <span class="text-xs font-medium text-zinc-500 dark:text-zinc-400 truncate">{{ channelLabel }}</span>
          <span v-if="postedAt" class="text-zinc-300 dark:text-zinc-600">•</span>
          <p v-if="postedAt" class="text-xs text-zinc-500 dark:text-zinc-400">{{ postedAt }}</p>
          <span
            v-if="isProcessing"
            class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-600/20"
          >
            Processing…
          </span>
        </div>
        <button
          type="button"
          :aria-pressed="isStarred"
          :aria-label="isStarred ? 'Unstar comment' : 'Star comment'"
          @click="onToggleStar"
          class="p-1.5 rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-zinc-900 dark:focus:ring-zinc-100"
          :class="isStarred ? 'text-amber-400 hover:bg-amber-50 dark:hover:bg-amber-950/30' : 'text-zinc-400 hover:text-amber-400 hover:bg-zinc-50 dark:hover:bg-zinc-800'"
        >
          <Star class="h-5 w-5" :fill="isStarred ? 'currentColor' : 'none'" />
        </button>
      </div>

      <p class="text-[15px] text-zinc-800 dark:text-zinc-200 leading-relaxed mt-2 mb-3 break-words whitespace-pre-wrap">
        {{ visibleBody }}
        <button
          v-if="truncated"
          type="button"
          @click="expanded = !expanded"
          class="ml-1 text-xs font-medium text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 underline-offset-2 hover:underline focus:outline-none focus:ring-2 focus:ring-zinc-900 dark:focus:ring-zinc-100 rounded"
        >
          {{ expanded ? 'Show less' : 'Show more' }}
        </button>
      </p>

      <div class="flex flex-wrap items-center gap-2">
        <span
          v-if="comment.category"
          :class="[
            'inline-flex items-center px-2.5 py-1 rounded-md text-xs font-semibold tracking-wide ring-1 ring-inset',
            categoryColor(comment.category),
          ]"
        >
          {{ categoryLabel(comment.category) }}
        </span>
        <span
          v-if="comment.topic"
          class="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium bg-zinc-50 text-zinc-700 ring-1 ring-inset ring-zinc-500/10"
        >
          {{ comment.topic }}
        </span>
        <span
          v-if="hasInsights"
          class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium bg-indigo-50 text-indigo-700 ring-1 ring-inset ring-indigo-600/20 cursor-pointer hover:bg-indigo-100 transition-colors"
          @click="insightsExpanded = !insightsExpanded"
        >
          {{ insights.length }} insight{{ insights.length > 1 ? 's' : '' }}
        </span>
      </div>

      <div v-if="hasInsights && insightsExpanded" class="mt-3 space-y-2 border-t border-zinc-100 dark:border-zinc-800 pt-3">
        <div
          v-for="(insight, i) in insights"
          :key="i"
          class="flex flex-col gap-1"
        >
          <div class="flex items-center gap-2">
            <span
              :class="[
                'inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium ring-1 ring-inset',
                INSIGHT_TYPE_COLORS[insight.type] ?? INSIGHT_TYPE_COLORS.other,
              ]"
            >
              {{ INSIGHT_TYPE_LABELS[insight.type] ?? insight.type }}
            </span>
            <span class="text-[10px] text-zinc-400 tabular-nums">{{ Math.round(insight.confidence * 100) }}%</span>
          </div>
          <p class="text-xs text-zinc-700">{{ insight.description }}</p>
        </div>
      </div>
    </div>
  </article>
</template>
