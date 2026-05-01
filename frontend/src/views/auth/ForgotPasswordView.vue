<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'

const email = ref('')
const message = ref('')
const error = ref('')
const loading = ref(false)

const handleForgot = async () => {
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    // Mock API call
    setTimeout(() => {
      message.value = 'If an account exists with this email, a reset link has been sent.'
      loading.value = false
    }, 1000)
  } catch (err: any) {
    error.value = 'Failed to request password reset.'
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-white flex">
    <!-- Left Panel -->
    <div class="flex-1 flex flex-col justify-center px-6 sm:px-12 lg:flex-none lg:w-1/2 xl:w-5/12 mx-auto lg:mx-0 relative z-10">
      <div class="mx-auto w-full max-w-sm lg:max-w-md">
        <!-- Brand/Logo -->
        <div class="flex items-center gap-2 mb-10">
          <div class="w-8 h-8 rounded-lg bg-zinc-900 flex items-center justify-center shadow-md">
            <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <span class="text-xl font-bold tracking-tight text-zinc-900">Noumena</span>
        </div>

        <h1 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-2">Reset password</h1>
        <p class="text-zinc-500 text-base mb-8">
          Enter your email address and we'll send you a link to reset your password.
        </p>

        <form class="space-y-5" @submit.prevent="handleForgot">
          <div v-if="error" class="bg-red-50 p-4 rounded-xl text-red-700 text-sm border border-red-100">
            {{ error }}
          </div>
          <div v-if="message" class="bg-emerald-50 p-4 rounded-xl text-emerald-700 text-sm border border-emerald-100">
            {{ message }}
          </div>
          
          <div>
            <label for="email" class="block text-sm font-medium text-zinc-700 mb-1.5"> Email </label>
            <input id="email" name="email" type="email" autocomplete="email" required v-model="email" class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 sm:text-sm transition-colors bg-zinc-50/50 focus:bg-white" placeholder="you@company.com" />
          </div>

          <div class="pt-2">
            <button type="submit" :disabled="loading" class="w-full flex justify-center items-center py-3.5 px-4 border border-transparent rounded-xl shadow-sm text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 disabled:opacity-50 transition-all duration-200">
              <span v-if="loading">Sending link...</span>
              <span v-else>Send reset link</span>
            </button>
          </div>
        </form>

        <p class="mt-10 text-center text-sm text-zinc-600">
          Remembered your password?
          <RouterLink to="/login" class="font-medium text-zinc-900 hover:underline">
            Sign in
          </RouterLink>
        </p>
      </div>
    </div>

    <!-- Right Panel (Decorative) -->
    <div class="hidden lg:flex relative w-0 flex-1 bg-zinc-950 overflow-hidden items-center justify-center">
      <!-- Ambient glow circles -->
      <div class="absolute top-[-10%] left-[-10%] w-[40rem] h-[40rem] bg-indigo-500 rounded-full mix-blend-screen filter blur-[120px] opacity-20 animate-blob"></div>
      <div class="absolute bottom-[-10%] right-[-10%] w-[40rem] h-[40rem] bg-rose-500 rounded-full mix-blend-screen filter blur-[120px] opacity-10 animate-blob animation-delay-2000"></div>

      <!-- Conceptual Lock Graphic over dark blurred background -->
      <div class="relative z-10 flex flex-col items-center">
        <div class="w-24 h-24 bg-white/5 rounded-3xl border border-white/10 backdrop-blur-xl flex items-center justify-center shadow-2xl mb-8">
          <svg class="w-10 h-10 text-white/80" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
          </svg>
        </div>
        <div class="text-center">
          <h3 class="text-xl font-medium text-white mb-2 tracking-tight">Secure Account Recovery</h3>
          <p class="text-zinc-400 max-w-sm">We'll help you get back into your account securely without losing your data.</p>
        </div>
      </div>
      
      <!-- Subtle dot grid overlay for texture -->
      <div class="absolute inset-0 bg-[url('data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMjQiIGhlaWdodD0iMjQiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyI+PGNpcmNsZSBjeD0iMSIgY3k9IjEiIHI9IjEiIGZpbGw9InJnYmEoMjU1LCAyNTUsIDI1NSwgMC4wNSkiLz48L3N2Zz4=')] opacity-60"></div>
    </div>
  </div>
</template>

<style scoped>
@keyframes blob {
  0% { transform: translate(0px, 0px) scale(1); }
  33% { transform: translate(30px, -50px) scale(1.1); }
  66% { transform: translate(-20px, 20px) scale(0.9); }
  100% { transform: translate(0px, 0px) scale(1); }
}
.animate-blob {
  animation: blob 7s infinite alternate;
}
.animation-delay-2000 {
  animation-delay: 2s;
}
</style>
