<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
import { validatePassword } from '../../lib/validation'
import { generateState, openOAuthPopup } from '../../lib/oauthPopup'
import { googleAuthUrl, loginWithGoogleRedirect } from '../../services/auth'
import TextInput from '../../components/forms/TextInput.vue'
import PasswordInput from '../../components/forms/PasswordInput.vue'
import PrimaryButton from '../../components/forms/PrimaryButton.vue'

const router = useRouter()
const auth = useAuthStore()

const email = ref('')
const password = ref('')
const error = ref<string | null>(null)
const loading = ref(false)
const googleLoading = ref(false)

const validation = computed(() => validatePassword(password.value))

async function handleSignup() {
  if (!validation.value.valid) {
    error.value = validation.value.firstError || 'Password does not meet requirements.'
    return
  }
  loading.value = true
  error.value = null
  try {
    await auth.register(email.value, password.value)
    router.push('/verify-email')
  } catch (err: any) {
    error.value = err?.response?.data?.error || 'Failed to create account.'
  } finally {
    loading.value = false
  }
}

async function handleGoogle() {
  if (googleLoading.value) return
  googleLoading.value = true
  error.value = null
  const state = generateState('google')
  try {
    const { auth_url } = await googleAuthUrl()
    try {
      const result = await openOAuthPopup('google', auth_url, { state })
      await auth.loginWithGoogle(result.code, result.state)
      if (!auth.isEmailVerified) router.push('/verify-email')
      else if (!auth.hasChannels) router.push('/onboarding')
      else router.push('/dashboard')
    } catch (popupErr: any) {
      if (popupErr?.message === 'popup_blocked') {
        error.value = 'Pop-up blocked. Please allow pop-ups and try again.'
      } else if (popupErr?.message === 'popup_closed') {
        error.value = 'Sign-in cancelled.'
      } else if (popupErr?.message === 'popup_timeout') {
        error.value = 'Sign-in timed out. Please try again.'
      } else {
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
      <div class="mx-auto w-full max-w-sm lg:max-w-md my-12">
        <div class="flex items-center gap-2 mb-10">
          <div class="w-8 h-8 rounded-lg bg-zinc-900 flex items-center justify-center shadow-md">
            <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <span class="text-xl font-bold tracking-tight text-zinc-900">Noumena</span>
        </div>

        <h1 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-2">Create an account</h1>
        <p class="text-zinc-500 text-base mb-8">Start understanding your audience in minutes.</p>

        <form class="space-y-5" @submit.prevent="handleSignup">
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

          <PasswordInput
            v-model="password"
            label="Password"
            placeholder="Create a password"
            autocomplete="new-password"
            show-strength
            required
          />

          <PrimaryButton
            type="submit"
            :loading="loading"
            :disabled="password.length > 0 && !validation.valid"
            full-width
          >
            <span v-if="loading">Creating account…</span><span v-else>Create account</span>
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
            <span v-if="googleLoading">Opening Google…</span><span v-else>Sign up with Google</span>
          </button>
        </div>

        <p class="mt-10 text-center text-sm text-zinc-600">
          Already have an account?
          <RouterLink to="/login" class="font-medium text-zinc-900 hover:underline">Sign in</RouterLink>
        </p>
      </div>
    </div>

    <div class="hidden lg:flex relative w-0 flex-1 bg-zinc-950 overflow-hidden items-center justify-center">
      <div class="absolute top-1/4 left-1/4 w-[40rem] h-[40rem] bg-brand-600 rounded-full mix-blend-screen filter blur-[120px] opacity-20" data-unslop-ignore></div>
      <div class="absolute bottom-1/4 right-1/4 w-[40rem] h-[40rem] bg-blue-600 rounded-full mix-blend-screen filter blur-[120px] opacity-20" data-unslop-ignore></div>
      <div class="relative z-10 max-w-lg p-10 rounded-[2rem] bg-white/5 border border-white/10 backdrop-blur-3xl shadow-2xl">
        <blockquote class="text-2xl font-medium text-zinc-100 leading-snug">
          "Using Noumena feels like having a data science team on my payroll."
        </blockquote>
      </div>
    </div>
  </div>
</template>
