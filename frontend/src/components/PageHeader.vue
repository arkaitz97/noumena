<script setup lang="ts">
// PageHeader — eyebrow + title + subtitle with the staggered rise-in, shared
// by all DashboardLayout screens. #actions holds right-aligned controls (md+).
defineProps<{
  eyebrow: string
  title: string
  subtitle?: string
}>()
</script>

<template>
  <div class="md:flex md:items-end md:justify-between">
    <div class="flex-1 min-w-0">
      <p class="eyebrow rise mb-2">{{ eyebrow }}</p>
      <h1 class="rise d2 text-display-sm font-bold text-zinc-900 dark:text-white">
        {{ title }}
      </h1>
      <p
        v-if="$slots.subtitle || subtitle"
        class="rise d3 mt-2 text-sm text-zinc-500 dark:text-zinc-400"
      >
        <slot name="subtitle">{{ subtitle }}</slot>
      </p>
    </div>
    <div v-if="$slots.actions" class="rise d3 mt-4 md:mt-0 flex-shrink-0">
      <slot name="actions" />
    </div>
  </div>
</template>
