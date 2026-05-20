<script setup lang="ts">
import { ref } from 'vue'
import { ChevronDown, Sparkles } from 'lucide-vue-next'
import type { WeeklyDigest } from '../services/types'
import { shortDate } from '../lib/relativeTime'

defineProps<{ digest: WeeklyDigest | null; loading?: boolean }>()

const openIdeas = ref(true)
const openQuestions = ref(true)
const openTrend = ref(false)
const openHealth = ref(false)
</script>

<template>
  <section class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5">
    <header class="flex items-center gap-2 mb-3">
      <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-indigo-500 to-purple-500 flex items-center justify-center text-white">
        <Sparkles class="w-4 h-4" />
      </div>
      <div>
        <h3 class="text-sm font-semibold text-zinc-900">Weekly digest</h3>
        <p v-if="digest" class="text-[11px] text-zinc-500">
          {{ shortDate(digest.week_start) }} – {{ shortDate(digest.week_end) }}
        </p>
        <p v-else-if="!loading" class="text-[11px] text-zinc-500">
          Your first weekly digest will arrive next Monday.
        </p>
      </div>
    </header>

    <div v-if="loading" class="space-y-2">
      <div class="h-3 rounded bg-zinc-100 animate-pulse"></div>
      <div class="h-3 rounded bg-zinc-100 w-2/3 animate-pulse"></div>
    </div>

    <div v-else-if="!digest" class="text-xs text-zinc-500 leading-relaxed">
      Once you've collected a few days of comments, Noumena will summarise the
      week's top ideas, questions, and sentiment trend here every Monday.
    </div>

    <div v-else class="space-y-3 text-sm">
      <!-- Top ideas -->
      <div class="rounded-xl border border-zinc-100">
        <button
          type="button"
          @click="openIdeas = !openIdeas"
          :aria-expanded="openIdeas"
          class="w-full flex items-center justify-between px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-50 rounded-xl"
        >
          Top ideas ({{ digest.top_ideas?.length ?? 0 }})
          <ChevronDown class="w-4 h-4 transition-transform" :class="openIdeas ? 'rotate-180' : ''" />
        </button>
        <ul v-if="openIdeas" class="px-3 pb-3 space-y-1.5">
          <li v-for="(it, i) in (digest.top_ideas || [])" :key="i" class="text-xs text-zinc-700">
            <span class="font-medium text-zinc-900">{{ it.title }}</span>
            <span v-if="it.source_count" class="text-zinc-400"> · {{ it.source_count }}</span>
          </li>
          <li v-if="(digest.top_ideas || []).length === 0" class="text-xs text-zinc-400">No ideas this week.</li>
        </ul>
      </div>

      <!-- Top questions -->
      <div class="rounded-xl border border-zinc-100">
        <button
          type="button"
          @click="openQuestions = !openQuestions"
          :aria-expanded="openQuestions"
          class="w-full flex items-center justify-between px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-50 rounded-xl"
        >
          Top questions ({{ digest.top_questions?.length ?? 0 }})
          <ChevronDown class="w-4 h-4 transition-transform" :class="openQuestions ? 'rotate-180' : ''" />
        </button>
        <ul v-if="openQuestions" class="px-3 pb-3 space-y-1.5">
          <li v-for="(it, i) in (digest.top_questions || [])" :key="i" class="text-xs text-zinc-700">
            <span class="font-medium text-zinc-900">{{ it.title }}</span>
            <span v-if="it.source_count" class="text-zinc-400"> · {{ it.source_count }}</span>
          </li>
          <li v-if="(digest.top_questions || []).length === 0" class="text-xs text-zinc-400">No questions this week.</li>
        </ul>
      </div>

      <!-- Sentiment trend -->
      <div class="rounded-xl border border-zinc-100">
        <button
          type="button"
          @click="openTrend = !openTrend"
          :aria-expanded="openTrend"
          class="w-full flex items-center justify-between px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-50 rounded-xl"
        >
          Sentiment trend
          <ChevronDown class="w-4 h-4 transition-transform" :class="openTrend ? 'rotate-180' : ''" />
        </button>
        <div v-if="openTrend" class="px-3 pb-3 text-xs text-zinc-700">
          <p>7d avg: <span class="font-medium">{{ (digest.sentiment_trend?.avg_7d ?? 0).toFixed(2) }}</span></p>
          <p>30d avg: <span class="font-medium">{{ (digest.sentiment_trend?.avg_30d ?? 0).toFixed(2) }}</span></p>
        </div>
      </div>

      <!-- Health -->
      <div class="rounded-xl border border-zinc-100">
        <button
          type="button"
          @click="openHealth = !openHealth"
          :aria-expanded="openHealth"
          class="w-full flex items-center justify-between px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-50 rounded-xl"
        >
          Channel health
          <ChevronDown class="w-4 h-4 transition-transform" :class="openHealth ? 'rotate-180' : ''" />
        </button>
        <div v-if="openHealth" class="px-3 pb-3 text-xs text-zinc-700">
          <p v-if="digest.health_score !== null && digest.health_score !== undefined">
            Score: <span class="font-medium">{{ digest.health_score }}</span> / 100
          </p>
          <p v-else class="text-zinc-400">Not enough data yet.</p>
        </div>
      </div>
    </div>
  </section>
</template>
