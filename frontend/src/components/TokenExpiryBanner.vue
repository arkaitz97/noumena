<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { AlertTriangle, X } from 'lucide-vue-next'
import type { TokenExpiryChannel } from '../services/tokenExpiry'

const props = defineProps<{
  visible: boolean
  channels: TokenExpiryChannel[]
}>()

const emit = defineEmits<{ (e: 'dismiss'): void }>()

const heading = computed(() =>
  props.channels.length === 1
    ? 'Channel token expires soon'
    : `${props.channels.length} channels expire soon`,
)

function dayLabel(days: number): string {
  return days === 1 ? 'day' : 'days'
}
</script>

<template>
  <div
    v-if="visible && channels.length > 0"
    class="bg-amber-50 dark:bg-amber-400/10 ring-1 ring-amber-500/20 dark:ring-amber-400/30 rounded-xl px-4 py-3 flex items-center gap-3"
    role="alert"
  >
    <AlertTriangle class="w-5 h-5 text-amber-500 flex-shrink-0" />
    <div class="flex-1 min-w-0">
      <p class="text-sm font-medium text-amber-800 dark:text-amber-200">{{ heading }}</p>
      <ul class="mt-0.5 space-y-0.5 text-sm text-amber-800 dark:text-amber-200">
        <li v-for="channel in channels" :key="channel.channel_id" class="truncate">
          Channel &quot;{{ channel.title }}&quot; token expires in
          {{ channel.days_remaining }} {{ dayLabel(channel.days_remaining) }}.
        </li>
      </ul>
    </div>
    <RouterLink
      to="/settings/channels"
      class="inline-flex items-center justify-center px-3.5 py-2 rounded-lg text-sm font-medium text-amber-900 dark:text-amber-200 bg-amber-100 dark:bg-amber-400/20 hover:bg-amber-200 dark:hover:bg-amber-400/30 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-500 transition-all duration-200 ease-snappy active:scale-[0.98] flex-shrink-0"
    >
      Reconnect now
    </RouterLink>
    <button
      type="button"
      aria-label="Dismiss"
      @click="emit('dismiss')"
      class="flex items-center justify-center h-11 w-11 rounded-lg text-amber-500 dark:text-amber-400 hover:text-amber-700 dark:hover:text-amber-200 hover:bg-amber-100 dark:hover:bg-amber-400/20 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-500 transition-all duration-200 active:scale-95 flex-shrink-0"
    >
      <X class="w-4 h-4" />
    </button>
  </div>
</template>
