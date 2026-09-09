<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
import TextInput from '../../components/forms/TextInput.vue'
import PrimaryButton from '../../components/forms/PrimaryButton.vue'
import AppLogo from '../../components/AppLogo.vue'

const auth = useAuthStore()
const email = ref('')
const loading = ref(false)
const submitted = ref(false)
const error = ref<string | null>(null)

const genericMessage =
  'If an account exists with this email, we\'ve sent a password reset link. Please check your inbox.'

async function handleSubmit() {
  loading.value = true
  error.value = null
  try {
    await auth.forgotPassword(email.value)
    submitted.value = true
  } catch (err: any) {
    if (err?.response?.status === 429) {
      error.value = 'Too many requests. Please wait a while and try again.'
    } else {
      // Backend always returns 200 with the generic message, but defend
      // against unexpected errors anyway.
      submitted.value = true
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8 relative overflow-hidden">
    <div class="absolute top-0 right-0 -mt-16 -mr-16 w-[28rem] h-[28rem] bg-gradient-to-br from-brand-500 to-brand-300 rounded-full blur-[80px] opacity-20 pointer-events-none"></div>

    <div class="sm:mx-auto sm:w-full sm:max-w-md relative z-10">
      <div class="flex items-center justify-center mb-8">
        <AppLogo size="lg" show-text />
      </div>

      <div class="bg-white py-10 px-8 shadow-card ring-1 ring-zinc-950/5 rounded-xl">
        <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-2">Reset your password</h2>
        <p class="text-zinc-500 text-sm mb-8">
          Enter the email associated with your account and we'll send you a link to reset your password.
        </p>

        <div v-if="submitted" class="bg-emerald-50 text-emerald-800 dark:bg-emerald-400/10 dark:text-emerald-300 ring-1 ring-emerald-600/20 rounded-xl p-4 text-sm">
          {{ genericMessage }}
        </div>

        <form v-else class="space-y-5" @submit.prevent="handleSubmit">
          <div v-if="error" class="bg-rose-50 p-4 rounded-xl text-rose-700 dark:bg-rose-400/10 dark:text-rose-300 text-sm border border-rose-100">
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

          <PrimaryButton type="submit" :loading="loading" full-width>
            <span v-if="loading">Sending…</span><span v-else>Send reset link</span>
          </PrimaryButton>
        </form>

        <div class="mt-8 text-sm text-center">
          <RouterLink to="/login" class="font-medium text-zinc-500 hover:text-zinc-900 transition-colors">
            ← Back to sign in
          </RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>
