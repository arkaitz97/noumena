// Reactive rate-limit state (fix-frontend-plumbing). The axios interceptor
// (services/api.ts) sets this on 429; RateLimitBanner consumes it. Replaces
// the dead `window.__noumena_rate_limit` global.

import { ref } from 'vue'
import { defineStore } from 'pinia'

export const useRateLimitStore = defineStore('rateLimit', () => {
  const visible = ref(false)
  const retryAfterSeconds = ref(0)

  function show(retryAfter: number): void {
    retryAfterSeconds.value = Math.max(1, Math.round(retryAfter))
    visible.value = true
  }

  function dismiss(): void {
    visible.value = false
  }

  return { visible, retryAfterSeconds, show, dismiss }
})