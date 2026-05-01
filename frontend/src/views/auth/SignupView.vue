<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter, RouterLink } from 'vue-router'

const router = useRouter()
const email = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

const isPasswordValid = computed(() => {
  const minLength = password.value.length >= 8
  const hasUpper = /[A-Z]/.test(password.value)
  const hasNumber = /[0-9]/.test(password.value)
  return minLength && hasUpper && hasNumber
})

const handleSignup = async () => {
  if (!isPasswordValid.value) {
    error.value = 'Password does not meet requirements.'
    return
  }
  
  loading.value = true
  error.value = ''
  try {
    // TODO: Connect to actual API
    // await apiClient.post('/auth/signup', { email: email.value, password: password.value })
    router.push('/verify-email')
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Failed to create account.'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-white flex">
    <!-- Left Panel -->
    <div class="flex-1 flex flex-col justify-center px-6 sm:px-12 lg:flex-none lg:w-1/2 xl:w-5/12 mx-auto lg:mx-0 relative z-10">
      <div class="mx-auto w-full max-w-sm lg:max-w-md my-12">
        <!-- Brand/Logo -->
        <div class="flex items-center gap-2 mb-10">
          <div class="w-8 h-8 rounded-lg bg-zinc-900 flex items-center justify-center shadow-md">
            <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <span class="text-xl font-bold tracking-tight text-zinc-900">Noumena</span>
        </div>

        <h1 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-2">Create an account</h1>
        <p class="text-zinc-500 text-base mb-8">
          Start understanding your audience in minutes.
        </p>

        <form class="space-y-5" @submit.prevent="handleSignup">
          <div v-if="error" class="bg-red-50 p-4 rounded-xl text-red-700 text-sm border border-red-100">
            {{ error }}
          </div>
          
          <div>
            <label for="email" class="block text-sm font-medium text-zinc-700 mb-1.5"> Email </label>
            <input id="email" name="email" type="email" autocomplete="email" required v-model="email" class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 sm:text-sm transition-colors bg-zinc-50/50 focus:bg-white" placeholder="you@company.com" />
          </div>

          <div>
            <label for="password" class="block text-sm font-medium text-zinc-700 mb-1.5"> Password </label>
            <input id="password" name="password" type="password" autocomplete="new-password" required v-model="password" class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 sm:text-sm transition-colors bg-zinc-50/50 focus:bg-white" placeholder="Create a password" />
            
            <div class="mt-3 text-xs text-zinc-500 space-y-1.5 font-medium">
              <p :class="{'text-emerald-600': password.length >= 8}" class="flex items-center transition-colors">
                <svg v-if="password.length >= 8" class="w-3.5 h-3.5 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
                <span v-else class="w-1.5 h-1.5 rounded-full bg-zinc-300 mr-2.5 ml-1 inline-block"></span>
                At least 8 characters
              </p>
              <p :class="{'text-emerald-600': /[A-Z]/.test(password)}" class="flex items-center transition-colors">
                <svg v-if="/[A-Z]/.test(password)" class="w-3.5 h-3.5 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
                <span v-else class="w-1.5 h-1.5 rounded-full bg-zinc-300 mr-2.5 ml-1 inline-block"></span>
                One uppercase letter
              </p>
              <p :class="{'text-emerald-600': /[0-9]/.test(password)}" class="flex items-center transition-colors">
                <svg v-if="/[0-9]/.test(password)" class="w-3.5 h-3.5 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
                <span v-else class="w-1.5 h-1.5 rounded-full bg-zinc-300 mr-2.5 ml-1 inline-block"></span>
                One number
              </p>
            </div>
          </div>

          <div class="pt-2">
            <button type="submit" :disabled="loading || (password.length > 0 && !isPasswordValid)" class="w-full flex justify-center items-center py-3.5 px-4 border border-transparent rounded-xl shadow-sm text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200">
              <span v-if="loading">Creating account...</span>
              <span v-else>Create account</span>
            </button>
          </div>
        </form>

        <div class="mt-8">
          <div class="relative">
            <div class="absolute inset-0 flex items-center">
              <div class="w-full border-t border-zinc-200"></div>
            </div>
            <div class="relative flex justify-center text-sm">
              <span class="px-3 bg-white text-zinc-500 font-medium">Or</span>
            </div>
          </div>

          <div class="mt-8">
            <a href="/api/auth/google/login" class="w-full flex justify-center items-center py-3.5 px-4 rounded-xl border border-zinc-200 bg-white text-sm font-medium text-zinc-700 shadow-[0_1px_2px_rgba(0,0,0,0.05)] hover:border-zinc-300 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:ring-offset-2 transition-all duration-200">
               <svg class="w-5 h-5 mr-3" aria-hidden="true" focusable="false" data-prefix="fab" data-icon="google" role="img" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 488 512"><path fill="currentColor" d="M488 261.8C488 403.3 391.1 504 248 504 110.8 504 0 393.2 0 256S110.8 8 248 8c66.8 0 123 24.5 166.3 64.9l-67.5 64.9C258.5 52.6 94.3 116.6 94.3 256c0 86.5 69.1 156.6 153.7 156.6 98.2 0 135-70.4 140.8-106.9H248v-85.3h236.1c2.3 12.7 3.9 24.9 3.9 41.4z"></path></svg>
               Sign up with Google
            </a>
          </div>
        </div>

        <p class="mt-10 text-center text-sm text-zinc-600">
          Already have an account?
          <RouterLink to="/login" class="font-medium text-zinc-900 hover:underline">
            Sign in
          </RouterLink>
        </p>
      </div>
    </div>

    <!-- Right Panel (Decorative) -->
    <div class="hidden lg:flex relative w-0 flex-1 bg-zinc-950 overflow-hidden items-center justify-center">
      <!-- Ambient glow circles -->
      <div class="absolute top-1/4 left-1/4 w-[40rem] h-[40rem] bg-indigo-600 rounded-full mix-blend-screen filter blur-[120px] opacity-20 animate-blob"></div>
      <div class="absolute bottom-1/4 right-1/4 w-[40rem] h-[40rem] bg-blue-600 rounded-full mix-blend-screen filter blur-[120px] opacity-20 animate-blob animation-delay-2000"></div>

      <!-- Glassmorphism Card -->
      <div class="relative z-10 max-w-lg p-10 rounded-[2rem] bg-white/5 border border-white/10 backdrop-blur-3xl shadow-2xl">
        <div class="inline-flex items-center px-3 py-1 rounded-full bg-white/10 border border-white/10 mb-8">
          <svg class="w-4 h-4 text-indigo-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
          <span class="text-xs font-medium text-zinc-300 uppercase tracking-widest">Connect & Convert</span>
        </div>
        
        <blockquote class="text-2xl font-medium text-zinc-100 leading-snug mb-8">
          "Using Noumena feels like having a data science team on my payroll. It finds the signals in the noise effortlessly."
        </blockquote>
        
        <div class="flex items-center">
          <div class="h-12 w-12 rounded-full bg-gradient-to-br from-indigo-400 to-blue-600 flex items-center justify-center text-white font-semibold text-lg ring-2 ring-white/20">
            D
          </div>
          <div class="ml-4">
            <div class="text-base font-semibold text-white">David Miller</div>
            <div class="text-sm text-zinc-400 font-medium">Independent Creator</div>
          </div>
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
