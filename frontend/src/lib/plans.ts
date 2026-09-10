// Presentational plan catalog (names/pricing/features). The backend
// (`internal/billing/plans.go`) is the source of truth for plan validity;
// this drives the Billing picker and marketing copy so names never drift
// between Billing, Landing, and FAQ.

import type { Plan } from '../services/types'

export interface PlanOption {
  id: Plan | string
  name: string
  monthlyUsd: number
  annualUsd: number
  features: string[]
  highlight?: boolean
}

export const PLANS: PlanOption[] = [
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
    features: ['Unlimited channels', '1M comments / mo', 'Team seats', 'Dedicated CSM'],
  },
]
