<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
    showText?: boolean
    textClass?: string
    badge?: string
    markOnly?: boolean
    containerClass?: string
    colorClass?: string
  }>(),
  {
    size: 'md',
    showText: false,
    textClass: 'font-bold tracking-tight text-zinc-900 dark:text-white',
    badge: undefined,
    markOnly: false,
    containerClass: '',
    colorClass: 'text-brand-500',
  }
)

const containerSizeMap = {
  xs: 'w-6 h-6 rounded-lg',
  sm: 'w-8 h-8 rounded-lg',
  md: 'w-9 h-9 rounded-xl',
  lg: 'w-11 h-11 rounded-xl',
  xl: 'w-14 h-14 rounded-xl',
}

const iconSizeMap = {
  xs: 'w-3.5 h-3.5',
  sm: 'w-4.5 h-4.5',
  md: 'w-5 h-5',
  lg: 'w-6 h-6',
  xl: 'w-8 h-8',
}

const textSizeMap = {
  xs: 'text-sm',
  sm: 'text-base',
  md: 'text-lg',
  lg: 'text-xl',
  xl: 'text-2xl',
}

const computedContainerClass = computed(() => {
  return [
    containerSizeMap[props.size],
    'bg-zinc-900 dark:bg-zinc-800',
    'flex items-center justify-center shadow-card ring-1 ring-white/10 dark:ring-white/15 shrink-0',
    props.containerClass,
  ].join(' ')
})

const computedIconClass = computed(() => {
  return [iconSizeMap[props.size], props.colorClass, 'shrink-0'].join(' ')
})

const computedTextClass = computed(() => {
  return [textSizeMap[props.size], props.textClass].join(' ')
})
</script>

<template>
  <div class="inline-flex items-center gap-2.5 select-none">
    <!-- Outer squircle badge -->
    <div v-if="!markOnly" :class="computedContainerClass" aria-hidden="true">
      <svg
        :class="computedIconClass"
        viewBox="0 0 48 48"
        fill="currentColor"
        xmlns="http://www.w3.org/2000/svg"
      >
        <rect x="8" y="8" width="6.5" height="32" rx="3.25" />
        <path
          d="M 17 11.25 C 17 9.45 18.45 8 20.25 8 H 20.25 C 22.05 8 23.5 9.45 23.5 11.25 L 31 36.75 C 31 38.55 29.55 40 27.75 40 H 27.75 C 25.95 40 24.5 38.55 24.5 36.75 Z"
        />
        <rect x="33.5" y="8" width="6.5" height="32" rx="3.25" />
      </svg>
    </div>

    <!-- Standalone Mark -->
    <svg
      v-else
      :class="computedIconClass"
      viewBox="0 0 48 48"
      fill="currentColor"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <rect x="8" y="8" width="6.5" height="32" rx="3.25" />
      <path
        d="M 17 11.25 C 17 9.45 18.45 8 20.25 8 H 20.25 C 22.05 8 23.5 9.45 23.5 11.25 L 31 36.75 C 31 38.55 29.55 40 27.75 40 H 27.75 C 25.95 40 24.5 38.55 24.5 36.75 Z"
      />
      <rect x="33.5" y="8" width="6.5" height="32" rx="3.25" />
    </svg>

    <!-- Wordmark & Badge -->
    <div v-if="showText" class="flex items-center gap-2">
      <span :class="computedTextClass">Noumena</span>
      <span
        v-if="badge"
        class="text-[10px] uppercase font-mono font-bold tracking-wider px-1.5 py-0.5 rounded bg-brand-500/10 text-brand-600 dark:text-brand-400 border border-brand-500/20"
      >
        {{ badge }}
      </span>
    </div>
  </div>
</template>
