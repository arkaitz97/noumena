<script setup lang="ts">
import { ref } from 'vue'

const channels = ref([
  { id: 1, name: 'Sarah Videos', platform: 'YouTube', status: 'Active', commentsProcessed: '15.2K' },
  { id: 2, name: 'Tech Channel', platform: 'YouTube', status: 'Paused', commentsProcessed: '3.4K' }
])

const connectYouTube = () => {
  // Redirect to backend OAuth initiation route
  window.location.href = '/api/channels/connect/youtube'
}

const actionChannel = ref<any>(null)
const pauseModalOpen = ref(false)
const removeModalOpen = ref(false)

const openPauseModal = (channel: any) => {
  actionChannel.value = channel
  pauseModalOpen.value = true
}

const openRemoveModal = (channel: any) => {
  actionChannel.value = channel
  removeModalOpen.value = true
}

const confirmPause = () => {
  if (actionChannel.value) {
    actionChannel.value.status = 'Paused'
  }
  pauseModalOpen.value = false
}

const confirmRemove = () => {
  if (actionChannel.value) {
    channels.value = channels.value.filter(c => c.id !== actionChannel.value.id)
  }
  removeModalOpen.value = false
}
</script>

<template>
  <div class="space-y-10">
    <div class="sm:flex sm:items-center sm:justify-between px-4 sm:px-0">
      <div>
        <h3 class="text-2xl font-semibold leading-7 text-zinc-900 tracking-tight">Connected Channels</h3>
        <p class="mt-2 text-sm text-zinc-500">Manage your connected social media channels and their tracking status.</p>
      </div>
      <div class="mt-5 sm:mt-0 flex-shrink-0">
        <button @click="connectYouTube" type="button" class="w-full sm:w-auto inline-flex items-center justify-center px-5 py-2.5 border border-transparent shadow-sm text-sm font-medium rounded-xl text-white bg-zinc-900 hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 transition-all duration-200">
          <svg class="-ml-1 mr-2.5 h-4 w-4 text-red-500" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true"><path fill-rule="evenodd" d="M21.582,6.186c-0.23-0.86-0.908-1.538-1.768-1.768C18.254,4,12,4,12,4S5.746,4,4.186,4.418 c-0.86,0.23-1.538,0.908-1.768,1.768C2,7.746,2,12,2,12s0,4.254,0.418,5.814c0.23,0.86,0.908,1.538,1.768,1.768 C5.746,20,12,20,12,20s6.254,0,7.814-0.418c0.86-0.23,1.538-0.908,1.768-1.768C22,16.254,22,12,22,12S22,7.746,21.582,6.186z M9.996,15.005l0-6.01l5.418,3.004L9.996,15.005z" clip-rule="evenodd" /></svg>
          Connect YouTube
        </button>
      </div>
    </div>

    <!-- Channels List -->
    <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 sm:rounded-2xl overflow-hidden">
      <ul role="list" class="divide-y divide-zinc-100">
        <li v-for="channel in channels" :key="channel.id" class="group transition-colors hover:bg-zinc-50/50">
          <div class="px-6 py-6 sm:py-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div class="flex items-center min-w-0">
              <div class="flex-shrink-0 h-12 w-12 flex items-center justify-center rounded-xl bg-red-50 text-red-600 border border-red-100 shadow-sm relative">
                <svg class="h-6 w-6" fill="currentColor" viewBox="0 0 24 24"><path d="M21.582,6.186c-0.23-0.86-0.908-1.538-1.768-1.768C18.254,4,12,4,12,4S5.746,4,4.186,4.418 c-0.86,0.23-1.538,0.908-1.768,1.768C2,7.746,2,12,2,12s0,4.254,0.418,5.814c0.23,0.86,0.908,1.538,1.768,1.768 C5.746,20,12,20,12,20s6.254,0,7.814-0.418c0.86-0.23,1.538-0.908,1.768-1.768C22,16.254,22,12,22,12S22,7.746,21.582,6.186z M9.996,15.005l0-6.01l5.418,3.004L9.996,15.005z" /></svg>
                <div v-if="channel.status === 'Active'" class="absolute -top-1 -right-1 flex h-3 w-3">
                  <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                  <span class="relative inline-flex rounded-full h-3 w-3 bg-emerald-500 border-2 border-white"></span>
                </div>
              </div>
              <div class="ml-4 min-w-0">
                <div class="flex items-center gap-2">
                  <h4 class="text-base font-semibold text-zinc-900 truncate">{{ channel.name }}</h4>
                  <span :class="[channel.status === 'Active' ? 'bg-emerald-50 text-emerald-700 ring-emerald-600/20' : 'bg-zinc-100 text-zinc-700 ring-zinc-500/20', 'inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset']">
                    {{ channel.status }}
                  </span>
                </div>
                <div class="mt-1 flex items-center text-sm text-zinc-500 gap-2">
                  <span>{{ channel.platform }}</span>
                  <span class="w-1 h-1 rounded-full bg-zinc-300"></span>
                  <span class="font-medium text-zinc-700">{{ channel.commentsProcessed }} comments processed</span>
                </div>
              </div>
            </div>
            <div class="flex items-center space-x-2 sm:opacity-0 group-hover:opacity-100 transition-opacity">
              <button v-if="channel.status === 'Active'" @click="openPauseModal(channel)" class="px-3 py-1.5 rounded-lg text-sm font-medium text-zinc-600 bg-white border border-zinc-200 hover:bg-zinc-50 hover:text-zinc-900 transition-colors shadow-sm focus:outline-none focus:ring-2 focus:ring-zinc-900">Pause</button>
              <button v-else @click="channel.status = 'Active'" class="px-3 py-1.5 rounded-lg text-sm font-medium text-emerald-700 bg-emerald-50 hover:bg-emerald-100 transition-colors shadow-sm focus:outline-none focus:ring-2 focus:ring-emerald-600">Resume Tracking</button>
              
              <button @click="openRemoveModal(channel)" class="p-1.5 rounded-lg text-zinc-400 hover:text-rose-600 hover:bg-rose-50 transition-colors focus:outline-none focus:ring-2 focus:ring-rose-500">
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
              </button>
            </div>
          </div>
        </li>
        <li v-if="channels.length === 0" class="px-6 py-12 text-center">
          <svg class="mx-auto h-12 w-12 text-zinc-300 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
          </svg>
          <h3 class="text-sm font-semibold text-zinc-900 mb-1">No channels connected</h3>
          <p class="text-sm text-zinc-500 max-w-sm mx-auto">Connect your first channel to start getting AI-powered insights from your audience.</p>
        </li>
      </ul>
    </div>

    <!-- Pause Modal -->
    <div v-if="pauseModalOpen" class="fixed z-50 inset-0 overflow-y-auto" role="dialog" aria-modal="true">
      <div class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
        <div class="fixed inset-0 bg-zinc-900/60 backdrop-blur-sm transition-opacity" @click="pauseModalOpen = false" aria-hidden="true"></div>
        <span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>
        <div class="inline-block align-bottom bg-white rounded-2xl px-4 pt-5 pb-6 text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full sm:p-8">
          <div class="sm:flex sm:items-start">
            <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-amber-100 sm:mx-0 sm:h-10 sm:w-10">
              <svg class="h-6 w-6 text-amber-600" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" /></svg>
            </div>
            <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left w-full">
              <h3 class="text-xl leading-6 font-semibold text-zinc-900 mb-2">Pause Tracking</h3>
              <div class="mt-2">
                <p class="text-sm text-zinc-500 leading-relaxed">
                  Pausing will stop this channel from appearing in your feed and stop consuming your quota. You can resume at any time.
                </p>
              </div>
            </div>
          </div>
          <div class="mt-8 sm:flex sm:flex-row-reverse gap-3">
            <button @click="confirmPause" type="button" class="w-full inline-flex justify-center rounded-xl border border-transparent shadow-sm px-5 py-2.5 bg-amber-600 text-sm font-medium text-white hover:bg-amber-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-500 sm:w-auto transition-colors">
              Pause Channel
            </button>
            <button @click="pauseModalOpen = false" type="button" class="mt-3 sm:mt-0 w-full inline-flex justify-center rounded-xl border border-zinc-200 shadow-sm px-5 py-2.5 bg-white text-sm font-medium text-zinc-700 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 sm:w-auto transition-colors">
              Cancel
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Remove Modal -->
    <div v-if="removeModalOpen" class="fixed z-50 inset-0 overflow-y-auto" role="dialog" aria-modal="true">
      <div class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
        <div class="fixed inset-0 bg-zinc-900/60 backdrop-blur-sm transition-opacity" @click="removeModalOpen = false" aria-hidden="true"></div>
        <span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>
        <div class="inline-block align-bottom bg-white rounded-2xl px-4 pt-5 pb-6 text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full sm:p-8">
          <div class="sm:flex sm:items-start">
            <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-rose-100 sm:mx-0 sm:h-10 sm:w-10">
              <svg class="h-6 w-6 text-rose-600" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
            </div>
            <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left w-full">
              <h3 class="text-xl leading-6 font-semibold text-zinc-900 mb-2">Remove Channel</h3>
              <div class="mt-2">
                <p class="text-sm text-zinc-500 leading-relaxed">
                  Are you sure you want to permanently remove this channel? All your personalized settings and saved comments for this channel will be lost.
                </p>
              </div>
            </div>
          </div>
          <div class="mt-8 sm:flex sm:flex-row-reverse gap-3">
            <button @click="confirmRemove" type="button" class="w-full inline-flex justify-center rounded-xl border border-transparent shadow-sm px-5 py-2.5 bg-rose-600 text-sm font-medium text-white hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-rose-500 sm:w-auto transition-colors">
              Remove
            </button>
            <button @click="removeModalOpen = false" type="button" class="mt-3 sm:mt-0 w-full inline-flex justify-center rounded-xl border border-zinc-200 shadow-sm px-5 py-2.5 bg-white text-sm font-medium text-zinc-700 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 sm:w-auto transition-colors">
              Cancel
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
