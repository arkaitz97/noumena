<script setup lang="ts">
import { computed, ref } from 'vue'
import { Star } from 'lucide-vue-next'
import type { Comment } from '../services/types'
import { categoryColor, categoryLabel } from '../lib/categories'
import { relative } from '../lib/relativeTime'

const props = defineProps<{
  comment: Comment
  isStarred?: boolean
}>()

const emit = defineEmits<{ (e: 'toggle-star', id: string, next: boolean): void }>()

const TRUNCATE = 280
const expanded = ref(false)

const isStarred = computed(() => props.isStarred ?? props.comment.is_starred)
const body = computed(() => props.comment.body || '')
const truncated = computed(() => body.value.length > TRUNCATE)
const visibleBody = computed(() =>
  truncated.value && !expanded.value ? body.value.slice(0, TRUNCATE).trimEnd() + '…' : body.value,
)

const sentiment = computed(() => props.comment.sentiment_score ?? null)
const sentimentColor = computed(() => {
  const s = sentiment.value
  if (s === null) return 'bg-zinc-300'
  if (s >= 0.3) return 'bg-emerald-400'
  if (s <= -0.3) return 'bg-rose-400'
  return 'bg-zinc-300'
})
const sentimentBarPct = computed(() => {
  const s = sentiment.value ?? 0
  // map [-1, 1] → [0, 100]
  return Math.round(((s + 1) / 2) * 100)
})

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
    class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5 hover:shadow-md transition-shadow duration-200 relative overflow-hidden"
  >
    <div :class="['absolute left-0 top-0 bottom-0 w-1', sentimentColor]" aria-hidden="true"></div>

    <div class="pl-3">
      <div class="flex items-start justify-between gap-2">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <p class="text-sm font-semibold text-zinc-900 truncate">{{ author }}</p>
          <span class="text-zinc-300">•</span>
          <span class="text-xs font-medium text-zinc-500 truncate">{{ channelLabel }}</span>
          <span v-if="postedAt" class="text-zinc-300">•</span>
          <p v-if="postedAt" class="text-xs text-zinc-500">{{ postedAt }}</p>
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
          class="p-1.5 rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-zinc-900"
          :class="isStarred ? 'text-amber-400 hover:bg-amber-50' : 'text-zinc-400 hover:text-amber-400 hover:bg-zinc-50'"
        >
          <Star class="h-5 w-5" :fill="isStarred ? 'currentColor' : 'none'" />
        </button>
      </div>

      <p class="text-[15px] text-zinc-800 leading-relaxed mt-2 mb-3 break-words whitespace-pre-wrap">
        {{ visibleBody }}
        <button
          v-if="truncated"
          type="button"
          @click="expanded = !expanded"
          class="ml-1 text-xs font-medium text-zinc-500 hover:text-zinc-900 underline-offset-2 hover:underline focus:outline-none focus:ring-2 focus:ring-zinc-900 rounded"
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
        <div
          v-if="sentiment !== null"
          class="flex items-center gap-1.5"
          :title="`Sentiment ${sentiment.toFixed(2)}`"
        >
          <div class="relative w-20 h-1.5 rounded-full bg-zinc-100 overflow-hidden">
            <div
              :class="['absolute top-0 bottom-0 w-1 rounded-full', sentimentColor]"
              :style="{ left: `calc(${sentimentBarPct}% - 2px)` }"
            ></div>
          </div>
          <span class="text-[11px] font-medium text-zinc-500 tabular-nums">{{ sentiment.toFixed(2) }}</span>
        </div>
      </div>
    </div>
  </article>
</template>
