<script setup lang="ts">
import { Check } from 'lucide-vue-next'
import type { BillingInterval, Plan } from '../services/types'
import SegmentedControl from './SegmentedControl.vue'

export interface PlanOption {
  id: Plan | string
  name: string
  monthlyUsd: number
  annualUsd: number
  features: string[]
  highlight?: boolean
}

const props = defineProps<{
  plans: PlanOption[]
  currentPlan: Plan | string | null
  interval: BillingInterval
}>()

const emit = defineEmits<{
  (e: 'select-plan', plan: Plan | string, interval: BillingInterval, prepopened: Window | null): void
  (e: 'update:interval', i: BillingInterval): void
}>()

function planRank(p: Plan | string): number {
  const order: Record<string, number> = { free: 0, creator: 1, studio: 2, agency: 3 }
  return order[p] ?? 0
}

function ctaLabel(p: PlanOption): string {
  if (p.id === props.currentPlan) return 'Current plan'
  if (props.currentPlan && planRank(p.id) < planRank(props.currentPlan)) return 'Downgrade'
  return p.id === 'free' ? 'Free' : 'Upgrade'
}

function isCurrent(p: PlanOption): boolean {
  return p.id === props.currentPlan
}

const annualSavingsPct = (p: PlanOption): number | null => {
  if (p.monthlyUsd <= 0) return null
  const yearlyAtMonthly = p.monthlyUsd * 12
  if (yearlyAtMonthly <= 0) return null
  const saving = (yearlyAtMonthly - p.annualUsd) / yearlyAtMonthly
  if (saving <= 0) return null
  return Math.round(saving * 100)
}

function priceDisplay(p: PlanOption): string {
  if (p.monthlyUsd === 0) return '$0'
  if (props.interval === 'annual') {
    return `$${Math.round(p.annualUsd / 12)}`
  }
  return `$${p.monthlyUsd}`
}

function onSelect(p: PlanOption): void {
  if (isCurrent(p)) return
  // Pre-open synchronously for popup-blocker safety. Free plan still gets
  // a checkout call (downgrade flow) but we can pass null too.
  let prepopened: Window | null = null
  if (p.monthlyUsd > 0 || (props.currentPlan && p.id !== props.currentPlan)) {
    prepopened = window.open('about:blank', '_blank', 'noopener')
  }
  emit('select-plan', p.id, props.interval, prepopened)
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h3 class="text-base font-semibold text-zinc-900 dark:text-zinc-100 tracking-tight">Plans</h3>
      <SegmentedControl
        :model-value="interval"
        :options="[
          { value: 'monthly', label: 'Monthly' },
          { value: 'annual', label: 'Annual' },
        ]"
        label="Billing interval"
        @update:model-value="(i) => emit('update:interval', i as BillingInterval)"
      />
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      <article
        v-for="p in plans"
        :key="p.id"
        :class="[
          'bg-white dark:bg-zinc-900 shadow-sm ring-1 rounded-xl p-5 flex flex-col gap-3',
          isCurrent(p) ? 'ring-zinc-900 dark:ring-zinc-100' : 'ring-zinc-950/5 dark:ring-zinc-100/10',
          p.highlight ? 'relative' : '',
        ]"
      >
        <div>
          <p class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 tracking-tight">{{ p.name }}</p>
          <p class="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 mt-1">
            {{ priceDisplay(p) }}<span class="text-sm font-medium text-zinc-500 dark:text-zinc-400">/mo</span>
          </p>
          <p v-if="interval === 'annual' && annualSavingsPct(p)" class="text-[11px] text-emerald-600 dark:text-emerald-400 font-medium mt-0.5">
            Save {{ annualSavingsPct(p) }}%
          </p>
        </div>
        <ul class="space-y-1.5 text-xs text-zinc-700 dark:text-zinc-300 flex-1">
          <li v-for="(f, i) in p.features" :key="i" class="flex items-start gap-1.5">
            <Check class="w-3.5 h-3.5 mt-0.5 text-emerald-600 dark:text-emerald-400 shrink-0" />
            <span>{{ f }}</span>
          </li>
        </ul>
        <button
          type="button"
          :disabled="isCurrent(p)"
          @click="onSelect(p)"
          :class="[
            'inline-flex justify-center items-center py-2 px-3 rounded-xl text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900',
            isCurrent(p)
              ? 'bg-zinc-100 dark:bg-zinc-800 text-zinc-500 dark:text-zinc-400 cursor-not-allowed'
              : 'bg-zinc-900 text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200',
          ]"
        >
          {{ ctaLabel(p) }}
        </button>
      </article>
    </div>
  </section>
</template>
