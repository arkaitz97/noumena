<script setup lang="ts">
import { computed, ref } from 'vue'
import { Star, Play } from 'lucide-vue-next'
import type { Comment, Insight } from '../services/types'
import { categoryColor, categoryLabel } from '../lib/categories'
import { insightTypeColor, insightTypeLabel } from '../lib/insightTypes'
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

const insightListId = computed(() => `comment-insights-${props.comment.id.replace(/[^a-zA-Z0-9_-]/g, '')}`)

const isStarred = computed(() => props.isStarred ?? props.comment.is_starred)
const body = computed(() => props.comment.body || '')
const truncated = computed(() => body.value.length > TRUNCATE)
const visibleBody = computed(() =>
  truncated.value && !expanded.value ? body.value.slice(0, TRUNCATE).trimEnd() + '…' : body.value,
)

const insights = computed<Insight[]>(() => props.comment.actionable_insights ?? [])
const hasInsights = computed(() => insights.value.length > 0)

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
    class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-5 hover:shadow-pop transition-shadow duration-200 relative overflow-hidden"
    :class="isFocused ? 'ring-2 ring-brand-500 dark:ring-brand-400 shadow-md' : ''"
  >
    <div>
      <div class="flex items-start justify-between gap-2">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <p class="text-sm font-semibold text-zinc-900 dark:text-white truncate">{{ author }}</p>
          <span class="text-zinc-300 dark:text-zinc-600">•</span>
          <a
            v-if="channelUrl"
            :href="channelUrl"
            target="_blank"
            rel="noopener"
            class="text-xs font-medium text-zinc-500 dark:text-zinc-400 truncate underline-offset-2 hover:underline hover:text-zinc-900 dark:hover:text-zinc-100"
          >
            {{ channelLabel }}
          </a>
          <span v-else class="text-xs font-medium text-zinc-500 dark:text-zinc-400 truncate">{{ channelLabel }}</span>
          <span v-if="videoUrl" class="text-zinc-300 dark:text-zinc-600">•</span>
          <a
            v-if="videoUrl"
            :href="videoUrl"
            target="_blank"
            rel="noopener"
            class="inline-flex items-center gap-1 text-xs font-medium text-zinc-500 dark:text-zinc-400 truncate underline-offset-2 hover:underline hover:text-zinc-900 dark:hover:text-zinc-100"
          >
            <Play class="w-3 h-3 shrink-0" aria-hidden="true" />
            <span class="truncate">{{ videoLabel }}</span>
          </a>
          <span v-if="postedAt" class="text-zinc-300 dark:text-zinc-600">•</span>
          <p v-if="postedAt" class="text-xs text-zinc-500 dark:text-zinc-400">{{ postedAt }}</p>
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
          class="flex items-center justify-center h-11 w-11 rounded-lg transition-all duration-200 active:scale-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
          :class="isStarred ? 'text-amber-400 hover:bg-amber-50 dark:hover:bg-amber-950/30' : 'text-zinc-400 hover:text-amber-400 hover:bg-zinc-100/80 dark:hover:bg-white/5'"
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
          class="ml-1 text-xs font-medium text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 underline-offset-2 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 rounded"
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
          class="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium bg-zinc-50 dark:bg-white/5 text-zinc-700 dark:text-zinc-300 ring-1 ring-inset ring-zinc-500/10 dark:ring-white/10"
        >
          {{ comment.topic }}
        </span>
        <button
          v-if="hasInsights"
          type="button"
          class="-m-2 p-2 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 rounded-lg"
          :aria-expanded="insightsExpanded"
          :aria-controls="insightListId"
          @click="insightsExpanded = !insightsExpanded"
        >
          <span
            class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium bg-brand-50 dark:bg-brand-950/40 text-brand-700 dark:text-brand-300 ring-1 ring-inset ring-brand-600/20 dark:ring-brand-500/30 hover:bg-brand-100 dark:hover:bg-brand-900/50 transition-colors"
          >
            {{ insights.length }} insight{{ insights.length > 1 ? 's' : '' }}
          </span>
        </button>
      </div>

      <div v-if="hasInsights && insightsExpanded" :id="insightListId" class="mt-3 space-y-2 border-t border-zinc-200/70 dark:border-white/5 pt-3">
        <div
          v-for="(insight, i) in insights"
          :key="i"
          class="flex flex-col gap-1"
        >
          <div class="flex items-center gap-2">
            <span
              :class="[
                'inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium ring-1 ring-inset',
                insightTypeColor(insight.type),
              ]"
            >
              {{ insightTypeLabel(insight.type) }}
            </span>
            <span class="text-[10px] text-zinc-500 dark:text-zinc-400 font-mono tabular-nums">{{ Math.round(insight.confidence * 100) }}%</span>
          </div>
          <p class="text-xs text-zinc-700 dark:text-zinc-300">{{ insight.description }}</p>
        </div>
      </div>
    </div>
  </article>
</template>
