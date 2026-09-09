<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
import { toast } from '../../lib/toast'
import { generateState, openOAuthPopup } from '../../lib/oauthPopup'
import * as channels from '../../services/channels'
import type { SyncStatus } from '../../services/types'
import PrimaryButton from '../../components/forms/PrimaryButton.vue'
import SecondaryButton from '../../components/forms/SecondaryButton.vue'
import AppLogo from '../../components/AppLogo.vue'

const router = useRouter()
const auth = useAuthStore()

type StepIdx = 1 | 2 | 3 | 4
const step = ref<StepIdx>(1)
const stepLabels = ['Welcome', 'Connect', 'Backfill', 'Tour']

// Step 2 state
const connecting = ref(false)
const connectError = ref<string | null>(null)
const connectedChannel = ref<{ id: string; display_name: string } | null>(null)

// Step 3 state — polling
const syncStatus = ref<SyncStatus | null>(null)
const progressPct = computed<number>(() => syncStatus.value?.backfill_progress_percent ?? 0)
const pollIntervalMs = ref(10000)
let pollTimer: number | null = null
let pollStartedAt = 0
const stableCount = ref(0)
const lastProgress = ref<number | null>(null)
const escapeHatchVisible = ref(false)
const POLL_MAX_MS = 30 * 60 * 1000 // 30 min
const PROGRESS_THRESHOLD = 10

function stopPolling() {
  if (pollTimer !== null) {
    window.clearTimeout(pollTimer)
    pollTimer = null
  }
}

async function pollOnce(): Promise<void> {
  if (!connectedChannel.value) return
  try {
    const s = await channels.syncStatus(connectedChannel.value.id)
    syncStatus.value = s
    const pct = s.backfill_progress_percent ?? 0
    if (lastProgress.value !== null && Math.abs(pct - lastProgress.value) < 0.01) {
      stableCount.value++
    } else {
      stableCount.value = 0
    }
    lastProgress.value = pct
    if (stableCount.value >= 4 && pollIntervalMs.value < 60000) {
      pollIntervalMs.value = Math.min(60000, pollIntervalMs.value * 2)
    }
    if (pct >= 100) {
      toast.success('Sync complete! Your insights are ready.')
      stopPolling()
      setTimeout(() => router.push('/dashboard'), 1200)
      return
    }
    // Failed: the channel reports an error. Stop polling and offer the
    // escape hatch so the user can manage channels instead.
    if (s.recent_error) {
      escapeHatchVisible.value = true
      stopPolling()
    }
  } catch (err) {
    // Transient failure — continue polling.
  }
}

function schedulePoll() {
  if (Date.now() - pollStartedAt > POLL_MAX_MS) {
    escapeHatchVisible.value = true
    stopPolling()
    return
  }
  pollTimer = window.setTimeout(async () => {
    await pollOnce()
    // Keep polling until the backfill completes or fails — the old
    // "progress >= 10%" gate froze the bar mid-sync.
    if (progressPct.value < 100 && !syncStatus.value?.recent_error) {
      schedulePoll()
    }
  }, pollIntervalMs.value)
}

async function startBackfillPolling() {
  pollStartedAt = Date.now()
  pollIntervalMs.value = 10000
  stableCount.value = 0
  lastProgress.value = null
  escapeHatchVisible.value = false
  await pollOnce()
  if (progressPct.value < 100 && !syncStatus.value?.recent_error) schedulePoll()
}

onBeforeUnmount(() => {
  stopPolling()
})

// --- Step navigation ---
function goToStep(s: StepIdx) {
  step.value = s
}

function startOnboarding() {
  goToStep(2)
}

async function connectYouTube() {
  if (connecting.value) return
  connecting.value = true
  connectError.value = null
  const state = generateState('youtube')
  try {
    const { auth_url } = await channels.connectYouTubeUrl()
    try {
      const result = await openOAuthPopup('youtube', auth_url, { state })
      const exchange = await channels.exchangeYouTube(result.code, result.state, 90)
      connectedChannel.value = { id: exchange.id, display_name: exchange.display_name }
      await auth.refreshChannelsCount()
      toast.success(`Connected ${exchange.display_name}`)
      goToStep(3)
      await startBackfillPolling()
    } catch (popupErr: any) {
      const m = popupErr?.message
      if (m === 'popup_blocked')
        connectError.value = 'Pop-up blocked. Please allow pop-ups and try again.'
      else if (m === 'popup_closed') connectError.value = 'Connection cancelled.'
      else if (m === 'popup_timeout') connectError.value = 'Connection timed out. Please try again.'
      else connectError.value = popupErr?.response?.data?.error || popupErr?.message || 'Connection failed.'
    }
  } catch (err: any) {
    connectError.value = err?.response?.data?.error || 'Could not start connection.'
  } finally {
    connecting.value = false
  }
}

const continueEnabled = computed(
  () => progressPct.value >= PROGRESS_THRESHOLD || escapeHatchVisible.value,
)

function continueToStep4() {
  stopPolling()
  goToStep(4)
}

function skipTour() {
  router.push('/dashboard')
}

function finishOnboarding() {
  router.push('/dashboard')
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex flex-col">
    <header class="px-6 sm:px-12 py-6 flex items-center justify-between">
      <AppLogo size="md" show-text />
      <span class="text-xs font-medium text-zinc-500 dark:text-zinc-400">Step {{ step }} of 4 · {{ stepLabels[step - 1] }}</span>
    </header>

    <!-- Progress bar -->
    <div class="px-6 sm:px-12">
      <div class="h-1.5 bg-zinc-200 dark:bg-zinc-800 rounded-full overflow-hidden">
        <div
          class="h-full bg-brand-500 transition-all duration-500"
          :style="{ width: ((step / 4) * 100) + '%' }"
        ></div>
      </div>
    </div>

    <main class="flex-1 flex items-center justify-center px-6 sm:px-12 py-12">
      <div class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl max-w-xl w-full p-8 sm:p-12 relative overflow-hidden">
        <div class="absolute top-0 right-0 -mt-16 -mr-16 w-48 h-48 bg-gradient-to-br from-brand-500 to-brand-400 rounded-full blur-[80px] opacity-20 pointer-events-none"></div>

        <!-- STEP 1: Welcome -->
        <template v-if="step === 1">
          <h1 class="text-3xl font-semibold tracking-tight text-zinc-900 dark:text-white mb-3">Welcome to Noumena</h1>
          <p class="text-zinc-500 dark:text-zinc-400 leading-relaxed mb-8">
            Let's connect your first channel. We'll automatically pull in your latest comments and
            start surfacing the insights hidden in them.
          </p>
          <ul class="space-y-3 text-sm text-zinc-700 dark:text-zinc-300 mb-10">
            <li class="flex items-start gap-3">
              <span class="mt-1 w-5 h-5 rounded-full bg-emerald-50 dark:bg-emerald-400/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center ring-1 ring-emerald-600/20">
                <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" /></svg>
              </span>
              Read-only access — we never post or modify anything.
            </li>
            <li class="flex items-start gap-3">
              <span class="mt-1 w-5 h-5 rounded-full bg-emerald-50 dark:bg-emerald-400/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center ring-1 ring-emerald-600/20">
                <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" /></svg>
              </span>
              First insights ready in under 30 minutes.
            </li>
            <li class="flex items-start gap-3">
              <span class="mt-1 w-5 h-5 rounded-full bg-emerald-50 dark:bg-emerald-400/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center ring-1 ring-emerald-600/20">
                <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" /></svg>
              </span>
              You can disconnect at any time from Settings.
            </li>
          </ul>
          <PrimaryButton full-width @click="startOnboarding">Get started</PrimaryButton>
        </template>

        <!-- STEP 2: Connect -->
        <template v-else-if="step === 2">
          <h1 class="text-3xl font-semibold tracking-tight text-zinc-900 dark:text-white mb-3">Connect your YouTube channel</h1>
          <p class="text-zinc-500 dark:text-zinc-400 leading-relaxed mb-8">
            We'll open a Google sign-in window and ask for read-only access to your channel's
            comments.
          </p>
          <div v-if="connectError" class="bg-rose-50 p-4 rounded-xl text-rose-700 dark:bg-rose-400/10 dark:text-rose-300 text-sm border border-rose-100 mb-6">
            {{ connectError }}
          </div>

          <button
            @click="connectYouTube"
            :disabled="connecting"
            class="group relative w-full flex justify-center items-center py-3.5 px-4 rounded-xl border border-zinc-200 bg-white text-base font-medium text-zinc-700 dark:text-zinc-200 dark:bg-zinc-800/60 dark:border-white/10 shadow-card dark:shadow-none hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 transition-all duration-200 ease-snappy active:scale-[0.98] disabled:opacity-60"
          >
            <svg class="w-6 h-6 mr-3 text-[#FF0000]" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path fill-rule="evenodd" d="M21.582,6.186c-0.23-0.86-0.908-1.538-1.768-1.768C18.254,4,12,4,12,4S5.746,4,4.186,4.418 c-0.86,0.23-1.538,0.908-1.768,1.768C2,7.746,2,12,2,12s0,4.254,0.418,5.814c0.23,0.86,0.908,1.538,1.768,1.768 C5.746,20,12,20,12,20s6.254,0,7.814-0.418c0.86-0.23,1.538-0.908,1.768-1.768C22,16.254,22,12,22,12S22,7.746,21.582,6.186z M9.996,15.005l0-6.01l5.418,3.004L9.996,15.005z" clip-rule="evenodd" />
            </svg>
            <span v-if="connecting">Opening Google…</span>
            <span v-else>Continue with YouTube</span>
          </button>
        </template>

        <!-- STEP 3: Backfill -->
        <template v-else-if="step === 3">
          <h1 class="text-3xl font-semibold tracking-tight text-zinc-900 dark:text-white mb-3">Syncing your channel</h1>
          <p v-if="connectedChannel" class="text-zinc-500 dark:text-zinc-400 mb-2">
            Connected: <span class="font-medium text-zinc-900 dark:text-zinc-100">{{ connectedChannel.display_name }}</span>
          </p>
          <p class="text-zinc-500 dark:text-zinc-400 leading-relaxed mb-8">
            We're pulling in your recent comments. You can continue once we have enough data to show
            useful insights.
          </p>

          <div class="mb-8">
            <div class="flex items-center justify-between text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-2">
              <span>Backfill progress</span>
              <span>{{ Math.floor(progressPct) }}%</span>
            </div>
            <div class="h-2 bg-zinc-100 dark:bg-zinc-800 rounded-full overflow-hidden">
              <div
                class="h-full bg-brand-500 transition-all duration-500"
                :style="{ width: Math.min(100, progressPct) + '%' }"
              ></div>
            </div>
            <p v-if="syncStatus?.recent_error" class="mt-3 text-xs text-rose-600 dark:text-rose-400">
              {{ syncStatus.recent_error.message }}
            </p>
          </div>

          <p v-if="escapeHatchVisible" class="text-sm text-zinc-500 dark:text-zinc-400 mb-6">
            This is taking longer than usual — you can continue and we'll keep syncing in the background.
            <RouterLink to="/settings/channels" class="underline font-medium text-zinc-700 dark:text-zinc-300">
              Manage channels
            </RouterLink>.
          </p>

          <PrimaryButton full-width :disabled="!continueEnabled" @click="continueToStep4">
            <span v-if="continueEnabled">Continue</span>
            <span v-else>Syncing… ({{ Math.floor(progressPct) }}%)</span>
          </PrimaryButton>
        </template>

        <!-- STEP 4: Tour -->
        <template v-else>
          <h1 class="text-3xl font-semibold tracking-tight text-zinc-900 dark:text-white mb-3">You're all set</h1>
          <p class="text-zinc-500 dark:text-zinc-400 leading-relaxed mb-8">
            A quick tour of the dashboard, or jump straight in?
          </p>
          <ul class="space-y-3 text-sm text-zinc-700 dark:text-zinc-300 mb-10">
            <li><span class="font-semibold text-zinc-900 dark:text-zinc-100">Insights Feed</span> — your prioritized comment stream.</li>
            <li><span class="font-semibold text-zinc-900 dark:text-zinc-100">Saved Comments</span> — flagged items for follow-up.</li>
            <li><span class="font-semibold text-zinc-900 dark:text-zinc-100">Channels</span> — connect more or pause syncing.</li>
          </ul>
          <div class="flex flex-col sm:flex-row gap-3">
            <PrimaryButton full-width @click="finishOnboarding">Take me to the dashboard</PrimaryButton>
            <SecondaryButton full-width @click="skipTour">Skip</SecondaryButton>
          </div>
        </template>
      </div>
    </main>
  </div>
</template>
