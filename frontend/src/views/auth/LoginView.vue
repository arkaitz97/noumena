<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
import { generateState, openOAuthPopup } from '../../lib/oauthPopup'
import { googleAuthUrl, linkGoogle, loginWithGoogleRedirect } from '../../services/auth'
import TextInput from '../../components/forms/TextInput.vue'
import PrimaryButton from '../../components/forms/PrimaryButton.vue'

const router = useRouter()
const auth = useAuthStore()

const email = ref('')
const password = ref('')
const rememberMe = ref(true)
const error = ref<string | null>(null)
const loading = ref(false)
const googleLoading = ref(false)

// When a Google sign-in collides with an existing password account, the
// backend answers 409 link_required with a server-verified ID token. We
// stash it here, ask for the account password, and complete the link after
// login (the endpoint requires an authenticated session).
const PENDING_LINK_KEY = 'pending_google_link'

async function postLoginRedirect() {
  const next = (router.currentRoute.value.query.next as string | undefined) || ''
  if (!auth.isEmailVerified) return router.push('/verify-email')
  if (!auth.hasChannels) return router.push('/onboarding')
  return router.push(next || '/dashboard')
}

async function completePendingGoogleLink(): Promise<void> {
  const raw = sessionStorage.getItem(PENDING_LINK_KEY)
  if (!raw) return
  sessionStorage.removeItem(PENDING_LINK_KEY)
  try {
    const pending = JSON.parse(raw) as { email?: string; sub?: string; id_token?: string }
    if (pending.email && pending.sub && pending.id_token) {
      await linkGoogle(pending.email, pending.sub, pending.id_token)
    }
  } catch {
    // Non-fatal: the account is already signed in; the link can be retried
    // from a fresh Google sign-in.
    error.value = 'Google account link failed. Please try signing in with Google again.'
  }
}

async function handleLogin() {
  loading.value = true
  error.value = null
  try {
    await auth.login(email.value, password.value, rememberMe.value)
    await completePendingGoogleLink()
    await postLoginRedirect()
  } catch (err: any) {
    error.value = err?.response?.data?.error || 'Invalid email or password.'
  } finally {
    loading.value = false
  }
}

async function handleGoogle() {
  if (googleLoading.value) return
  googleLoading.value = true
  error.value = null

  // We must call window.open synchronously inside this click handler. We
  // pre-allocate state, then attempt a JSON `auth_url` lookup; if that
  // fails (current backend behavior), fall back to a top-level redirect.
  const state = generateState('google')
  try {
    const { auth_url } = await googleAuthUrl()
    try {
      const result = await openOAuthPopup('google', auth_url, { state })
      try {
        await auth.loginWithGoogle(result.code, result.state)
        await postLoginRedirect()
      } catch (linkErr: any) {
        // Google identity collides with an existing password account: the
        // backend returns the verified ID token; sign in with the password
        // to confirm the link.
        const data = linkErr?.response?.data
        if (linkErr?.response?.status === 409 && data?.error === 'link_required') {
          sessionStorage.setItem(
            PENDING_LINK_KEY,
            JSON.stringify({ email: data.email, sub: data.sub, id_token: data.id_token }),
          )
          email.value = data.email || email.value
          error.value =
            'An account with this email already exists. Enter your password to link your Google account.'
          return
        }
        throw linkErr
      }
    } catch (popupErr: any) {
      if (popupErr?.message === 'popup_blocked') {
        error.value = 'Pop-up blocked. Please allow pop-ups and try again.'
      } else if (popupErr?.message === 'popup_closed') {
        error.value = 'Sign-in cancelled.'
      } else if (popupErr?.message === 'popup_timeout') {
        error.value = 'Sign-in timed out. Please try again.'
      } else {
        // Backend may not yet expose the JSON exchange endpoint; fall back
        // to the top-level redirect flow as a graceful degradation.
        loginWithGoogleRedirect()
        return
      }
    }
  } finally {
    googleLoading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-white flex">
    <div class="flex-1 flex flex-col justify-center px-6 sm:px-12 lg:flex-none lg:w-1/2 xl:w-5/12 mx-auto lg:mx-0 relative z-10">
      <div class="mx-auto w-full max-w-sm lg:max-w-md">
        <div class="flex items-center gap-2 mb-10">
          <div class="w-8 h-8 rounded-lg bg-zinc-900 flex items-center justify-center shadow-md">
            <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <span class="text-xl font-bold tracking-tight text-zinc-900">Noumena</span>
        </div>

        <h1 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-2">Sign in to your account</h1>
        <p class="text-zinc-500 text-base mb-8">Welcome back! Please enter your details.</p>

        <form class="space-y-5" @submit.prevent="handleLogin">
          <div v-if="error" class="bg-rose-50 p-4 rounded-xl text-rose-700 text-sm border border-rose-100">
            {{ error }}
          </div>

          <TextInput
            v-model="email"
            label="Email"
            type="email"
            autocomplete="email"
            placeholder="you@company.com"
            required
          />

          <div>
            <label for="login-password" class="block text-sm font-medium text-zinc-700 mb-1.5">Password</label>
            <input
              id="login-password"
              type="password"
              autocomplete="current-password"
              required
              v-model="password"
              class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 sm:text-sm transition-colors bg-zinc-50/50 focus:bg-white"
              placeholder="••••••••"
            />
          </div>

          <div class="flex items-center justify-between pt-1">
            <label class="flex items-center cursor-pointer">
              <input type="checkbox" v-model="rememberMe" class="h-4 w-4 text-zinc-900 focus:ring-zinc-900 border-zinc-300 rounded" />
              <span class="ml-2 block text-sm text-zinc-600">Remember me for 30 days</span>
            </label>
            <RouterLink to="/forgot-password" class="text-sm font-medium text-zinc-900 hover:text-zinc-700 transition-colors">
              Forgot password?
            </RouterLink>
          </div>

          <PrimaryButton type="submit" :loading="loading" full-width>
            <span v-if="loading">Signing in…</span><span v-else>Sign in</span>
          </PrimaryButton>
        </form>

        <div class="mt-8">
          <div class="relative">
            <div class="absolute inset-0 flex items-center"><div class="w-full border-t border-zinc-200"></div></div>
            <div class="relative flex justify-center text-sm">
              <span class="px-3 bg-white text-zinc-500 font-medium">Or</span>
            </div>
          </div>

          <button
            type="button"
            @click="handleGoogle"
            :disabled="googleLoading"
            class="mt-8 w-full flex justify-center items-center py-3.5 px-4 rounded-xl border border-zinc-200 bg-white text-sm font-medium text-zinc-700 shadow-sm hover:border-zinc-300 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:ring-offset-2 transition-all duration-200 disabled:opacity-60"
          >
            <svg class="w-5 h-5 mr-3" viewBox="0 0 488 512" aria-hidden="true">
              <path fill="currentColor" d="M488 261.8C488 403.3 391.1 504 248 504 110.8 504 0 393.2 0 256S110.8 8 248 8c66.8 0 123 24.5 166.3 64.9l-67.5 64.9C258.5 52.6 94.3 116.6 94.3 256c0 86.5 69.1 156.6 153.7 156.6 98.2 0 135-70.4 140.8-106.9H248v-85.3h236.1c2.3 12.7 3.9 24.9 3.9 41.4z" />
            </svg>
            <span v-if="googleLoading">Opening Google…</span><span v-else>Sign in with Google</span>
          </button>
        </div>

        <p class="mt-10 text-center text-sm text-zinc-600">
          Don't have an account?
          <RouterLink to="/signup" class="font-medium text-zinc-900 hover:underline">Create one</RouterLink>
        </p>
      </div>
    </div>

    <div class="hidden lg:flex relative w-0 flex-1 bg-zinc-950 overflow-hidden items-center justify-center">
      <div class="absolute top-1/4 left-1/4 w-[40rem] h-[40rem] bg-brand-600 rounded-full mix-blend-screen filter blur-[120px] opacity-20" data-unslop-ignore></div>
      <div class="absolute bottom-1/4 right-1/4 w-[40rem] h-[40rem] bg-blue-600 rounded-full mix-blend-screen filter blur-[120px] opacity-20" data-unslop-ignore></div>
      <div class="relative z-10 max-w-lg p-10 rounded-[2rem] bg-white/5 border border-white/10 backdrop-blur-3xl shadow-2xl">
        <blockquote class="text-2xl font-medium text-zinc-100 leading-snug">
          "Noumena turned thousands of comments into the only weekly report I now actually read."
        </blockquote>
      </div>
    </div>
  </div>
</template>
