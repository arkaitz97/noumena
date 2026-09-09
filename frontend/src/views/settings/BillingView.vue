<script setup lang="ts">
// Billing (/settings/billing) — current plan summary + plan picker + portal.
//
// Stripe Checkout: PlanComparison pre-opens a synchronous about:blank
// window in its click handler, then we feed it the URL once /billing/checkout
// resolves. Returning to this tab after Checkout refetches status once.

import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ExternalLink, AlertTriangle, Rocket } from 'lucide-vue-next'
import { useBillingStore } from '../../stores/billing'
import type { UsagePeriod } from '../../services/billing'
import type { BillingInterval, Plan } from '../../services/types'
import PlanComparison from '../../components/PlanComparison.vue'
import UsageBar from '../../components/UsageBar.vue'
import { shortDate } from '../../lib/relativeTime'
import { PLANS } from '../../lib/plans'
import Card from '../../components/Card.vue'

const billing = useBillingStore()

const interval = ref<BillingInterval>('monthly')

const currentPlan = computed<Plan | string | null>(() => billing.status?.plan ?? null)

const periodEndDisplay = computed(() => {
  if (!billing.status?.period_end) return null
  return shortDate(billing.status.period_end)
})

// Active trial countdown. The backend computes `trial` additively on
// /api/billing/status (active = ends_at in the future AND not on a paid
// plan). Hidden when inactive or already expired.
const trial = computed<{ daysLeft: number; end: Date } | null>(() => {
  const t = billing.status?.trial
  if (!t?.active) return null
  const end = new Date(t.ends_at)
  if (Number.isNaN(end.getTime())) return null
  return { end, daysLeft: Math.max(0, t.days_remaining) }
})

/** Mini usage-bar width for a history row, clamped to [0, 100]%. */
function usagePct(p: UsagePeriod): string {
  if (!p.quota || p.quota <= 0) return '0%'
  return `${Math.min(100, Math.round((p.comments_used / p.quota) * 100))}%`
}

function scrollToPlans(): void {
  document.getElementById('plan-picker')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

const showUsageAlert = computed(() => billing.nearLimit)

const usageAlertCopy = computed(() => {
  const s = billing.status
  if (!s) return ''
  if (billing.commentsUsedPct >= 0.95) {
    return `You've used ${(billing.commentsUsedPct * 100).toFixed(0)}% of your comment quota for this period.`
  }
  if (s.channels_unlimited) return ''
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
  void billing.fetchUsage()
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
      <h1 class="text-2xl font-semibold leading-7 text-zinc-900 dark:text-white tracking-tight">Billing & subscription</h1>
      <p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400">Manage your plan, payment method, and invoices.</p>
    </div>

    <!-- Loading skeleton -->
    <div v-if="billing.loading && !billing.status" class="space-y-3">
      <Card class="h-48 animate-pulse"></Card>
      <Card class="h-72 animate-pulse"></Card>
    </div>

    <template v-else-if="billing.status">
      <!-- Trial countdown banner -->
      <div
        v-if="trial"
        class="bg-brand-50 dark:bg-brand-950/40 ring-1 ring-brand-600/20 rounded-xl p-4 flex items-start gap-3"
        role="status"
      >
        <Rocket class="w-5 h-5 text-brand-600 dark:text-brand-300 flex-shrink-0 mt-0.5" />
        <div class="flex-1">
          <p class="text-sm font-semibold text-brand-900 dark:text-brand-200">
            Free trial ends in {{ trial.daysLeft }} day{{ trial.daysLeft === 1 ? '' : 's' }}
            ({{ shortDate(trial.end) }}).
          </p>
          <p class="text-xs text-brand-800 dark:text-brand-300 mt-1">
            Pick a plan below to keep your channel insights flowing without interruption.
          </p>
        </div>
        <button
          type="button"
          @click="scrollToPlans"
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold text-white bg-brand-600 hover:bg-brand-700 dark:bg-brand-500 dark:hover:bg-brand-400 transition-all duration-200 ease-snappy focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/60"
        >
          Upgrade
        </button>
      </div>

      <!-- Usage alert banner -->
      <div
        v-if="showUsageAlert"
        class="bg-amber-50 dark:bg-amber-950/40 ring-1 ring-amber-600/20 rounded-xl p-4 flex items-start gap-3" data-unslop-ignore
        role="alert"
      >
        <AlertTriangle class="w-5 h-5 text-amber-700 dark:text-amber-400 flex-shrink-0 mt-0.5" />
        <div class="flex-1">
          <p class="text-sm font-semibold text-amber-900 dark:text-amber-200">You're approaching your plan limit.</p>
          <p class="text-xs text-amber-800 dark:text-amber-300 mt-1">{{ usageAlertCopy }}</p>
        </div>
      </div>

      <!-- Current plan summary -->
      <Card padding="lg">
        <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-5">
          <div>
            <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider">Current plan</p>
            <h2 class="mt-1 text-xl font-semibold text-zinc-900 dark:text-white tracking-tight capitalize">
              {{ billing.status.plan }}
              <span v-if="billing.status.interval" class="text-sm font-medium text-zinc-500 dark:text-zinc-400">
                · {{ billing.status.interval }}
              </span>
            </h2>
            <p v-if="periodEndDisplay" class="text-xs text-zinc-500 dark:text-zinc-400 mt-1">
              Next billing date: {{ periodEndDisplay }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              :disabled="portalDisabled"
              :title="portalDisabled ? 'Available after your first paid plan.' : ''"
              @click="billing.openPortal()"
              class="inline-flex items-center gap-1.5 py-2 px-4 rounded-xl text-sm font-medium text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed"
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
            :unlimited="billing.status.channels_unlimited"
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
          class="mt-4 text-xs text-amber-700 dark:text-amber-400"
        >
          Overage this period: ${{ billing.status.overage_cost_usd.toFixed(2) }}
        </p>
      </Card>

      <!-- Usage history -->
      <Card padding="lg">
        <div class="mb-5">
          <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider">Usage history</p>
          <h3 class="mt-1 text-lg font-semibold text-zinc-900 dark:text-white tracking-tight">
            Comment quota by period
          </h3>
        </div>

        <div
          v-if="billing.usageLoading && billing.usagePeriods.length === 0"
          class="space-y-2"
          aria-label="Loading usage history"
        >
          <div class="h-9 bg-zinc-100 dark:bg-zinc-800 animate-pulse rounded-lg"></div>
          <div class="h-9 bg-zinc-100 dark:bg-zinc-800 animate-pulse rounded-lg"></div>
          <div class="h-9 bg-zinc-100 dark:bg-zinc-800 animate-pulse rounded-lg"></div>
        </div>

        <p v-else-if="billing.usagePeriods.length === 0" class="text-sm text-zinc-500 dark:text-zinc-400">
          No usage history yet.
        </p>

        <div v-else class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-left text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider border-b border-zinc-200/70 dark:border-white/5">
                <th scope="col" class="py-2 pr-4 font-medium">Period</th>
                <th scope="col" class="py-2 pr-4 font-medium text-right">Comments used</th>
                <th scope="col" class="py-2 pr-4 font-medium text-right">Quota</th>
                <th scope="col" class="py-2 font-medium">Usage</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-zinc-200/70 dark:divide-white/5">
              <tr v-for="p in billing.usagePeriods" :key="p.period_start">
                <td class="py-3 pr-4 text-zinc-900 dark:text-white whitespace-nowrap">
                  {{ shortDate(p.period_start) }} – {{ shortDate(p.period_end) }}
                </td>
                <td class="py-3 pr-4 text-right text-zinc-700 dark:text-zinc-300 font-mono tabular-nums whitespace-nowrap">
                  {{ p.comments_used }}
                </td>
                <td class="py-3 pr-4 text-right text-zinc-700 dark:text-zinc-300 font-mono tabular-nums whitespace-nowrap">
                  {{ p.quota }}
                </td>
                <td class="py-3">
                  <div class="w-24 h-2 bg-zinc-100 dark:bg-zinc-800 rounded-full overflow-hidden">
                    <div
                      class="h-full rounded-full bg-brand-500" data-unslop-ignore
                      :style="{ width: usagePct(p) }"
                      :aria-label="`${usagePct(p)} of quota used`"
                    ></div>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </Card>

      <!-- Plan picker -->
      <div id="plan-picker">
        <PlanComparison
          :plans="PLANS"
          :current-plan="currentPlan"
          :interval="interval"
          @update:interval="interval = $event"
          @select-plan="onSelectPlan"
        />
      </div>
    </template>
  </div>
</template>
