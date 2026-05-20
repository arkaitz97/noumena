<script setup lang="ts">
defineProps<{ message?: string | null }>()
</script>

<template>
  <p v-if="message" class="mt-1.5 text-xs text-rose-600 font-medium flex items-start gap-1">
    <svg class="w-3.5 h-3.5 mt-0.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
    <span>{{ message }}</span>
  </p>
</template>
