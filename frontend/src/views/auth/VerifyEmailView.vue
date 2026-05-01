<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'

const email = ref('alex@example.com') // In a real app, read from auth store
const resending = ref(false)
const resent = ref(false)

const resendEmail = async () => {
  resending.value = true
  // Simulate API call
  await new Promise(resolve => setTimeout(resolve, 1000))
  resent.value = true
  resending.value = false
}
</script>
<template>
  <div class="min-h-screen bg-zinc-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8 relative overflow-hidden">
    <!-- Ambient subtle background blur -->
    <div class="absolute top-[-10%] left-[10%] w-[30rem] h-[30rem] bg-indigo-200 rounded-full mix-blend-multiply filter blur-[100px] opacity-40"></div>
    <div class="absolute bottom-[-10%] right-[10%] w-[30rem] h-[30rem] bg-blue-100 rounded-full mix-blend-multiply filter blur-[100px] opacity-40"></div>

    <div class="sm:mx-auto sm:w-full sm:max-w-md relative z-10 text-center">
      <div class="flex items-center justify-center gap-2 mb-10">
        <div class="w-8 h-8 rounded-lg bg-zinc-900 flex items-center justify-center shadow-md">
          <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
        </div>
        <span class="text-xl font-bold tracking-tight text-zinc-900">Noumena</span>
      </div>

      <div class="flex justify-center mb-6">
        <div class="h-20 w-20 bg-white rounded-2xl flex items-center justify-center shadow-sm border border-zinc-100 ring-4 ring-zinc-50 relative">
          <div class="absolute inset-0 bg-indigo-50 rounded-2xl animate-ping opacity-30"></div>
          <svg class="h-10 w-10 text-indigo-600 relative z-10" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
          </svg>
        </div>
      </div>
      <h2 class="text-3xl font-semibold tracking-tight text-zinc-900 mb-4">Check your inbox</h2>
      <p class="text-lg text-zinc-500 px-4 leading-relaxed mb-10">
        We've sent a verification link to <br/><span class="font-medium text-zinc-900">{{ email }}</span>. 
      </p>

      <div class="bg-white py-8 px-6 shadow-[0_4px_20px_-4px_rgba(0,0,0,0.05)] rounded-[2rem] border border-zinc-100/80 mx-4 sm:mx-0">
        <p class="text-sm text-zinc-500 mb-6 font-medium">Didn't receive the email? Check your spam folder or request a new one below.</p>
        
        <button @click="resendEmail" :disabled="resending || resent" class="w-full flex justify-center items-center py-3.5 px-4 border border-zinc-200 rounded-xl shadow-sm text-sm font-medium text-zinc-700 bg-white hover:bg-zinc-50 hover:border-zinc-300 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200">
          <span v-if="resending" class="flex items-center">
            <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-zinc-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
            Sending...
          </span>
          <span v-else-if="resent" class="text-emerald-600 flex items-center">
            <svg class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
            Email sent
          </span>
          <span v-else>Resend verification link</span>
        </button>

        <div class="mt-8 pt-6 border-t border-zinc-100 text-sm">
          <RouterLink to="/login" class="font-medium text-zinc-500 hover:text-zinc-900 transition-colors inline-flex items-center">
            <svg class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18" /></svg>
            Back to sign in
          </RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>
