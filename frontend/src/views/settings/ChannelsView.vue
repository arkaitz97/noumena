<script setup lang="ts">
// Channels (/settings/channels) — connect / pause / resume / remove / backfill.
//
// Connect button MUST call connectYouTube synchronously inside its click
// handler so window.open is not blocked. The backfill modal opens *first*
// (no popup needed), and the popup is opened from within the modal's
// "Connect" click handler.

import { computed, onMounted, ref } from 'vue'
import { Plus, Tv, Info } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { useChannelsStore } from '../../stores/channels'
import { useBillingStore } from '../../stores/billing'
import type { Channel } from '../../services/types'
import ChannelCard from '../../components/ChannelCard.vue'
import ConfirmModal from '../../components/ConfirmModal.vue'

const channels = useChannelsStore()
const billing = useBillingStore()

// ---- connect modal -------------------------------------------------------
const connectOpen = ref(false)
const backfillDays = ref(90)
const connecting = ref(false)
const connectMode = ref<'oauth' | 'byName'>('oauth')
const channelRef = ref('')
const refError = ref('')

function openConnect(): void {
  if (atChannelLimit.value) return
  backfillDays.value = 90
  connectMode.value = 'oauth'
  channelRef.value = ''
  refError.value = ''
  connectOpen.value = true
}

async function submitConnect(): Promise<void> {
  if (connectMode.value === 'byName') {
    if (!channelRef.value.trim()) {
      refError.value = 'Enter a channel handle (e.g. @MrBeast) or a channel URL.'
      return
    }
    connecting.value = true
    try {
      await channels.connectYouTubeByName(channelRef.value.trim(), backfillDays.value)
    } finally {
      connecting.value = false
      connectOpen.value = false
    }
    return
  }
  connecting.value = true
  try {
    await channels.connectYouTube(backfillDays.value)
  } finally {
    connecting.value = false
    connectOpen.value = false
  }
}

// ---- remove modal --------------------------------------------------------
const removeTarget = ref<Channel | null>(null)
const removeOpen = ref(false)

function openRemove(id: string): void {
  const ch = channels.channels.find((c) => c.id === id) || null
  removeTarget.value = ch
  removeOpen.value = true
}

async function confirmRemove(): Promise<void> {
  if (!removeTarget.value) return
  const id = removeTarget.value.id
  removeOpen.value = false
  await channels.remove(id)
  // Refresh billing so the limit-check stays honest.
  void billing.fetchStatus()
}

// Reauthorize: popup pre-opened synchronously by ChannelCard's click
// handler; the store fetches the auth URL and navigates the popup.
function onReauthorize(id: string, popup: Window | null): void {
  void channels.reauthorize(id, popup)
}

// ---- limit gating --------------------------------------------------------
const atChannelLimit = computed(() => {
  const s = billing.status
  if (!s || s.channels_unlimited) return false
  return s.channels_used >= s.channels_limit
})

// ---- mount ---------------------------------------------------------------
onMounted(async () => {
  void billing.fetchStatus()
  await channels.fetchAll()
  // Dispatch one status fetch per channel in parallel.
  await Promise.all(channels.channels.map((c) => channels.fetchStatus(c.id)))
})
</script>

<template>
  <div class="max-w-5xl mx-auto py-8 px-4 sm:px-6 lg:px-8 space-y-8">
    <!-- Header + connect button -->
    <div class="sm:flex sm:items-center sm:justify-between">
      <div>
        <p class="eyebrow mb-2">Sources</p>
        <h1 class="text-display-sm font-bold text-zinc-900 dark:text-white">Connected channels</h1>
        <p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
          Manage your tracked channels, pause processing, or remove channels you no longer need.
        </p>
      </div>
      <div class="mt-5 sm:mt-0 flex-shrink-0">
        <button
          type="button"
          :disabled="atChannelLimit"
          @click="openConnect"
          :title="atChannelLimit ? 'Channel limit reached — upgrade your plan to add more.' : ''"
          class="w-full sm:w-auto inline-flex items-center justify-center gap-2 px-5 py-2.5 border border-transparent shadow-card text-sm font-medium rounded-xl text-white bg-zinc-900 hover:bg-zinc-700 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Plus class="w-4 h-4" />
          Connect YouTube channel
        </button>
        <p v-if="atChannelLimit" class="mt-2 text-xs text-rose-600 dark:text-rose-400">
          Channel limit reached.
          <RouterLink to="/settings/billing?next=/settings/channels" class="underline font-medium">
            Upgrade your plan
          </RouterLink>
          to add more.
        </p>
      </div>
    </div>

    <!-- Loading skeleton -->
    <div v-if="channels.loading && channels.channels.length === 0" class="space-y-3">
      <div
        v-for="i in 2"
        :key="i"
        class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl h-24 animate-pulse"
      ></div>
    </div>

    <!-- Empty state -->
    <div
      v-else-if="channels.channels.length === 0"
      class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-12 text-center"
    >
      <Tv class="mx-auto h-10 w-10 text-zinc-300 mb-3" />
      <h2 class="text-sm font-semibold text-zinc-900 dark:text-white mb-1">No channels connected</h2>
      <p class="text-sm text-zinc-500 dark:text-zinc-400 max-w-sm mx-auto">
        Connect your first YouTube channel to start getting AI-powered insights.
      </p>
      <button
        type="button"
        :disabled="atChannelLimit"
        @click="openConnect"
        class="mt-5 inline-flex items-center gap-2 px-4 py-2 rounded-xl text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-700 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200 shadow-card dark:shadow-none transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        <Plus class="w-4 h-4" /> Connect channel
      </button>
    </div>

    <!-- List -->
    <div v-else class="space-y-3">
      <ChannelCard
        v-for="channel in channels.channels"
        :key="channel.id"
        :channel="channel"
        :status="channels.statusById[channel.id] || null"
        @pause="(id) => channels.pause(id)"
        @resume="(id) => channels.resume(id)"
        @remove="openRemove"
        @update-backfill="(id, d) => channels.updateBackfill(id, d)"
        @reauthorize="onReauthorize"
      />
    </div>

    <!-- Connect modal: choose backfill window before opening the OAuth popup -->
    <ConfirmModal
      v-model:open="connectOpen"
      title="Connect YouTube channel"
      :confirm-disabled="connecting"
      :confirm-label="connecting ? 'Connecting…' : 'Connect'"
      cancel-label="Cancel"
      @confirm="submitConnect"
    >
      <p class="text-sm text-zinc-500 dark:text-zinc-400 leading-relaxed">
        Choose how to connect, then pick how far back to import existing comments.
        Larger windows take longer to process.
      </p>
      <div class="mt-4">
        <div class="flex rounded-xl bg-zinc-100 dark:bg-zinc-800 p-1" role="tablist" aria-label="Connection method">
          <button
            type="button"
            role="tab"
            :aria-selected="connectMode === 'oauth'"
            :class="[
              'flex-1 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors focus:outline-none',
              connectMode === 'oauth'
                ? 'bg-white dark:bg-zinc-700 text-zinc-900 dark:text-white shadow-card'
                : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200',
            ]"
            @click="connectMode = 'oauth'"
          >
            My channel
          </button>
          <button
            type="button"
            role="tab"
            :aria-selected="connectMode === 'byName'"
            :class="[
              'flex-1 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors focus:outline-none',
              connectMode === 'byName'
                ? 'bg-white dark:bg-zinc-700 text-zinc-900 dark:text-white shadow-card'
                : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200',
            ]"
            @click="connectMode = 'byName'"
          >
            Competitor channel
          </button>
        </div>

        <div v-if="connectMode === 'byName'" class="mt-4">
          <label for="channel-ref" class="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">
            Channel handle or URL
          </label>
          <input
            id="channel-ref"
            v-model="channelRef"
            type="text"
            placeholder="@MrBeast or https://www.youtube.com/@MrBeast"
            class="block w-full px-3 py-2.5 border border-zinc-200/70 dark:border-white/5 rounded-xl text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 focus-visible:border-brand-500/70 bg-zinc-50/50 dark:bg-zinc-800 dark:text-white"
          />
          <p v-if="refError" class="mt-1.5 text-xs text-rose-600 dark:text-rose-400" role="alert">{{ refError }}</p>
          <p class="mt-2 text-xs text-zinc-500 dark:text-zinc-400 flex items-start gap-1">
            <Info class="w-3.5 h-3.5 mt-0.5 flex-shrink-0" />
            Tracks the channel's public comments. No YouTube sign-in needed.
          </p>
        </div>
      </div>
      <div class="mt-4">
        <label for="backfill-days" class="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">
          Backfill window
        </label>
        <select
          id="backfill-days"
          v-model.number="backfillDays"
          class="block w-full px-3 py-2.5 border border-zinc-200/70 dark:border-white/5 rounded-xl text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60 focus-visible:border-brand-500/70 bg-zinc-50/50 dark:bg-zinc-800 dark:text-white"
        >
          <option :value="30">Last 30 days</option>
          <option :value="60">Last 60 days</option>
          <option :value="90">Last 90 days (recommended)</option>
          <option :value="180">Last 180 days</option>
        </select>
        <p v-if="connectMode === 'oauth'" class="mt-2 text-xs text-zinc-500 dark:text-zinc-400 flex items-start gap-1">
          <Info class="w-3.5 h-3.5 mt-0.5 flex-shrink-0" />
          A pop-up will open for YouTube to confirm access. Allow pop-ups for this site if you're prompted.
        </p>
      </div>
    </ConfirmModal>

    <!-- Remove modal -->
    <ConfirmModal
      v-model:open="removeOpen"
      title="Remove channel"
      :destructive="true"
      :confirm-label="`Remove ${removeTarget?.display_name ?? 'channel'}`"
      cancel-label="Cancel"
      @confirm="confirmRemove"
    >
      <p class="text-sm text-zinc-700 dark:text-zinc-300">
        We'll stop tracking
        <strong class="font-semibold text-zinc-900">{{ removeTarget?.display_name }}</strong>
        and detach it from your account.
      </p>
      <p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
        Other users connected to the same channel keep their data and tracking.
        You can reconnect this channel at any time.
      </p>
    </ConfirmModal>
  </div>
</template>
