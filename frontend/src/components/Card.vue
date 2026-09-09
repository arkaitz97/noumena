<script setup lang="ts">
// Shared surface card. Centralizes the repeated
// `bg-white … ring-1 rounded-xl` blocks so dark variants and radii stay
// consistent across views (design: fix-frontend-ux-a11y §2).

import { computed, type Component } from 'vue'

const props = withDefaults(
  defineProps<{
    tag?: string
    padding?: 'md' | 'lg' | 'none'
    as?: Component
  }>(),
  {
    padding: 'md',
  },
)

const Tag = computed(() => props.as ?? props.tag ?? 'div')
const padClass = computed(() =>
  props.padding === 'lg' ? 'p-6 @md:p-8' : props.padding === 'none' ? '' : 'p-5',
)
</script>

<template>
  <component
    :is="Tag"
    class="@container bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl"
    :class="padClass"
  >
    <slot />
  </component>
</template>
