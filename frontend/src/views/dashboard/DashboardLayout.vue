<script setup lang="ts">
import { ref } from 'vue'
import { RouterView, RouterLink, useRoute } from 'vue-router'

const quotaExceeded = ref(true) // true for demo purposes
const route = useRoute()

const isCurrent = (path: string) => {
  return route.path.startsWith(path) && (path !== '/dashboard' || route.path === '/dashboard')
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 flex flex-col font-sans selection:bg-indigo-100 selection:text-indigo-900">
    <!-- Optional Quota Banner with premium styling -->
    <div v-if="quotaExceeded" class="bg-gradient-to-r from-red-600 to-rose-600 shadow-sm">
      <div class="max-w-7xl mx-auto py-2.5 px-4 sm:px-6 lg:px-8">
        <div class="flex items-center justify-between flex-wrap gap-2">
          <div class="w-0 flex-1 flex items-center">
            <span class="flex p-1.5 rounded-lg bg-white/20 backdrop-blur-sm">
              <svg class="h-5 w-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" /></svg>
            </span>
            <p class="ml-3 font-medium text-white truncate text-sm">
              <span class="md:hidden">Quota exceeded!</span>
              <span class="hidden md:inline">You've reached your monthly AI processing limit. Upgrade your plan to keep receiving insights.</span>
            </p>
          </div>
          <div class="order-3 mt-2 flex-shrink-0 w-full sm:order-2 sm:mt-0 sm:w-auto">
            <RouterLink to="/settings/billing" class="flex items-center justify-center px-4 py-1.5 border border-transparent rounded-full shadow-sm text-xs font-semibold text-red-600 bg-white hover:bg-red-50 focus:ring-2 focus:ring-white focus:ring-offset-2 focus:ring-offset-red-600 transition-colors">
              Upgrade Plan
            </RouterLink>
          </div>
        </div>
      </div>
    </div>

    <!-- Premium Navigation -->
    <nav class="bg-white/80 backdrop-blur-md border-b border-zinc-200 sticky top-0 z-40">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div class="flex justify-between h-16">
          <div class="flex items-center gap-8">
            <!-- Brand -->
            <RouterLink to="/dashboard" class="flex-shrink-0 flex items-center gap-2 group">
              <div class="w-8 h-8 rounded-lg bg-zinc-900 flex items-center justify-center shadow-md group-hover:scale-105 transition-transform duration-200">
                <svg class="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
              </div>
              <span class="font-bold text-lg tracking-tight text-zinc-900">Noumena</span>
            </RouterLink>
            
            <!-- Links -->
            <div class="hidden sm:flex sm:items-baseline space-x-1">
              <RouterLink to="/dashboard" :class="[isCurrent('/dashboard') && route.path === '/dashboard' ? 'bg-zinc-100 text-zinc-900' : 'text-zinc-600 hover:text-zinc-900 hover:bg-zinc-50', 'px-3 py-2 rounded-lg text-sm font-medium transition-colors']">Dashboard</RouterLink>
              <RouterLink to="/dashboard/saved" :class="[isCurrent('/dashboard/saved') ? 'bg-zinc-100 text-zinc-900' : 'text-zinc-600 hover:text-zinc-900 hover:bg-zinc-50', 'px-3 py-2 rounded-lg text-sm font-medium transition-colors']">Saved</RouterLink>
              <RouterLink to="/settings/channels" :class="[isCurrent('/settings') ? 'bg-zinc-100 text-zinc-900' : 'text-zinc-600 hover:text-zinc-900 hover:bg-zinc-50', 'px-3 py-2 rounded-lg text-sm font-medium transition-colors']">Settings</RouterLink>
            </div>
          </div>
          
          <!-- User Profile / Avatar Mock -->
          <div class="flex items-center">
            <button class="flex items-center justify-center h-8 w-8 rounded-full bg-gradient-to-tr from-indigo-500 to-purple-500 text-white font-medium text-sm focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:ring-offset-2 ring-offset-white border border-white shadow-sm">
              JD
            </button>
          </div>
        </div>
      </div>
    </nav>

    <!-- Main Content Area -->
    <main class="flex-1 w-full mx-auto relative">
      <RouterView />
    </main>
  </div>
</template>
