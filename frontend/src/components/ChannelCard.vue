<script setup lang="ts">
import { computed } from 'vue'
import { Pause, Play, Trash2, AlertTriangle, CircleAlert, RefreshCw } from 'lucide-vue-next'
import type { Channel, ChannelStatus } from '../services/types'
import { relative } from '../lib/relativeTime'
import Card from './Card.vue'

const props = defineProps<{
  channel: Channel
  status?: ChannelStatus | null
}>()

const emit = defineEmits<{
  (e: 'pause', id: string): void
  (e: 'resume', id: string): void
  (e: 'remove', id: string): void
  (e: 'update-backfill', id: string, days: number): void
  (e: 'reauthorize', id: string, popup: Window | null): void
}>()

const trackingStatus = computed(() => props.status?.tracking_status || props.channel.tracking_status)
const isActive = computed(() => trackingStatus.value === 'active')
const lastSynced = computed(() => relative(props.status?.last_synced_at || props.channel.last_synced_at))
const commentCount = computed(() => props.status?.comment_count ?? null)

const tokenExpiresAt = computed(() => {
  const v = props.channel.token_expires_at
  if (!v) return null
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? null : d
})

const tokenWarning = computed(() => {
  const t = tokenExpiresAt.value
  if (!t) return null
  const now = Date.now()
  if (t.getTime() < now) return 'expired'
  if (t.getTime() - now < 7 * 86400000) return 'soon'
  return null
})

function onBackfillChange(e: Event): void {
  const v = Number((e.target as HTMLSelectElement).value)
  if (Number.isFinite(v)) emit('update-backfill', props.channel.id, v)
}

// Reconnect must pre-open the popup synchronously inside this click handler
// (popup-blocker contract) — the auth-URL fetch happens afterwards.
function onReauthorize(): void {
  const popup = window.open('', 'oauth_youtube', 'width=500,height=600,menubar=no,toolbar=no,location=yes,status=no,resizable=yes,scrollbars=yes')
  emit('reauthorize', props.channel.id, popup)
}
</script>

<template>
  <Card tag="article" class="overflow-hidden">
    <div
      v-if="tokenWarning === 'soon'"
      class="px-5 py-2 bg-amber-50 dark:bg-amber-400/10 text-amber-800 dark:text-amber-200 text-xs flex items-center gap-2" data-unslop-ignore
    >
      <AlertTriangle class="w-4 h-4" />
      <span class="flex-1">
        Your access will expire {{ tokenExpiresAt ? relative(tokenExpiresAt) : 'soon' }}. Reconnect to continue tracking.
      </span>
      <button
        type="button"
        @click="onReauthorize"
        class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg text-[11px] font-semibold text-amber-800 dark:text-amber-200 bg-amber-100 dark:bg-amber-400/20 hover:bg-amber-200 dark:hover:bg-amber-400/30 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-500" data-unslop-ignore
      >
        <RefreshCw class="w-3 h-3" /> Reconnect
      </button>
    </div>
    <div
      v-else-if="tokenWarning === 'expired'"
      class="px-5 py-2 bg-rose-50 dark:bg-rose-400/10 text-rose-800 dark:text-rose-200 text-xs flex items-center gap-2"
    >
      <CircleAlert class="w-4 h-4" />
      <span class="flex-1">Your access has expired. Reconnect to resume tracking.</span>
      <button
        type="button"
        @click="onReauthorize"
        class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg text-[11px] font-semibold text-rose-800 dark:text-rose-200 bg-rose-100 dark:bg-rose-400/20 hover:bg-rose-200 dark:hover:bg-rose-400/30 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-rose-500"
      >
        <RefreshCw class="w-3 h-3" /> Reconnect
      </button>
    </div>

    <div class="px-5 py-5 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
      <div class="flex items-center min-w-0 gap-3">
        <div class="flex-shrink-0 h-12 w-12 flex items-center justify-center rounded-xl bg-red-50 text-red-600 border border-red-100 dark:border-red-500/20 shadow-sm relative">
          <svg class="h-6 w-6" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
            <path d="M21.582,6.186c-0.23-0.86-0.908-1.538-1.768-1.768C18.254,4,12,4,12,4S5.746,4,4.186,4.418 c-0.86,0.23-1.538,0.908-1.768,1.768C2,7.746,2,12,2,12s0,4.254,0.418,5.814c0.23,0.86,0.908,1.538,1.768,1.768 C5.746,20,12,20,12,20s6.254,0,7.814-0.418c0.86-0.23,1.538-0.908,1.768-1.768C22,16.254,22,12,22,12S22,7.746,21.582,6.186z M9.996,15.005l0-6.01l5.418,3.004L9.996,15.005z" />
          </svg>
          <div v-if="isActive" class="absolute -top-1 -right-1 flex h-3 w-3">
            <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" data-unslop-ignore></span>
            <span class="relative inline-flex rounded-full h-3 w-3 bg-emerald-500 border-2 border-white"></span>
          </div>
        </div>
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h4 class="text-base font-semibold text-zinc-900 dark:text-white truncate">{{ channel.display_name }}</h4>
            <span
              :class="[
                'inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset',
                isActive
                  ? 'bg-emerald-50 dark:bg-emerald-400/10 text-emerald-700 dark:text-emerald-300 ring-emerald-600/20 dark:ring-emerald-400/20'
                  : 'bg-zinc-50 dark:bg-white/5 text-zinc-700 dark:text-zinc-300 ring-zinc-500/10 dark:ring-white/10',
              ]"
            >
              {{ isActive ? 'Active' : 'Paused' }}
            </span>
          </div>
          <div class="mt-1 flex items-center text-xs text-zinc-500 dark:text-zinc-400 gap-2 flex-wrap">
            <span>YouTube</span>
            <span v-if="lastSynced" class="w-1 h-1 rounded-full bg-zinc-300"></span>
            <span v-if="lastSynced">Synced {{ lastSynced }}</span>
            <span v-if="commentCount !== null" class="w-1 h-1 rounded-full bg-zinc-300"></span>
            <span v-if="commentCount !== null" class="font-medium text-zinc-700 dark:text-zinc-300">{{ commentCount }} comments</span>
          </div>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <label class="inline-flex items-center gap-1.5 text-xs text-zinc-500 dark:text-zinc-400">
          Backfill
          <select
            :value="channel.backfill_days"
            @change="onBackfillChange"
            class="appearance-none px-2 py-1 text-xs border border-zinc-200 dark:border-white/10 rounded-lg bg-zinc-50/50 dark:bg-zinc-800 dark:text-zinc-100 hover:bg-zinc-50 dark:hover:bg-zinc-700 focus:bg-white dark:focus:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-brand-500/30 focus:border-brand-500/70 cursor-pointer"
            aria-label="Backfill window in days"
          >
            <option :value="30">30d</option>
            <option :value="60">60d</option>
            <option :value="90">90d</option>
            <option :value="180">180d</option>
          </select>
        </label>
        <button
          v-if="isActive"
          type="button"
          @click="emit('pause', channel.id)"
          class="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-sm font-medium text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] shadow-card dark:shadow-none focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
        >
          <Pause class="w-3.5 h-3.5" /> Pause
        </button>
        <button
          v-else
          type="button"
          @click="emit('resume', channel.id)"
          class="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-sm font-medium text-emerald-700 dark:text-emerald-300 bg-emerald-50 dark:bg-emerald-400/10 hover:bg-emerald-100 dark:hover:bg-emerald-400/20 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500"
        >
          <Play class="w-3.5 h-3.5" /> Resume
        </button>
        <button
          type="button"
          @click="emit('remove', channel.id)"
          aria-label="Remove channel"
          class="flex items-center justify-center h-11 w-11 rounded-lg text-zinc-500 dark:text-zinc-400 hover:text-rose-600 dark:hover:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-950/30 transition-all duration-200 active:scale-95 focus:outline-none focus-visible:ring-2 focus-visible:ring-rose-500"
        >
          <Trash2 class="w-4 h-4" />
        </button>
      </div>
    </div>

    <p class="px-5 pb-3 text-[11px] text-zinc-500 dark:text-zinc-400">
      Backfill setting applies to channels you connect in the future, not this one.
    </p>
  </Card>
</template>
