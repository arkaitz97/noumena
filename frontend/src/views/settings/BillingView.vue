<script setup lang="ts">
// Billing (/settings/billing) — current plan summary + plan picker + portal.
//
// Stripe Checkout: PlanComparison pre-opens a synchronous about:blank
// window in its click handler, then we feed it the URL once /billing/checkout
// resolves. Returning to this tab after Checkout refetches status once.

import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ExternalLink, AlertTriangle } from 'lucide-vue-next'
import { useBillingStore } from '../../stores/billing'
import type { BillingInterval, Plan } from '../../services/types'
import PlanComparison, { type PlanOption } from '../../components/PlanComparison.vue'
import UsageBar from '../../components/UsageBar.vue'
import { shortDate } from '../../lib/relativeTime'

const billing = useBillingStore()

// Static plan catalog. The backend is the source of truth for selection
// validity; this is purely presentational copy.
const PLANS: PlanOption[] = [
  {
    id: 'free',
    name: 'Free',
    monthlyUsd: 0,
    annualUsd: 0,
    features: ['1 channel', '1,000 comments / mo', 'Basic insights'],
  },
  {
    id: 'creator',
    name: 'Creator',
    monthlyUsd: 29,
    annualUsd: 290,
    features: ['3 channels', '50,000 comments / mo', 'Sentiment + toxicity', 'Weekly digest'],
    highlight: true,
  },
  {
    id: 'studio',
    name: 'Studio',
    monthlyUsd: 79,
    annualUsd: 790,
    features: ['10 channels', '250,000 comments / mo', 'Topic trends', 'Priority support'],
  },
  {
    id: 'agency',
    name: 'Agency',
    monthlyUsd: 199,
    annualUsd: 1990,
    features: ['25 channels', '1M comments / mo', 'Team seats', 'Dedicated CSM'],
  },
]

const interval = ref<BillingInterval>('monthly')

const currentPlan = computed<Plan | string | null>(() => billing.status?.plan ?? null)

const periodEndDisplay = computed(() => {
  if (!billing.status?.period_end) return null
  return shortDate(billing.status.period_end)
})

const showUsageAlert = computed(() => billing.nearLimit)

const usageAlertCopy = computed(() => {
  const s = billing.status
  if (!s) return ''
  if (billing.commentsUsedPct >= 0.95) {
    return `You've used ${(billing.commentsUsedPct * 100).toFixed(0)}% of your comment quota for this period.`
  }
  if (s.channels_used >= s.channels_limit) {
    return `You've reached your channel limit (${s.channels_used}/${s.channels_limit}).`
  }
  return ''
})

// Hide / disable the portal button when on free plan or after a 409 has
// flipped the unavailable flag.
const portalDisabled = computed(() => {
  if (billing.portalUnavailable) return true
  if (!billing.status) return true
  return billing.status.plan === 'free'
})

async function onSelectPlan(
  plan: Plan | string,
  selectedInterval: BillingInterval,
  prepopened: Window | null,
): Promise<void> {
  await billing.startCheckout(plan, selectedInterval, prepopened)
  // Re-fetch status when the user returns to this tab.
  if (typeof window !== 'undefined') {
    window.addEventListener('focus', refetchOnce, { once: true })
  }
}

function refetchOnce(): void {
  void billing.fetchStatus()
}

onMounted(() => {
  void billing.fetchStatus()
})

onBeforeUnmount(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('focus', refetchOnce)
  }
})
</script>

<template>
  <div class="max-w-5xl mx-auto py-8 px-4 sm:px-6 lg:px-8 space-y-8">
    <div>
      <h3 class="text-2xl font-semibold leading-7 text-zinc-900 tracking-tight">Billing & subscription</h3>
      <p class="mt-2 text-sm text-zinc-500">Manage your plan, payment method, and invoices.</p>
    </div>

    <!-- Loading skeleton -->
    <div v-if="billing.loading && !billing.status" class="space-y-3">
      <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl h-48 animate-pulse"></div>
      <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl h-72 animate-pulse"></div>
    </div>

    <template v-else-if="billing.status">
      <!-- Usage alert banner -->
      <div
        v-if="showUsageAlert"
        class="bg-amber-50 ring-1 ring-amber-600/20 rounded-2xl p-4 flex items-start gap-3"
        role="alert"
      >
        <AlertTriangle class="w-5 h-5 text-amber-700 flex-shrink-0 mt-0.5" />
        <div class="flex-1">
          <p class="text-sm font-semibold text-amber-900">You're approaching your plan limit.</p>
          <p class="text-xs text-amber-800 mt-1">{{ usageAlertCopy }}</p>
        </div>
      </div>

      <!-- Current plan summary -->
      <section class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-6">
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-5">
          <div>
            <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider">Current plan</p>
            <h4 class="mt-1 text-xl font-semibold text-zinc-900 tracking-tight capitalize">
              {{ billing.status.plan }}
              <span v-if="billing.status.interval" class="text-sm font-medium text-zinc-500">
                · {{ billing.status.interval }}
              </span>
            </h4>
            <p v-if="periodEndDisplay" class="text-xs text-zinc-500 mt-1">
              Next billing date: {{ periodEndDisplay }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              :disabled="portalDisabled"
              :title="portalDisabled ? 'Available after your first paid plan.' : ''"
              @click="billing.openPortal()"
              class="inline-flex items-center gap-1.5 py-2 px-4 rounded-xl text-sm font-medium text-zinc-700 bg-white border border-zinc-200 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              <ExternalLink class="w-4 h-4" />
              Manage payment & invoices
            </button>
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
          <UsageBar
            :used="billing.status.channels_used"
            :limit="billing.status.channels_limit"
            unit=""
            label="Channels"
          />
          <UsageBar
            :used="billing.status.comments_used"
            :limit="billing.status.comments_limit"
            unit="comments"
            label="Comments this period"
          />
        </div>

        <p
          v-if="billing.status.overage_cost_usd && billing.status.overage_cost_usd > 0"
          class="mt-4 text-xs text-amber-700"
        >
          Overage this period: ${{ billing.status.overage_cost_usd.toFixed(2) }}
        </p>
      </section>

      <!-- Plan picker -->
      <PlanComparison
        :plans="PLANS"
        :current-plan="currentPlan"
        :interval="interval"
        @update:interval="interval = $event"
        @select-plan="onSelectPlan"
      />
    </template>
  </div>
</template>
