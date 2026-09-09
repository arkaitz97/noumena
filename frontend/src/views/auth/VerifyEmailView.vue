<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
import { toast } from '../../lib/toast'
import SecondaryButton from '../../components/forms/SecondaryButton.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

type Phase = 'idle' | 'verifying' | 'success' | 'invalid'
const phase = ref<Phase>('idle')
const resending = ref(false)
const resendError = ref<string | null>(null)
const resentAt = ref<number | null>(null)

const userEmail = computed(() => auth.user?.email || 'your inbox')

onMounted(async () => {
  const token = (route.query.token as string | undefined) || ''
  if (!token) return
  phase.value = 'verifying'
  try {
    await auth.verifyEmail(token)
    phase.value = 'success'
    setTimeout(() => router.push('/onboarding'), 1200)
  } catch {
    phase.value = 'invalid'
  }
})

async function handleResend() {
  resending.value = true
  resendError.value = null
  try {
    await auth.resendVerification()
    resentAt.value = Date.now()
    toast.success('Verification email sent.')
  } catch (err: any) {
    if (err?.response?.status === 429) {
      resendError.value = 'You\'ve requested too many emails. Please wait an hour and try again.'
    } else {
      resendError.value = err?.response?.data?.error || 'Could not send email. Try again later.'
    }
  } finally {
    resending.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8 relative overflow-hidden">
    <div class="absolute top-[-10%] left-[10%] w-[30rem] h-[30rem] bg-brand-200 rounded-full mix-blend-multiply filter blur-[100px] opacity-40" data-unslop-ignore></div>
    <div class="absolute bottom-[-10%] right-[10%] w-[30rem] h-[30rem] bg-blue-100 rounded-full mix-blend-multiply filter blur-[100px] opacity-40" data-unslop-ignore></div>

    <div class="sm:mx-auto sm:w-full sm:max-w-md relative z-10 text-center">
      <div class="flex items-center justify-center gap-2 mb-10">
        <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-brand-400 to-brand-700 flex items-center justify-center shadow-card ring-1 ring-white/20">
          <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
        </div>
        <span class="text-xl font-bold tracking-tight text-zinc-900">Noumena</span>
      </div>

      <!-- Verifying -->
      <template v-if="phase === 'verifying'">
        <h2 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-4">Verifying your email…</h2>
        <p class="text-zinc-500">Hold tight, this only takes a moment.</p>
      </template>

      <!-- Success -->
      <template v-else-if="phase === 'success'">
        <div class="flex justify-center mb-6">
          <div class="h-20 w-20 bg-emerald-50 dark:bg-emerald-400/10 rounded-xl flex items-center justify-center ring-1 ring-emerald-600/20">
            <svg class="h-10 w-10 text-emerald-600 dark:text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
            </svg>
          </div>
        </div>
        <h2 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-3">Email verified</h2>
        <p class="text-zinc-500">Redirecting you to onboarding…</p>
      </template>

      <!-- Invalid token -->
      <template v-else-if="phase === 'invalid'">
        <div class="flex justify-center mb-6">
          <div class="h-20 w-20 bg-rose-50 dark:bg-rose-400/10 rounded-xl flex items-center justify-center ring-1 ring-rose-600/20">
            <svg class="h-10 w-10 text-rose-600 dark:text-rose-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
        </div>
        <h2 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-3">Link expired or invalid</h2>
        <p class="text-zinc-500 mb-8">Request a new verification email below.</p>
      </template>

      <!-- Idle (no ?token) — show inbox prompt + resend -->
      <template v-else>
        <div class="flex justify-center mb-6">
          <div class="h-20 w-20 bg-white rounded-xl flex items-center justify-center shadow-card border border-zinc-100 ring-4 ring-zinc-50">
            <svg class="h-10 w-10 text-brand-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
          </div>
        </div>
        <h2 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-3">Check your inbox</h2>
        <p class="text-zinc-500 mb-10">
          We've sent a verification link to <br /><span class="font-medium text-zinc-900">{{ userEmail }}</span>.
        </p>
      </template>

      <!-- Resend card (always visible except during success) -->
      <div v-if="phase !== 'success'" class="bg-white py-8 px-6 shadow-card rounded-[2rem] border border-zinc-100/80 mx-4 sm:mx-0">
        <p v-if="resendError" class="text-sm text-rose-600 dark:text-rose-400 mb-4 font-medium">{{ resendError }}</p>
        <p v-else class="text-sm text-zinc-500 mb-6 font-medium">
          Didn't receive the email? Check your spam folder or request a new one.
        </p>

        <SecondaryButton :loading="resending" :disabled="resending" full-width @click="handleResend">
          <span v-if="resending">Sending…</span>
          <span v-else-if="resentAt" class="text-emerald-600 font-medium">Email sent — check your inbox</span>
          <span v-else>Resend verification link</span>
        </SecondaryButton>

        <div class="mt-8 pt-6 border-t border-zinc-100 text-sm">
          <RouterLink to="/login" class="font-medium text-zinc-500 hover:text-zinc-900 transition-colors inline-flex items-center">
            <svg class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18" />
            </svg>
            Back to sign in
          </RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>
