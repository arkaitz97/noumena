<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const password = ref('')
const confirmPassword = ref('')
const error = ref('')
const loading = ref(false)

const isPasswordValid = computed(() => {
  const minLength = password.value.length >= 8
  const hasUpper = /[A-Z]/.test(password.value)
  const hasNumber = /[0-9]/.test(password.value)
  return minLength && hasUpper && hasNumber
})

const handleReset = async () => {
  if (!isPasswordValid.value) {
    error.value = 'Password does not meet requirements.'
    return
  }
  if (password.value !== confirmPassword.value) {
    error.value = 'Passwords do not match.'
    return
  }
  
  loading.value = true
  error.value = ''
  try {
    // Mock API call
    setTimeout(() => {
      router.push('/login?reset=success')
    }, 1000)
  } catch (err: any) {
    error.value = 'Failed to reset password.'
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

        <h1 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-2">Set new password</h1>
        <p class="text-zinc-500 text-base mb-8">
          Please enter your new password below.
        </p>

        <form class="space-y-5" @submit.prevent="handleReset">
          <div v-if="error" class="bg-red-50 p-4 rounded-xl text-red-700 text-sm border border-red-100">
            {{ error }}
          </div>

          <div>
            <label for="password" class="block text-sm font-medium text-zinc-700 mb-1.5"> New Password </label>
            <input id="password" name="password" type="password" required v-model="password" class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 sm:text-sm transition-colors bg-zinc-50/50 focus:bg-white" placeholder="••••••••" />
            
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

          <div>
            <label for="confirm" class="block text-sm font-medium text-zinc-700 mb-1.5"> Confirm Password </label>
            <input id="confirm" name="confirm" type="password" required v-model="confirmPassword" class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 sm:text-sm transition-colors bg-zinc-50/50 focus:bg-white" placeholder="••••••••" />
          </div>

          <div class="pt-2">
            <button type="submit" :disabled="loading || !isPasswordValid || password !== confirmPassword" class="w-full flex justify-center items-center py-3.5 px-4 border border-transparent rounded-xl shadow-sm text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 disabled:opacity-50 transition-all duration-200">
              <span v-if="loading">Updating...</span>
              <span v-else>Update password</span>
            </button>
          </div>
        </form>
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
          <p class="text-zinc-400 max-w-sm">Create a strong password to keep your account safe.</p>
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
