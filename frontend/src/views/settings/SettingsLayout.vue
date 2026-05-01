<script setup lang="ts">
import { RouterView, RouterLink, useRoute } from 'vue-router'

const route = useRoute()

const isCurrent = (path: string) => {
  return route.path.startsWith(path) && (path !== '/dashboard' || route.path === '/dashboard')
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 flex flex-col font-sans selection:bg-indigo-100 selection:text-indigo-900">
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
    <main class="flex-1 max-w-5xl w-full mx-auto py-10 sm:px-6 lg:px-8 flex flex-col md:flex-row gap-8">
      <div class="w-full md:w-64 flex-shrink-0">
        <div class="mb-6 px-4 sm:px-0">
          <h2 class="text-xl font-semibold tracking-tight text-zinc-900">Settings</h2>
          <p class="text-sm text-zinc-500 mt-1">Manage your account and preferences.</p>
        </div>
        <nav class="space-y-1 px-2 sm:px-0">
          <RouterLink to="/settings/channels" active-class="bg-white shadow-sm ring-1 ring-zinc-950/5 text-zinc-900 font-semibold" class="text-zinc-600 hover:text-zinc-900 hover:bg-zinc-100 flex items-center px-4 py-2.5 text-sm font-medium rounded-xl transition-all">
            <svg class="mr-3 h-5 w-5 opacity-70" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z"></path></svg>
            Channels
          </RouterLink>
          <RouterLink to="/settings/billing" active-class="bg-white shadow-sm ring-1 ring-zinc-950/5 text-zinc-900 font-semibold" class="text-zinc-600 hover:text-zinc-900 hover:bg-zinc-100 flex items-center px-4 py-2.5 text-sm font-medium rounded-xl transition-all">
            <svg class="mr-3 h-5 w-5 opacity-70" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z"></path></svg>
            Billing
          </RouterLink>
          <RouterLink to="/settings/account" active-class="bg-white shadow-sm ring-1 ring-zinc-950/5 text-zinc-900 font-semibold" class="text-zinc-600 hover:text-zinc-900 hover:bg-zinc-100 flex items-center px-4 py-2.5 text-sm font-medium rounded-xl transition-all">
            <svg class="mr-3 h-5 w-5 opacity-70" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
            Account
          </RouterLink>
        </nav>
      </div>
      <div class="flex-1 w-full max-w-full">
        <RouterView />
      </div>
    </main>
  </div>
</template>
