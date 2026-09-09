<script setup lang="ts">
import { ref } from 'vue'
import { ChevronDown, Sparkles } from 'lucide-vue-next'
import type { WeeklyDigest } from '../services/types'
import { shortDate } from '../lib/relativeTime'
import { insightTypeLabel } from '../lib/insightTypes'
import Card from './Card.vue'

defineProps<{ digest: WeeklyDigest | null; loading?: boolean }>()

const openIdeas = ref(true)
const openQuestions = ref(true)
const openInsights = ref(false)
const openHealth = ref(false)
</script>

<template>
  <Card tag="section" class="p-5">
    <header class="flex items-center gap-2 mb-3">
      <div class="w-8 h-8 rounded-lg bg-brand-600 flex items-center justify-center text-white">
        <Sparkles class="w-4 h-4" />
      </div>
      <div>
        <h2 class="text-sm font-semibold text-zinc-900 dark:text-white">Weekly digest</h2>
        <p v-if="digest" class="text-[11px] text-zinc-500 dark:text-zinc-400">
          {{ shortDate(digest.week_start) }} – {{ shortDate(digest.week_end) }}
        </p>
        <p v-else-if="!loading" class="text-[11px] text-zinc-500 dark:text-zinc-400">
          Your first weekly digest will arrive next Monday.
        </p>
      </div>
    </header>

    <div v-if="loading" class="space-y-2">
      <div class="h-3 rounded bg-zinc-100 dark:bg-white/5 animate-pulse"></div>
      <div class="h-3 rounded bg-zinc-100 dark:bg-white/5 w-2/3 animate-pulse"></div>
    </div>

    <div v-else-if="!digest" class="text-xs text-zinc-500 dark:text-zinc-400 leading-relaxed">
      Once you've collected a few days of comments, Noumena will summarise the
      week's top ideas, questions, and actionable insights here every Monday.
    </div>

    <div v-else class="space-y-3 text-sm">
      <div class="rounded-xl border border-zinc-200/70 dark:border-white/5">
        <button
          type="button"
          @click="openIdeas = !openIdeas"
          :aria-expanded="openIdeas"
          class="w-full flex items-center justify-between px-3 py-2 text-left text-xs font-semibold text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100/80 dark:hover:bg-white/5 rounded-xl transition-all duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
        >
          Top ideas ({{ digest.top_ideas?.length ?? 0 }})
          <ChevronDown class="w-4 h-4 transition-transform" :class="openIdeas ? 'rotate-180' : ''" />
        </button>
        <ul v-if="openIdeas" class="px-3 pb-3 space-y-1.5">
          <li v-for="(it, i) in (digest.top_ideas || [])" :key="i" class="text-xs text-zinc-700 dark:text-zinc-300">
            <span class="font-medium text-zinc-900 dark:text-white">{{ it.title }}</span>
            <span v-if="it.source_count" class="text-zinc-500 dark:text-zinc-400"> · {{ it.source_count }}</span>
          </li>
          <li v-if="(digest.top_ideas || []).length === 0" class="text-xs text-zinc-500 dark:text-zinc-400">No ideas this week.</li>
        </ul>
      </div>

      <div class="rounded-xl border border-zinc-200/70 dark:border-white/5">
        <button
          type="button"
          @click="openQuestions = !openQuestions"
          :aria-expanded="openQuestions"
          class="w-full flex items-center justify-between px-3 py-2 text-left text-xs font-semibold text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100/80 dark:hover:bg-white/5 rounded-xl transition-all duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
        >
          Top questions ({{ digest.top_questions?.length ?? 0 }})
          <ChevronDown class="w-4 h-4 transition-transform" :class="openQuestions ? 'rotate-180' : ''" />
        </button>
        <ul v-if="openQuestions" class="px-3 pb-3 space-y-1.5">
          <li v-for="(it, i) in (digest.top_questions || [])" :key="i" class="text-xs text-zinc-700 dark:text-zinc-300">
            <span class="font-medium text-zinc-900 dark:text-white">{{ it.title }}</span>
            <span v-if="it.source_count" class="text-zinc-500 dark:text-zinc-400"> · {{ it.source_count }}</span>
          </li>
          <li v-if="(digest.top_questions || []).length === 0" class="text-xs text-zinc-500 dark:text-zinc-400">No questions this week.</li>
        </ul>
      </div>

      <div class="rounded-xl border border-zinc-200/70 dark:border-white/5">
        <button
          type="button"
          @click="openInsights = !openInsights"
          :aria-expanded="openInsights"
          class="w-full flex items-center justify-between px-3 py-2 text-left text-xs font-semibold text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100/80 dark:hover:bg-white/5 rounded-xl transition-all duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
        >
          Top actionable insights ({{ digest.top_actionable_insights?.length ?? 0 }})
          <ChevronDown class="w-4 h-4 transition-transform" :class="openInsights ? 'rotate-180' : ''" />
        </button>
        <div v-if="openInsights" class="px-3 pb-3 space-y-1.5">
          <div
            v-for="(insight, i) in (digest.top_actionable_insights || [])"
            :key="i"
            class="text-xs text-zinc-700 dark:text-zinc-300"
          >
            <span
              class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-brand-50 dark:bg-brand-950/40 text-brand-700 dark:text-brand-300 ring-1 ring-inset ring-brand-600/20 dark:ring-brand-500/30 mr-1"
            >
              {{ insightTypeLabel(insight.type) }}
            </span>
            <span class="font-medium text-zinc-900 dark:text-white">{{ insight.description }}</span>
            <span v-if="insight.count" class="text-zinc-500 dark:text-zinc-400"> · {{ insight.count }}</span>
          </div>
          <p v-if="(digest.top_actionable_insights || []).length === 0" class="text-xs text-zinc-500 dark:text-zinc-400">No insights this week.</p>
        </div>
      </div>

      <div class="rounded-xl border border-zinc-200/70 dark:border-white/5">
        <button
          type="button"
          @click="openHealth = !openHealth"
          :aria-expanded="openHealth"
          class="w-full flex items-center justify-between px-3 py-2 text-left text-xs font-semibold text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100/80 dark:hover:bg-white/5 rounded-xl transition-all duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
        >
          Channel health
          <ChevronDown class="w-4 h-4 transition-transform" :class="openHealth ? 'rotate-180' : ''" />
        </button>
        <div v-if="openHealth" class="px-3 pb-3 text-xs text-zinc-700 dark:text-zinc-300">
          <p v-if="digest.health_score !== null && digest.health_score !== undefined">
            Score: <span class="font-medium">{{ digest.health_score }}</span> / 100
          </p>
          <p v-else class="text-zinc-500 dark:text-zinc-400">Not enough data yet.</p>
        </div>
      </div>
    </div>
  </Card>
</template>