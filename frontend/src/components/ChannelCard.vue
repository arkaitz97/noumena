<script setup lang="ts">
import { computed } from 'vue'
import { Pause, Play, Trash2, AlertTriangle, CircleAlert } from 'lucide-vue-next'
import type { Channel, ChannelStatus } from '../services/types'
import { relative } from '../lib/relativeTime'

const props = defineProps<{
  channel: Channel
  status?: ChannelStatus | null
}>()

const emit = defineEmits<{
  (e: 'pause', id: string): void
  (e: 'resume', id: string): void
  (e: 'remove', id: string): void
  (e: 'update-backfill', id: string, days: number): void
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
</script>

<template>
  <article class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl overflow-hidden">
    <div
      v-if="tokenWarning === 'soon'"
      class="px-5 py-2 bg-amber-50 text-amber-800 text-xs flex items-center gap-2"
    >
      <AlertTriangle class="w-4 h-4" />
      Your access will expire {{ lastSynced ? '' : '' }}{{ tokenExpiresAt ? relative(tokenExpiresAt) : 'soon' }}.
      Reconnect to continue tracking.
    </div>
    <div
      v-else-if="tokenWarning === 'expired'"
      class="px-5 py-2 bg-rose-50 text-rose-800 text-xs flex items-center gap-2"
    >
      <CircleAlert class="w-4 h-4" />
      Your access has expired. Reconnect to resume tracking.
    </div>

    <div class="px-5 py-5 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
      <div class="flex items-center min-w-0 gap-3">
        <div class="flex-shrink-0 h-12 w-12 flex items-center justify-center rounded-xl bg-red-50 text-red-600 border border-red-100 shadow-sm relative">
          <svg class="h-6 w-6" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
            <path d="M21.582,6.186c-0.23-0.86-0.908-1.538-1.768-1.768C18.254,4,12,4,12,4S5.746,4,4.186,4.418 c-0.86,0.23-1.538,0.908-1.768,1.768C2,7.746,2,12,2,12s0,4.254,0.418,5.814c0.23,0.86,0.908,1.538,1.768,1.768 C5.746,20,12,20,12,20s6.254,0,7.814-0.418c0.86-0.23,1.538-0.908,1.768-1.768C22,16.254,22,12,22,12S22,7.746,21.582,6.186z M9.996,15.005l0-6.01l5.418,3.004L9.996,15.005z" />
          </svg>
          <div v-if="isActive" class="absolute -top-1 -right-1 flex h-3 w-3">
            <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
            <span class="relative inline-flex rounded-full h-3 w-3 bg-emerald-500 border-2 border-white"></span>
          </div>
        </div>
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h4 class="text-base font-semibold text-zinc-900 truncate">{{ channel.display_name }}</h4>
            <span
              :class="[
                'inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset',
                isActive
                  ? 'bg-emerald-50 text-emerald-700 ring-emerald-600/20'
                  : 'bg-zinc-100 text-zinc-700 ring-zinc-500/20',
              ]"
            >
              {{ isActive ? 'Active' : 'Paused' }}
            </span>
          </div>
          <div class="mt-1 flex items-center text-xs text-zinc-500 gap-2 flex-wrap">
            <span>YouTube</span>
            <span v-if="lastSynced" class="w-1 h-1 rounded-full bg-zinc-300"></span>
            <span v-if="lastSynced">Synced {{ lastSynced }}</span>
            <span v-if="commentCount !== null" class="w-1 h-1 rounded-full bg-zinc-300"></span>
            <span v-if="commentCount !== null" class="font-medium text-zinc-700">{{ commentCount }} comments</span>
          </div>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <label class="inline-flex items-center gap-1.5 text-xs text-zinc-500">
          Backfill
          <select
            :value="channel.backfill_days"
            @change="onBackfillChange"
            class="appearance-none px-2 py-1 text-xs border border-zinc-200 rounded-lg bg-zinc-50/50 hover:bg-zinc-50 focus:bg-white focus:outline-none focus:ring-2 focus:ring-zinc-900 cursor-pointer"
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
          class="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-sm font-medium text-zinc-700 bg-white border border-zinc-200 hover:bg-zinc-50 transition-colors focus:outline-none focus:ring-2 focus:ring-zinc-900"
        >
          <Pause class="w-3.5 h-3.5" /> Pause
        </button>
        <button
          v-else
          type="button"
          @click="emit('resume', channel.id)"
          class="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-sm font-medium text-emerald-700 bg-emerald-50 hover:bg-emerald-100 transition-colors focus:outline-none focus:ring-2 focus:ring-emerald-600"
        >
          <Play class="w-3.5 h-3.5" /> Resume
        </button>
        <button
          type="button"
          @click="emit('remove', channel.id)"
          aria-label="Remove channel"
          class="p-1.5 rounded-lg text-zinc-400 hover:text-rose-600 hover:bg-rose-50 transition-colors focus:outline-none focus:ring-2 focus:ring-rose-500"
        >
          <Trash2 class="w-4 h-4" />
        </button>
      </div>
    </div>

    <p class="px-5 pb-3 text-[11px] text-zinc-400">
      Backfill setting applies to channels you connect in the future, not this one.
    </p>
  </article>
</template>
