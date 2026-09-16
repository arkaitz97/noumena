<script setup lang="ts">
import { computed, ref } from 'vue'
import { Star, Play } from 'lucide-vue-next'
import type { Comment, Insight } from '../services/types'
import { categoryColor, categoryLabel } from '../lib/categories'
import { insightTypeColor, insightTypeLabel } from '../lib/insightTypes'
import { relative } from '../lib/relativeTime'

const props = withDefaults(
  defineProps<{
    comment: Comment
    isStarred?: boolean
    isFocused?: boolean
    hideVideo?: boolean
  }>(),
  {
    isStarred: undefined,
    isFocused: false,
    hideVideo: false,
  },
)

const emit = defineEmits<{ (e: 'toggle-star', id: string, next: boolean): void }>()

const TRUNCATE = 280
const expanded = ref(false)
const insightsExpanded = ref(false)

const insightListId = computed(() => `comment-insights-${props.comment.id.replace(/[^a-zA-Z0-9_-]/g, '')}`)

const isStarred = computed(() => props.isStarred ?? props.comment.is_starred)
const body = computed(() => props.comment.body || '')
const truncated = computed(() => body.value.length > TRUNCATE)
const visibleBody = computed(() =>
  truncated.value && !expanded.value ? body.value.slice(0, TRUNCATE).trimEnd() + '…' : body.value,
)

const insights = computed<Insight[]>(() => props.comment.actionable_insights ?? [])
const primaryInsight = computed(() => [...insights.value].sort((a, b) => b.confidence - a.confidence)[0] ?? null)

const author = computed(() =>
  props.comment.author_handle || props.comment.author_display_name || 'anonymous',
)
const channelLabel = computed(() => props.comment.channel_name || props.comment.channel_display_name || 'Channel')
const channelUrl = computed(() => {
  const id = props.comment.platform_id || props.comment.platform_channel_id
  return id ? `https://www.youtube.com/channel/${id}` : null
})
const videoUrl = computed(() =>
  props.comment.video_id ? `https://www.youtube.com/watch?v=${props.comment.video_id}` : null,
)
const videoLabel = computed(
  () => props.comment.video_title || (props.comment.video_id ? 'Open video' : ''),
)
const postedAt = computed(() => relative(props.comment.posted_at))
const isProcessing = computed(() => props.comment.processed_at === null)

function onToggleStar(): void {
  emit('toggle-star', props.comment.id, !isStarred.value)
}
</script>

<template>
  <article
    data-comment-card
    class="@container bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-5 hover:shadow-pop transition-shadow duration-200 relative overflow-hidden"
    :class="[
      isFocused ? 'ring-2 ring-brand-500 dark:ring-brand-400 shadow-md' : '',
    ]"
  >
    <div>
      <div v-if="primaryInsight" class="mb-3 pb-3 border-b border-zinc-100 dark:border-white/5">
        <div class="flex items-center justify-between gap-2 mb-1.5">
          <div class="flex items-center gap-2 min-w-0">
            <span class="inline-flex rounded px-1.5 py-0.5 text-[10px] font-semibold ring-1 ring-inset" :class="insightTypeColor(primaryInsight.type)">
              {{ insightTypeLabel(primaryInsight.type) }}
            </span>
            <span class="text-[10px] tabular-nums text-zinc-400 dark:text-zinc-500">
              {{ Math.round(primaryInsight.confidence * 100) }}% confidence
            </span>
            <span v-if="insights.length > 0" class="text-[10px] text-zinc-400 dark:text-zinc-500">
              · {{ insights.length }} insight{{ insights.length > 1 ? 's' : '' }}
            </span>
          </div>
          <button
            v-if="insights.length > 1"
            type="button"
            class="text-[11px] font-medium text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200 focus:outline-none focus-visible:underline shrink-0"
            :aria-expanded="insightsExpanded"
            :aria-controls="insightListId"
            @click="insightsExpanded = !insightsExpanded"
          >
            {{ insightsExpanded ? 'Hide additional' : `+${insights.length - 1} more` }}
          </button>
        </div>
        <p class="text-sm font-semibold text-zinc-900 dark:text-white leading-snug">{{ primaryInsight.description }}</p>

        <div v-if="insightsExpanded && insights.length > 1" :id="insightListId" class="mt-2.5 space-y-2 pt-2 border-t border-zinc-100 dark:border-white/5">
          <div
            v-for="(insight, i) in insights.slice(1)"
            :key="i"
            class="space-y-0.5 text-xs text-zinc-600 dark:text-zinc-300"
          >
            <div class="flex items-center gap-2">
              <span class="inline-flex rounded px-1.5 py-0.5 text-[10px] font-medium ring-1 ring-inset" :class="insightTypeColor(insight.type)">
                {{ insightTypeLabel(insight.type) }}
              </span>
              <span class="text-[10px] text-zinc-400 tabular-nums">{{ Math.round(insight.confidence * 100) }}%</span>
            </div>
            <p>{{ insight.description }}</p>
          </div>
        </div>
      </div>

      <div class="flex items-start justify-between gap-2">
        <div class="flex flex-wrap items-center gap-x-2 gap-y-1 min-w-0">
          <p class="text-sm font-semibold text-zinc-900 dark:text-white truncate max-w-[140px] @xs:max-w-[200px] @sm:max-w-none">{{ author }}</p>
          <span class="text-zinc-300 dark:text-zinc-600 text-xs">•</span>
          <a
            v-if="channelUrl"
            :href="channelUrl"
            target="_blank"
            rel="noopener"
            class="text-xs font-medium text-zinc-500 dark:text-zinc-400 truncate underline-offset-2 hover:underline hover:text-zinc-900 dark:hover:text-zinc-100 max-w-[120px] @xs:max-w-[180px] @sm:max-w-none"
          >
            {{ channelLabel }}
          </a>
          <span v-else class="text-xs font-medium text-zinc-500 dark:text-zinc-400 truncate max-w-[120px] @xs:max-w-[180px] @sm:max-w-none">{{ channelLabel }}</span>
          <template v-if="videoUrl && !hideVideo">
            <span class="text-zinc-300 dark:text-zinc-600 text-xs">•</span>
            <a
              :href="videoUrl"
              target="_blank"
              rel="noopener"
              class="inline-flex items-center gap-1 text-xs font-medium text-zinc-500 dark:text-zinc-400 truncate underline-offset-2 hover:underline hover:text-zinc-900 dark:hover:text-zinc-100 max-w-[140px] @xs:max-w-[220px] @sm:max-w-none"
            >
              <Play class="w-3 h-3 shrink-0" aria-hidden="true" />
              <span class="truncate">{{ videoLabel }}</span>
            </a>
          </template>
          <template v-if="postedAt">
            <span class="text-zinc-300 dark:text-zinc-600 text-xs">•</span>
            <p class="text-xs text-zinc-500 dark:text-zinc-400">{{ postedAt }}</p>
          </template>
          <span
            v-if="isProcessing"
            class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-amber-50 dark:bg-amber-400/10 text-amber-700 dark:text-amber-300 ring-1 ring-inset ring-amber-600/20 dark:ring-amber-400/20"
          >
            Processing…
          </span>
        </div>
        <button
          type="button"
          :aria-pressed="isStarred"
          :aria-label="isStarred ? 'Unstar comment' : 'Star comment'"
          @click="onToggleStar"
          class="flex items-center justify-center h-9 w-9 rounded-lg transition-all duration-200 active:scale-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 shrink-0"
          :class="isStarred ? 'text-amber-500 hover:text-amber-600 hover:bg-amber-100/60 dark:text-amber-400 dark:hover:bg-amber-400/10' : 'text-zinc-400 hover:text-amber-500 hover:bg-zinc-100 dark:hover:bg-white/5'"
        >
          <Star class="h-4.5 w-4.5" :fill="isStarred ? 'currentColor' : 'none'" />
        </button>
      </div>

      <p class="text-[15px] text-zinc-800 dark:text-zinc-200 leading-relaxed mt-2 mb-3 break-words whitespace-pre-wrap">
        {{ visibleBody }}
        <button
          v-if="truncated"
          type="button"
          @click="expanded = !expanded"
          class="ml-1 text-xs font-medium text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 underline-offset-2 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 rounded"
        >
          {{ expanded ? 'Show less' : 'Show more' }}
        </button>
      </p>

      <div v-if="comment.category || comment.topic" class="flex flex-wrap items-center gap-1.5">
        <span
          v-if="comment.category"
          :class="[
            'inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium ring-1 ring-inset',
            categoryColor(comment.category),
          ]"
        >
          {{ categoryLabel(comment.category) }}
        </span>
        <span
          v-if="comment.topic"
          class="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium bg-zinc-50 dark:bg-white/5 text-zinc-600 dark:text-zinc-300 ring-1 ring-inset ring-zinc-500/10 dark:ring-white/10 truncate max-w-[140px] @xs:max-w-[200px] @sm:max-w-none"
        >
          {{ comment.topic }}
        </span>
      </div>
    </div>
  </article>
</template>
