<script setup lang="ts">
defineProps<{
  type?: 'button' | 'submit' | 'reset'
  loading?: boolean
  disabled?: boolean
  fullWidth?: boolean
}>()
</script>

<template>
  <button
    :type="type || 'button'"
    :disabled="disabled || loading"
    class="relative inline-flex justify-center items-center gap-2 py-3 px-5 rounded-xl text-sm font-semibold text-white bg-zinc-900 shadow-card transition-all duration-200 ease-snappy hover:bg-zinc-700 hover:shadow-pop active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 disabled:opacity-50 disabled:pointer-events-none dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200"
    :class="fullWidth ? 'w-full py-3.5' : ''"
  >
    <svg
      v-if="loading"
      class="animate-spin -ml-1 mr-1 h-4 w-4"
      fill="none"
      viewBox="0 0 24 24"
    >
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path
        class="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      ></path>
    </svg>
    <slot />
  </button>
</template>
