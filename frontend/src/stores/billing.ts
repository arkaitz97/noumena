// useBillingStore — Stripe Checkout + Customer Portal + status read.

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as billingApi from '../services/billing'
import { NoStripeCustomerError } from '../services/billing'
import { toast } from '../lib/toast'
import type { BillingInterval, BillingStatus, Plan } from '../services/types'

export const useBillingStore = defineStore('billing', () => {
  const status = ref<BillingStatus | null>(null)
  const loading = ref(false)
  const portalUnavailable = ref(false)

  async function fetchStatus(): Promise<void> {
    loading.value = true
    try {
      status.value = await billingApi.status()
    } catch {
      toast.error('Could not load your billing status.')
    } finally {
      loading.value = false
    }
  }

  /**
   * Stripe Checkout flow. The caller MUST pass a `prepopened` Window the
   * click handler opened synchronously (via `window.open('about:blank')`)
   * so the redirect dodges the popup blocker.
   */
  async function startCheckout(
    plan: Plan | string,
    interval: BillingInterval,
    prepopened: Window | null = null,
  ): Promise<void> {
    try {
      const session = await billingApi.checkout(plan, interval)
      if (prepopened && !prepopened.closed) {
        prepopened.location.href = session.url
      } else {
        const w = window.open(session.url, '_blank', 'noopener')
        if (!w) {
          toast.error('Pop-up blocked. Click the upgrade button again to continue.')
        }
      }
    } catch {
      if (prepopened && !prepopened.closed) prepopened.close()
      toast.error('Could not start checkout. Please try again.')
    }
  }

  async function openPortal(): Promise<void> {
    try {
      const session = await billingApi.portal()
      window.open(session.url, '_blank', 'noopener')
    } catch (err) {
      if (err instanceof NoStripeCustomerError) {
        portalUnavailable.value = true
        toast.info('Available after your first paid plan.')
      } else {
        toast.error('Could not open the billing portal.')
      }
    }
  }

  // ---- computed helpers ----
  const channelsRemaining = computed(() => {
    if (!status.value) return null
    return Math.max(0, status.value.channels_limit - status.value.channels_used)
  })
  const commentsRemaining = computed(() => {
    if (!status.value) return null
    return Math.max(0, status.value.comments_limit - status.value.comments_used)
  })
  const commentsUsedPct = computed(() => {
    if (!status.value || status.value.comments_limit <= 0) return 0
    return status.value.comments_used / status.value.comments_limit
  })
  const channelsUsedPct = computed(() => {
    if (!status.value || status.value.channels_limit <= 0) return 0
    return status.value.channels_used / status.value.channels_limit
  })
  const nearLimit = computed(() => commentsUsedPct.value >= 0.95 || channelsUsedPct.value >= 1)

  return {
    status,
    loading,
    portalUnavailable,
    fetchStatus,
    startCheckout,
    openPortal,
    channelsRemaining,
    commentsRemaining,
    commentsUsedPct,
    channelsUsedPct,
    nearLimit,
  }
})
