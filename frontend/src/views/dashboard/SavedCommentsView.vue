<script setup lang="ts">
import { ref } from 'vue'

const comments = ref([
  {
    id: 2,
    author: '@tech_review_daily',
    body: 'Would love to see a comparison video between this and the Sony A7IV!',
    category: 'Content Idea',
    sentiment: 0.60,
    topic: 'Camera Comparison',
    channel: 'Tech Channel',
    postedAt: '5 hours ago',
    starred: true
  }
])

const toggleStar = (comment: any) => {
  comment.starred = !comment.starred
}
</script>

<template>
  <div class="max-w-4xl mx-auto py-8 sm:px-6 lg:px-8">
    <div class="px-4 py-6 sm:px-0">
      <div class="mb-8 border-b border-zinc-200 pb-5">
        <h2 class="text-2xl font-semibold leading-7 text-zinc-900 sm:text-3xl sm:tracking-tight">Saved Comments</h2>
        <p class="mt-2 text-sm text-zinc-500 max-w-2xl">Your personal collection of valuable audience insights, ideas, and feedback.</p>
      </div>

      <!-- Feed -->
      <div class="space-y-4">
        <div v-for="comment in comments" :key="comment.id" class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5 hover:shadow-md transition-shadow duration-200 relative group overflow-hidden">
          <div :class="[
            'absolute left-0 top-0 bottom-0 w-1',
            comment.sentiment > 0.7 ? 'bg-emerald-400' : (comment.sentiment > 0.4 ? 'bg-zinc-300' : 'bg-rose-400')
          ]"></div>
          
          <div class="pl-3">
            <div class="flex items-start justify-between">
              <div class="flex flex-wrap items-center gap-2 mb-2">
                <p class="text-sm font-semibold text-zinc-900">{{ comment.author }}</p>
                <span class="text-zinc-300">•</span>
                <p class="text-xs text-zinc-500">{{ comment.postedAt }}</p>
                <span class="text-zinc-300">•</span>
                <span class="text-xs font-medium text-zinc-500">{{ comment.channel }}</span>
              </div>
              <div class="flex items-center space-x-1 opacity-100 sm:opacity-0 group-hover:opacity-100 transition-opacity">
                  <button @click.prevent="toggleStar(comment)" class="p-1.5 rounded-lg text-zinc-400 hover:text-amber-400 hover:bg-zinc-50 transition-colors focus:outline-none">
                  <svg :class="[comment.starred ? 'text-amber-400 fill-amber-400' : 'text-zinc-400', 'h-5 w-5']" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5">
                    <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                  </svg>
                </button>
              </div>
            </div>
            
            <p class="text-[15px] text-zinc-800 leading-relaxed mb-4 w-full break-words pr-4">
              {{ comment.body }}
            </p>
            
            <div class="flex flex-wrap items-center gap-2">
              <span class="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-semibold tracking-wide bg-indigo-50 text-indigo-700 ring-1 ring-inset ring-indigo-700/10">
                {{ comment.category }}
              </span>
              <span class="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-semibold tracking-wide bg-purple-50 text-purple-700 ring-1 ring-inset ring-purple-700/10">
                {{ comment.topic }}
              </span>
            </div>
          </div>
        </div>
        
        <div v-if="comments.length === 0" class="text-center py-16 bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl">
          <svg class="mx-auto h-12 w-12 text-zinc-300 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
          </svg>
          <h3 class="mt-2 text-sm font-semibold text-zinc-900">No saved comments</h3>
          <p class="mt-1 text-sm text-zinc-500">You haven't explicitly starred any insights yet.</p>
          <div class="mt-6">
            <RouterLink to="/dashboard" class="inline-flex items-center px-3 py-2 border border-zinc-200 shadow-sm text-sm font-medium rounded-xl text-zinc-700 bg-white hover:bg-zinc-50 transition-colors">
              <svg class="-ml-1 mr-2 h-4 w-4 text-zinc-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18" /></svg>
              Go to feed
            </RouterLink>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
