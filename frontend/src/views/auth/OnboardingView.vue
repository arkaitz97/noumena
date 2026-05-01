<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()

const connectYouTube = () => {
  // Redirect to backend OAuth initiation route
  window.location.href = '/api/channels/connect/youtube'
}

const skipForNow = () => {
  localStorage.setItem('hasChannels', 'true')
  router.push('/dashboard')
}
</script>

<template>
  <div class="min-h-screen bg-white flex">
    <!-- Left Panel -->
    <div class="flex-1 flex flex-col justify-center px-6 sm:px-12 lg:flex-none lg:w-1/2 xl:w-5/12 mx-auto lg:mx-0 relative z-10">
      <div class="mx-auto w-full max-w-sm lg:max-w-md">
        <!-- Brand/Logo -->
        <div class="flex items-center gap-2 mb-16">
          <div class="w-8 h-8 rounded-lg bg-zinc-900 flex items-center justify-center shadow-md">
            <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <span class="text-xl font-bold tracking-tight text-zinc-900">Noumena</span>
        </div>

        <h1 class="text-4xl font-semibold tracking-tight text-zinc-900 mb-4">Connect your channel</h1>
        <p class="text-zinc-500 text-lg mb-10 leading-relaxed">
          Unlock AI-powered insights by connecting your first platform. We'll automatically sync and analyze your latest audience interactions.
        </p>

        <!-- Actions -->
        <div class="space-y-4">
          <button @click="connectYouTube" class="group relative w-full flex justify-center items-center py-3.5 px-4 rounded-xl border border-zinc-200 bg-white text-base font-medium text-zinc-900 shadow-[0_1px_2px_rgba(0,0,0,0.05)] hover:border-zinc-300 hover:bg-zinc-50 hover:shadow-[0_4px_12px_rgba(0,0,0,0.05)] focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:ring-offset-2 transition-all duration-200">
            <svg class="w-6 h-6 mr-3 text-[#FF0000] drop-shadow-sm group-hover:scale-105 transition-transform duration-200" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path fill-rule="evenodd" d="M21.582,6.186c-0.23-0.86-0.908-1.538-1.768-1.768C18.254,4,12,4,12,4S5.746,4,4.186,4.418 c-0.86,0.23-1.538,0.908-1.768,1.768C2,7.746,2,12,2,12s0,4.254,0.418,5.814c0.23,0.86,0.908,1.538,1.768,1.768 C5.746,20,12,20,12,20s6.254,0,7.814-0.418c0.86-0.23,1.538-0.908,1.768-1.768C22,16.254,22,12,22,12S22,7.746,21.582,6.186z M9.996,15.005l0-6.01l5.418,3.004L9.996,15.005z" clip-rule="evenodd" />
            </svg>
            Continue with YouTube
          </button>
          
          <button @click="skipForNow" class="w-full flex justify-center py-3.5 px-4 text-sm font-medium text-zinc-500 hover:text-zinc-800 transition-colors duration-200">
            I'll do this later
          </button>
        </div>
      </div>
    </div>

    <!-- Right Panel (Decorative) -->
    <div class="hidden lg:flex relative w-0 flex-1 bg-zinc-950 overflow-hidden items-center justify-center">
      <!-- Ambient glow circles for premium feel -->
      <div class="absolute top-[-10%] left-[-10%] w-[40rem] h-[40rem] bg-indigo-500 rounded-full mix-blend-screen filter blur-[120px] opacity-20 animate-blob"></div>
      <div class="absolute bottom-[-10%] right-[-10%] w-[40rem] h-[40rem] bg-violet-600 rounded-full mix-blend-screen filter blur-[120px] opacity-20 animate-blob animation-delay-2000"></div>

      <!-- Sophisticated Glassmorphism Card -->
      <div class="relative z-10 max-w-lg p-10 rounded-[2rem] bg-white/5 border border-white/10 backdrop-blur-3xl shadow-2xl">
        <div class="inline-flex items-center px-3 py-1 rounded-full bg-white/10 border border-white/10 mb-8">
          <span class="flex h-2 w-2 rounded-full bg-green-400 mr-2 shadow-[0_0_8px_rgba(74,222,128,0.8)]"></span>
          <span class="text-xs font-medium text-zinc-300 uppercase tracking-widest">System Online</span>
        </div>
        
        <blockquote class="text-2xl font-medium text-zinc-100 leading-snug mb-8">
          "The level of insight provided by Noumena is simply unparalleled. It feels less like a tool and more like an extension of our team's intuition."
        </blockquote>
        
        <div class="flex items-center">
          <div class="h-12 w-12 rounded-full bg-gradient-to-br from-indigo-400 to-violet-600 flex items-center justify-center text-white font-semibold text-lg ring-2 ring-white/20">
            A
          </div>
          <div class="ml-4">
            <div class="text-base font-semibold text-white">Alex Chen</div>
            <div class="text-sm text-zinc-400 font-medium">Head of Operations, Nexus</div>
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
