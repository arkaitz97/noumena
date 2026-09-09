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
    class="bg-amber-50 border border-amber-200 rounded-xl px-4 py-3 flex items-center gap-3" data-unslop-ignore
    role="alert"
  >
    <AlertTriangle class="w-5 h-5 text-amber-500 flex-shrink-0" />
    <div class="flex-1 min-w-0">
      <p class="text-sm font-medium text-amber-800">{{ heading }}</p>
      <ul class="mt-0.5 space-y-0.5 text-sm text-amber-800">
        <li v-for="channel in channels" :key="channel.channel_id" class="truncate">
          Channel &quot;{{ channel.title }}&quot; token expires in
          {{ channel.days_remaining }} {{ dayLabel(channel.days_remaining) }}.
        </li>
      </ul>
    </div>
    <RouterLink
      to="/settings/channels"
      class="inline-flex items-center justify-center px-3.5 py-2 rounded-lg text-sm font-medium text-amber-900 bg-amber-100 hover:bg-amber-200 focus:outline-none focus:ring-2 focus:ring-amber-500 transition-colors flex-shrink-0" data-unslop-ignore
    >
      Reconnect now
    </RouterLink>
    <button
      type="button"
      aria-label="Dismiss"
      @click="emit('dismiss')"
      class="flex items-center justify-center h-11 w-11 rounded-lg text-amber-500 hover:text-amber-700 hover:bg-amber-100 focus:outline-none focus:ring-2 focus:ring-amber-500 flex-shrink-0" data-unslop-ignore
    >
      <X class="w-4 h-4" />
    </button>
  </div>
</template>
