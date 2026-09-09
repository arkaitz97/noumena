<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

// OAuthCallbackView — mounted in the popup after the OAuth provider
// redirects back to /oauth/callback?code=...&state=...
//
// Responsibilities:
//   1. Extract `code` and `state` from the URL.
//   2. Determine which provider this callback belongs to from the
//      sessionStorage key written by `lib/oauthPopup#generateState`.
//   3. postMessage the payload to `window.opener` and close the window.
//   4. Show a friendly error if there is no opener (user landed here
//      directly, e.g. by reload).

const route = useRoute()
const error = ref<string | null>(null)
const closing = ref(false)

function detectProvider(state: string): 'google' | 'youtube' | null {
  if (sessionStorage.getItem('oauth_state_google') === state) return 'google'
  if (sessionStorage.getItem('oauth_state_youtube') === state) return 'youtube'
  return null
}

onMounted(() => {
  const code = (route.query.code as string | undefined) || ''
  const state = (route.query.state as string | undefined) || ''
  const oauthError = (route.query.error as string | undefined) || ''

  if (oauthError) {
    error.value = `Authorization failed: ${oauthError}`
    return
  }

  if (!code || !state) {
    error.value = 'Missing authorization parameters.'
    return
  }

  if (!window.opener || window.opener === window) {
    error.value = 'This page should open in a pop-up. Please return to the original tab and try again.'
    return
  }

  const provider = detectProvider(state)
  if (!provider) {
    error.value = 'Could not determine the OAuth provider. Please try again.'
    return
  }

  try {
    window.opener.postMessage(
      { type: 'oauth-code', provider, code, state },
      window.location.origin,
    )
    closing.value = true
    setTimeout(() => window.close(), 50)
  } catch (e) {
    error.value = 'Could not communicate with the original window.'
  }
})
</script>

<template>
  <div class="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex items-center justify-center px-4">
    <div class="bg-white dark:bg-zinc-900 shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-8 max-w-md w-full text-center">
      <template v-if="error">
        <div class="mx-auto h-12 w-12 rounded-full bg-rose-50 flex items-center justify-center text-rose-600 mb-4 ring-1 ring-rose-600/20">
          <svg class="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <h2 class="text-lg font-semibold text-zinc-900 mb-2 tracking-tight">Sign-in error</h2>
        <p class="text-sm text-zinc-500 leading-relaxed">{{ error }}</p>
      </template>
      <template v-else>
        <div class="mx-auto h-12 w-12 rounded-full bg-emerald-50 flex items-center justify-center text-emerald-600 mb-4 ring-1 ring-emerald-600/20">
          <svg class="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <h2 class="text-lg font-semibold text-zinc-900 mb-2 tracking-tight">Almost there…</h2>
        <p class="text-sm text-zinc-500">{{ closing ? 'Closing window…' : 'Finalizing authorization…' }}</p>
      </template>
    </div>
  </div>
</template>
