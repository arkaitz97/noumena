// /api/billing/* surface — status, checkout session, customer portal.
//
// `portal()` returns HTTP 409 when the caller has no Stripe customer
// (free tier without a prior payment). We rewrap that into a typed error
// so the store can swap the button for the "available after first paid
// plan" hint without inspecting Axios internals.

import { AxiosError } from 'axios'
import apiClient from './api'
import type { BillingInterval, BillingStatus, Plan } from './types'

export class NoStripeCustomerError extends Error {
  constructor() {
    super('no_stripe_customer')
    this.name = 'NoStripeCustomerError'
  }
}

export async function status(): Promise<BillingStatus> {
  const { data } = await apiClient.get<BillingStatus>('/billing/status')
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
