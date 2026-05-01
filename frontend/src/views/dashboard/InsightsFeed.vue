<script setup lang="ts">
import { ref, computed } from 'vue'

const filters = ref({
  category: 'all',
  channel: 'all',
  sentiment: 'all',
  dateRange: '7d'
})

// Mock data based on PRD
const comments = ref([
  {
    id: 1,
    author: '@creative_sarah',
    body: 'How do you structure your lighting for the overhead shots? It looks so cinematic!',
    category: 'Question',
    sentiment: 0.85,
    topic: 'Lighting Setup',
    channel: 'Sarah Videos',
    postedAt: '2 hours ago',
    starred: false
  },
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
  },
  {
    id: 3,
    author: '@brand_collab',
    body: 'Hey! We love your videos. Are you open to sponsorships for your next series?',
    category: 'Collaboration Ask',
    sentiment: 0.90,
    topic: 'Sponsorship',
    channel: 'Main Channel',
    postedAt: '1 day ago',
    starred: false
  }
])

const filteredComments = computed(() => {
  return comments.value.filter(comment => {
    // Category filter
    let categoryMatch = false;
    if (filters.value.category === 'all') categoryMatch = true;
    else if (filters.value.category === 'question' && comment.category === 'Question') categoryMatch = true;
    else if (filters.value.category === 'idea' && comment.category === 'Content Idea') categoryMatch = true;
    else if (filters.value.category === 'collab' && comment.category === 'Collaboration Ask') categoryMatch = true;

    // Channel filter
    let channelMatch = false;
    if (filters.value.channel === 'all') channelMatch = true;
    else if (filters.value.channel === comment.channel) channelMatch = true;

    // Sentiment filter
    let sentimentMatch = false;
    if (filters.value.sentiment === 'all') sentimentMatch = true;
    else if (filters.value.sentiment === 'positive' && comment.sentiment > 0.5) sentimentMatch = true;
    else if (filters.value.sentiment === 'neutral' && comment.sentiment >= -0.5 && comment.sentiment <= 0.5) sentimentMatch = true;
    else if (filters.value.sentiment === 'negative' && comment.sentiment < -0.5) sentimentMatch = true;

    let dateMatch = true;

    return categoryMatch && channelMatch && sentimentMatch && dateMatch;
  });
})

const toggleStar = (comment: any) => {
  comment.starred = !comment.starred
}
</script>

<template>
  <div class="max-w-7xl mx-auto py-8 sm:px-6 lg:px-8">
    <div class="px-4 sm:px-0">
      
      <!-- Page Header -->
      <div class="md:flex md:items-center md:justify-between mb-8">
        <div class="flex-1 min-w-0">
          <h2 class="text-2xl font-semibold leading-7 text-zinc-900 sm:text-3xl sm:tracking-tight">Insights Feed</h2>
          <p class="mt-1 text-sm text-zinc-500">Analyze your audience signals and engage with the most valuable interactions.</p>
        </div>
        <div class="mt-4 flex md:mt-0 md:ml-4">
          <button type="button" class="inline-flex items-center px-4 py-2 border border-zinc-200 rounded-xl shadow-sm text-sm font-medium text-zinc-700 bg-white hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 transition-colors">
            <svg class="w-4 h-4 mr-2 text-zinc-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" /></svg>
            Export CSV
          </button>
        </div>
      </div>

      <!-- Weekly Digest Banner -->
      <div class="bg-gradient-to-r from-indigo-50 to-blue-50 border border-indigo-100 rounded-2xl p-5 mb-8 shadow-sm">
        <div class="flex items-start sm:items-center">
          <div class="flex-shrink-0 mt-0.5 sm:mt-0">
            <div class="w-10 h-10 rounded-full bg-indigo-100 flex items-center justify-center">
              <svg class="h-5 w-5 text-indigo-600" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/></svg>
            </div>
          </div>
          <div class="ml-4 flex-1">
            <h3 class="text-sm font-semibold text-indigo-900">Your Weekly Insight Digest is ready</h3>
            <div class="mt-1 text-sm text-indigo-700/80">
              <p>Top trend this week: <span class="font-medium text-indigo-900">"Lighting setup for videos"</span>. Sentiment is up 12% across your channels! Open your digest for a full custom analysis.</p>
            </div>
            <div class="mt-3 flex sm:mt-2 space-x-3">
              <button type="button" class="bg-indigo-600 px-3 py-1.5 rounded-lg text-xs font-semibold text-white shadow-sm hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-600 transition-colors focus:ring-offset-blue-50">
                View full digest
              </button>
              <button type="button" class="text-indigo-600 hover:text-indigo-800 text-xs font-medium px-2 py-1.5 transition-colors">
                Dismiss
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-4 gap-8">
        <!-- Sidebar / Filters -->
        <div class="lg:col-span-1 space-y-6">
          <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5">
            <h3 class="text-sm font-semibold text-zinc-900 mb-4">Filters</h3>
            
            <div class="space-y-4">
              <div>
                <label class="block text-xs font-medium text-zinc-500 mb-1.5 uppercase tracking-wide">Category</label>
                <select v-model="filters.category" class="block w-full pl-3 pr-10 py-2 text-sm text-zinc-900 border-zinc-200 focus:outline-none focus:ring-zinc-900 focus:border-zinc-900 rounded-xl bg-zinc-50/50 hover:bg-zinc-50 transition-colors cursor-pointer">
                   <option value="all" class="bg-white text-zinc-900">All Signals</option>
                   <option value="question" class="bg-white text-zinc-900">Questions</option>
                   <option value="idea" class="bg-white text-zinc-900">Content Ideas</option>
                   <option value="collab" class="bg-white text-zinc-900">Collaboration Asks</option>
                </select>
              </div>
              <div>
                <label class="block text-xs font-medium text-zinc-500 mb-1.5 uppercase tracking-wide">Channel</label>
                <select v-model="filters.channel" class="block w-full pl-3 pr-10 py-2 text-sm text-zinc-900 border-zinc-200 focus:outline-none focus:ring-zinc-900 focus:border-zinc-900 rounded-xl bg-zinc-50/50 hover:bg-zinc-50 transition-colors cursor-pointer">
                   <option value="all" class="bg-white text-zinc-900">All Channels</option>
                   <option value="Sarah Videos" class="bg-white text-zinc-900">Sarah Videos</option>
                   <option value="Tech Channel" class="bg-white text-zinc-900">Tech Channel</option>
                </select>
              </div>
              <div>
                <label class="block text-xs font-medium text-zinc-500 mb-1.5 uppercase tracking-wide">Sentiment</label>
                <select v-model="filters.sentiment" class="block w-full pl-3 pr-10 py-2 text-sm text-zinc-900 border-zinc-200 focus:outline-none focus:ring-zinc-900 focus:border-zinc-900 rounded-xl bg-zinc-50/50 hover:bg-zinc-50 transition-colors cursor-pointer">
                   <option value="all" class="bg-white text-zinc-900">Any</option>
                   <option value="positive" class="bg-white text-zinc-900">Positive (> 0.5)</option>
                   <option value="neutral" class="bg-white text-zinc-900">Neutral</option>
                   <option value="negative" class="bg-white text-zinc-900">Negative (< -0.5)</option>
                </select>
              </div>
              <div>
                <label class="block text-xs font-medium text-zinc-500 mb-1.5 uppercase tracking-wide">Date Range</label>
                <select v-model="filters.dateRange" class="block w-full pl-3 pr-10 py-2 text-sm text-zinc-900 border-zinc-200 focus:outline-none focus:ring-zinc-900 focus:border-zinc-900 rounded-xl bg-zinc-50/50 hover:bg-zinc-50 transition-colors cursor-pointer">
                   <option value="7d" class="bg-white text-zinc-900">Last 7 Days</option>
                   <option value="30d" class="bg-white text-zinc-900">Last 30 Days</option>
                   <option value="all" class="bg-white text-zinc-900">All Time</option>
                </select>
              </div>
            </div>
          </div>
          
          <!-- Shield Card -->
          <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5 border-t-[3px] border-emerald-500">
            <div class="flex items-center gap-3 mb-2">
              <svg class="h-5 w-5 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>
              <h3 class="text-sm font-semibold text-zinc-900">Toxicity Shield</h3>
            </div>
            <p class="text-2xl font-semibold tracking-tight text-zinc-900">14%</p>
            <p class="text-xs text-zinc-500 mt-1">of harmful comments hidden automatically this week.</p>
          </div>
        </div>

        <!-- Feed Component -->
        <div class="lg:col-span-3 space-y-4">
          <div v-for="comment in filteredComments" :key="comment.id" class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-5 hover:shadow-md transition-shadow duration-200 relative group overflow-hidden">
            <!-- Sentiment indicator bar -->
            <div :class="[
              'absolute left-0 top-0 bottom-0 w-1',
              comment.sentiment > 0.7 ? 'bg-emerald-400' : (comment.sentiment > 0.4 ? 'bg-zinc-300' : 'bg-rose-400')
            ]"></div>
            
            <div class="pl-3">
              <!-- Header -->
              <div class="flex items-start justify-between">
                <div class="flex flex-wrap items-center gap-2 mb-2">
                  <p class="text-sm font-semibold text-zinc-900">{{ comment.author }}</p>
                  <span class="text-zinc-300">•</span>
                  <p class="text-xs text-zinc-500">{{ comment.postedAt }}</p>
                  <span class="text-zinc-300">•</span>
                  <span class="text-xs font-medium text-zinc-500">{{ comment.channel }}</span>
                </div>
                <!-- Action Buttons -->
                <div class="flex items-center space-x-1 opacity-100 sm:opacity-0 group-hover:opacity-100 transition-opacity">
                   <button @click.prevent="toggleStar(comment)" class="p-1.5 rounded-lg text-zinc-400 hover:text-amber-400 hover:bg-zinc-50 transition-colors focus:outline-none">
                    <svg :class="[comment.starred ? 'text-amber-400 fill-amber-400' : 'text-zinc-400', 'h-5 w-5']" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5">
                      <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                    </svg>
                  </button>
                  <button class="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-900 hover:bg-zinc-50 transition-colors focus:outline-none">
                    <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M14 5l7 7m0 0l-7 7m7-7H3" /></svg>
                  </button>
                </div>
              </div>
              
              <!-- Body -->
              <p class="text-[15px] text-zinc-800 leading-relaxed mb-4 w-full break-words pr-4">
                {{ comment.body }}
              </p>
              
              <!-- Badges -->
              <div class="flex flex-wrap items-center gap-2">
                <span class="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-semibold tracking-wide bg-indigo-50 text-indigo-700 ring-1 ring-inset ring-indigo-700/10">
                  {{ comment.category }}
                </span>
                <span class="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-semibold tracking-wide bg-purple-50 text-purple-700 ring-1 ring-inset ring-purple-700/10">
                  {{ comment.topic }}
                </span>
                <span class="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium bg-zinc-50 text-zinc-600 ring-1 ring-inset ring-zinc-500/10">
                  <svg class="w-3.5 h-3.5 mr-1 text-zinc-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.828 14.828a4 4 0 01-5.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                  Score: {{ comment.sentiment }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
      
    </div>
  </div>
</template>
