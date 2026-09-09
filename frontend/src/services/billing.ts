// /api/billing/* surface — status, usage history, checkout session,
// customer portal.
//
// `portal()` returns HTTP 409 when the caller has no Stripe customer
// (free tier without a prior payment). We rewrap that into a typed error
// so the store can swap the button for the "available after first paid
// plan" hint without inspecting Axios internals.
//
// NOTE: the usage/trial shapes below are exported locally instead of living
// in services/types.ts because that file is the shared surface for the
// auth/channels API and is kept read-only for this change.

import { AxiosError } from 'axios'
import apiClient from './api'
import type { BillingInterval, BillingStatus, Plan } from './types'

export class NoStripeCustomerError extends Error {
  constructor() {
    super('no_stripe_customer')
    this.name = 'NoStripeCustomerError'
  }
}

export interface UsagePeriod {
  period_start: string
  period_end: string
  comments_used: number
  quota: number
}

export interface UsageResponse {
  periods: UsagePeriod[]
}

export interface BillingTrial {
  active: boolean
  ends_at: string
  days_remaining: number
}

/** `BillingStatus` + the additive `trial` block (`/api/billing/status`). */
export interface BillingStatusWithTrial extends BillingStatus {
  trial?: BillingTrial
}

export async function status(): Promise<BillingStatusWithTrial> {
  const { data } = await apiClient.get<BillingStatusWithTrial>('/billing/status')
  return data
}

/** Recent per-period usage, newest first. Defaults to 6 periods (1..12). */
export async function getUsage(periods = 6): Promise<UsageResponse> {
  const { data } = await apiClient.get<UsageResponse>('/billing/usage', {
    params: { periods },
  })
  return data
}

export interface CheckoutSession {
  url: string
  session_id?: string
}

export async function checkout(
  plan: Plan | string,
  interval: BillingInterval,
): Promise<CheckoutSession> {
  const { data } = await apiClient.post<CheckoutSession>('/billing/checkout', {
    plan,
    interval,
  })
  return data
}

export interface PortalSession {
  url: string
}

export async function portal(): Promise<PortalSession> {
  try {
    const { data } = await apiClient.get<PortalSession>('/billing/portal')
    return data
  } catch (err) {
    if (err instanceof AxiosError && err.response?.status === 409) {
      throw new NoStripeCustomerError()
    }
    throw err
  }
}
