<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
import { toast } from '../../lib/toast'
import { validatePassword } from '../../lib/validation'
import PasswordInput from '../../components/forms/PasswordInput.vue'
import PrimaryButton from '../../components/forms/PrimaryButton.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const token = computed(() => (route.query.token as string | undefined) || '')
const password = ref('')
const confirm = ref('')
const loading = ref(false)
const error = ref<string | null>(null)

const validation = computed(() => validatePassword(password.value))
const passwordsMatch = computed(() => password.value.length > 0 && password.value === confirm.value)
const canSubmit = computed(() => validation.value.valid && passwordsMatch.value && token.value !== '')

async function handleSubmit() {
  if (!canSubmit.value) {
    if (!token.value) error.value = 'Missing reset token. Request a new email.'
    else if (!validation.value.valid)
      error.value = validation.value.firstError || 'Password does not meet requirements.'
    else if (!passwordsMatch.value) error.value = 'Passwords do not match.'
    return
  }
  loading.value = true
  error.value = null
  try {
    await auth.resetPassword(token.value, password.value)
    toast.success('Password updated. You can now sign in.')
    router.push('/login')
  } catch (err: any) {
    if (err?.response?.status === 410) {
      error.value = 'This reset link has expired. Please request a new one.'
    } else {
      error.value = err?.response?.data?.error || 'Could not update password.'
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8 relative overflow-hidden">
    <div class="absolute top-0 right-0 -mt-16 -mr-16 w-[28rem] h-[28rem] bg-gradient-to-br from-brand-500 to-brand-300 rounded-full blur-[80px] opacity-20 pointer-events-none" data-unslop-ignore></div>

    <div class="sm:mx-auto sm:w-full sm:max-w-md relative z-10">
      <div class="flex items-center justify-center gap-2 mb-8">
        <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-brand-400 to-brand-700 flex items-center justify-center shadow-card ring-1 ring-white/20">
          <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
        </div>
        <span class="text-xl font-bold tracking-tight text-zinc-900">Noumena</span>
      </div>

      <div class="bg-white py-10 px-8 shadow-card ring-1 ring-zinc-950/5 rounded-xl">
        <h2 class="text-2xl font-semibold tracking-tight text-zinc-900 mb-2">Set a new password</h2>
        <p class="text-zinc-500 text-sm mb-8">Choose a strong password you don't use anywhere else.</p>

        <form class="space-y-5" @submit.prevent="handleSubmit">
          <div v-if="error" class="bg-rose-50 p-4 rounded-xl text-rose-700 dark:bg-rose-400/10 dark:text-rose-300 text-sm border border-rose-100">
            {{ error }}
          </div>

          <PasswordInput
            v-model="password"
            label="New password"
            placeholder="••••••••"
            autocomplete="new-password"
            show-strength
            required
          />

          <PasswordInput
            v-model="confirm"
            label="Confirm password"
            placeholder="••••••••"
            autocomplete="new-password"
            :error="confirm.length > 0 && !passwordsMatch ? 'Passwords do not match' : null"
            required
          />

          <PrimaryButton type="submit" :loading="loading" :disabled="!canSubmit" full-width>
            <span v-if="loading">Updating…</span><span v-else>Update password</span>
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
