<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()

const getStarted = () => {
  router.push('/signup')
}

const faqs = [
  {
    question: "Is Noumena free to try?",
    answer: "Yes. Every new account gets a free 14-day trial — no credit card required. When your trial ends, you can upgrade to a paid plan or continue on the free tier."
  },
  {
    question: "Do I need technical skills to use Noumena?",
    answer: "Not at all. Just connect your YouTube account with one click, and we handle all the data ingestion and AI processing automatically."
  },
  {
    question: "How fast will I see results?",
    answer: "You'll get your first batch of actionable insights within 30 minutes of connecting your channel."
  },
  {
    question: "Is my channel data secure?",
    answer: "Yes. We use read-only access to your YouTube comments and never share your audience data with third parties."
  },
  {
    question: "What if I have multiple channels?",
    answer: "Our Creator plan and above support tracking multiple channels from a single dashboard, with independent insights for each."
  }
]
const openFaq = ref<number | null>(null)
const toggleFaq = (index: number) => {
  openFaq.value = openFaq.value === index ? null : index
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 dark:bg-zinc-950 font-sans text-zinc-900 dark:text-zinc-100 overflow-hidden relative selection:bg-brand-100 selection:text-brand-900">
    <!-- Ambient Decorators -->
    <div class="absolute top-0 right-0 -mt-32 -mr-32 w-96 h-96 bg-gradient-to-br from-brand-500 to-brand-400 rounded-full blur-[100px] opacity-20 pointer-events-none" data-unslop-ignore></div>
    <div class="absolute bottom-0 left-0 -mb-32 -ml-32 w-[30rem] h-[30rem] bg-gradient-to-tr from-emerald-500 to-teal-400 rounded-full blur-[100px] opacity-20 pointer-events-none" data-unslop-ignore></div>

    <!-- Navigation (Minimal for conversion) -->
    <nav class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 flex items-center justify-between">
      <div class="flex items-center gap-2">
        <div class="w-8 h-8 rounded-xl bg-brand-600 flex items-center justify-center text-white font-bold shadow-sm">
          N
        </div>
        <span class="text-xl font-bold tracking-tight text-zinc-900 dark:text-zinc-100">Noumena</span>
      </div>
      <div class="flex items-center gap-4">
        <RouterLink to="/login" class="text-sm font-medium text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors">Log in</RouterLink>
      </div>
    </nav>

    <!-- 1. Hero Section -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 pt-20 pb-24 grid lg:grid-cols-2 gap-12 lg:gap-16 items-center">
      <div class="max-w-xl">
        <h1 class="text-4xl sm:text-6xl font-bold tracking-tight text-zinc-900 dark:text-zinc-100 mb-6 leading-[1.1]">
          Stop guessing what your YouTube audience wants. <span class="text-brand-600 dark:text-brand-300">Know for sure.</span>
        </h1>
        <p class="text-lg text-zinc-500 dark:text-zinc-400 leading-relaxed mb-8">
          Noumena ingests your channel's comments, filters the noise, and surfaces the video ideas, questions, and pain points your audience is actually asking for — without you reading a single thread.
        </p>
        <button @click="getStarted" class="inline-flex justify-center items-center py-4 px-10 border border-transparent rounded-xl shadow-md text-lg font-bold text-white bg-zinc-900 hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 transition-colors duration-200 cursor-pointer">
          Start for Free
        </button>
        <p class="mt-4 text-sm text-zinc-500 dark:text-zinc-400 font-medium">Start your free 14-day trial — no credit card required.</p>
        <div class="mt-10 flex flex-wrap gap-x-10 gap-y-4">
          <div>
            <p class="font-mono tabular-nums text-2xl font-semibold text-zinc-900 dark:text-zinc-100">30 min</p>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">to first insights</p>
          </div>
          <div>
            <p class="font-mono tabular-nums text-2xl font-semibold text-zinc-900 dark:text-zinc-100">7</p>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">insight categories</p>
          </div>
          <div>
            <p class="font-mono tabular-nums text-2xl font-semibold text-zinc-900 dark:text-zinc-100">1-click</p>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">channel connect</p>
          </div>
        </div>
      </div>

      <!-- Real-data dashboard mockup -->
      <div class="bg-white dark:bg-zinc-900 rounded-xl shadow-2xl ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 overflow-hidden" aria-hidden="true">
        <!-- Mock header -->
        <div class="h-12 border-b border-zinc-200/50 dark:border-zinc-800/50 flex items-center px-4 gap-2 bg-white/80 dark:bg-zinc-900/80">
          <div class="w-3 h-3 rounded-full bg-rose-400"></div>
          <div class="w-3 h-3 rounded-full bg-amber-400"></div>
          <div class="w-3 h-3 rounded-full bg-emerald-400"></div>
          <span class="ml-2 text-xs font-medium text-zinc-400 font-mono">Noumena · Insights Feed</span>
        </div>
        <!-- Mock body -->
        <div class="p-6 space-y-5">
          <!-- Metric row -->
          <div class="grid grid-cols-3 gap-4">
            <div class="ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-4">
              <p class="text-[10px] font-medium uppercase tracking-wider text-zinc-400">Comments analyzed</p>
              <p class="mt-1 font-mono tabular-nums text-xl font-semibold text-zinc-900 dark:text-zinc-100">12,480</p>
            </div>
            <div class="ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-4">
              <p class="text-[10px] font-medium uppercase tracking-wider text-zinc-400">Actionable</p>
              <p class="mt-1 font-mono tabular-nums text-xl font-semibold text-brand-600 dark:text-brand-300">312</p>
            </div>
            <div class="ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-4">
              <p class="text-[10px] font-medium uppercase tracking-wider text-zinc-400">Noise filtered</p>
              <p class="mt-1 font-mono tabular-nums text-xl font-semibold text-emerald-600 dark:text-emerald-400">64%</p>
            </div>
          </div>
          <!-- Insight feed -->
          <div class="ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-4 space-y-3">
            <p class="text-[10px] font-medium uppercase tracking-wider text-zinc-400">Top insights today</p>
            <div class="flex items-center gap-3">
              <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-brand-50 text-brand-700 ring-1 ring-inset ring-brand-600/20 dark:bg-brand-950/40 dark:text-brand-300 dark:ring-brand-500/30">Video Idea</span>
              <p class="text-sm text-zinc-700 dark:text-zinc-300 truncate">"You should cover the new AI feature rollout"</p>
              <span class="ml-auto font-mono tabular-nums text-xs text-zinc-400">+47</span>
            </div>
            <div class="flex items-center gap-3">
              <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-600/20 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-500/30 data-unslop-ignore">Editing Tip</span>
              <p class="text-sm text-zinc-700 dark:text-zinc-300 truncate">"Pace feels too slow at the start"</p>
              <span class="ml-auto font-mono tabular-nums text-xs text-zinc-400">+29</span>
            </div>
            <div class="flex items-center gap-3">
              <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-emerald-50 text-emerald-700 ring-1 ring-inset ring-emerald-600/20 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-500/30">Question</span>
              <p class="text-sm text-zinc-700 dark:text-zinc-300 truncate">"Does this work for Shorts too?"</p>
              <span class="ml-auto font-mono tabular-nums text-xs text-zinc-400">+21</span>
            </div>
          </div>
          <div class="flex items-center justify-between text-[10px] font-mono text-zinc-400">
            <span>Last sync 12 min ago</span>
            <span>confidence 0.94</span>
          </div>
        </div>
      </div>
    </section>

    <!-- 2. Social Proof / Logos -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-10 border-t border-zinc-200/50 dark:border-zinc-800/50">
      <p class="text-center text-sm font-semibold text-zinc-400 dark:text-zinc-500 uppercase tracking-wider mb-8">Trusted by top creators growing their channels</p>
      <div class="flex flex-wrap justify-center items-center gap-8 sm:gap-16 opacity-50 grayscale">
         <span class="text-xl font-bold font-serif">CreatorStudio</span>
         <span class="text-xl font-bold tracking-tighter">VlogDaily.</span>
         <span class="text-xl font-black italic">StreamForce</span>
         <span class="text-xl font-bold text-zinc-600">MediaGroup</span>
         <span class="text-xl font-medium tracking-widest hidden sm:block">SYNDICATE</span>
      </div>
    </section>

    <!-- 3. Problem -> Solution Section -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-24 border-t border-zinc-200/50 dark:border-zinc-800/50">
       <div class="grid grid-cols-1 lg:grid-cols-2 gap-16 items-center">
          <div>
            <h2 class="text-3xl sm:text-4xl font-bold tracking-tight text-zinc-900 dark:text-zinc-100 mb-6">You can't read 10,000 comments. <span class="text-brand-600">But AI can.</span></h2>
            <p class="text-lg text-zinc-500 dark:text-zinc-400 leading-relaxed mb-8">
              Every day, creators leave massive opportunities on the table because it's physically impossible to manually read, categorize, and analyze thousands of comments.
            </p>
            <ul class="space-y-4">
              <li class="flex items-start gap-3">
                 <div class="mt-1 w-6 h-6 rounded-full bg-rose-50 flex items-center justify-center text-rose-500 shrink-0">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
                 </div>
                 <span class="text-zinc-700 dark:text-zinc-300 font-medium">Missing brilliant video ideas hidden deep in comment threads.</span>
              </li>
              <li class="flex items-start gap-3">
                 <div class="mt-1 w-6 h-6 rounded-full bg-rose-50 flex items-center justify-center text-rose-500 shrink-0">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
                 </div>
                 <span class="text-zinc-700 dark:text-zinc-300 font-medium">Getting overwhelmed by spam, toxicity, and irrelevant noise.</span>
              </li>
              <li class="flex items-start gap-3">
                 <div class="mt-1 w-6 h-6 rounded-full bg-emerald-50 flex items-center justify-center text-emerald-500 shrink-0">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>
                 </div>
                 <span class="text-zinc-900 dark:text-zinc-100 font-semibold">Noumena filters the noise and highlights exactly what your audience cares about.</span>
              </li>
            </ul>
          </div>
          <div class="bg-white dark:bg-zinc-900 rounded-xl ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 p-8 shadow-sm">
             <div class="space-y-6">
                <div class="flex items-start gap-4 pb-6 border-b border-zinc-100 dark:border-zinc-800">
                   <div class="w-10 h-10 rounded-full bg-brand-50 flex items-center justify-center text-brand-600 shrink-0">
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
                   </div>
                   <div>
                      <h4 class="font-bold text-zinc-900 dark:text-zinc-100">Outcome 1: Move Faster</h4>
                       <p class="text-sm text-zinc-500 dark:text-zinc-400 mt-1">Get instant insight extraction and categorization instead of spending hours reading manually.</p>
                   </div>
                </div>
                <div class="flex items-start gap-4 pb-6 border-b border-zinc-100 dark:border-zinc-800">
                   <div class="w-10 h-10 rounded-full bg-brand-50 flex items-center justify-center text-brand-600 shrink-0">
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"></path></svg>
                   </div>
                   <div>
                      <h4 class="font-bold text-zinc-900 dark:text-zinc-100">Outcome 2: Better Content Ideas</h4>
                      <p class="text-sm text-zinc-500 dark:text-zinc-400 mt-1">Automatically surface the most requested video topics and questions your audience is asking.</p>
                   </div>
                </div>
                <div class="flex items-start gap-4">
                   <div class="w-10 h-10 rounded-full bg-emerald-50 flex items-center justify-center text-emerald-600 shrink-0">
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"></path></svg>
                   </div>
                   <div>
                      <h4 class="font-bold text-zinc-900 dark:text-zinc-100">Outcome 3: Peace of Mind</h4>
                      <p class="text-sm text-zinc-500 dark:text-zinc-400 mt-1">Toxic comments, spam, and unhelpful noise are filtered out automatically before you ever see them.</p>
                   </div>
                </div>
             </div>
          </div>
       </div>
    </section>

    <!-- 4. Outcome-focused Features -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-24 bg-white/50 dark:bg-zinc-900/50 border-y border-zinc-200/50 dark:border-zinc-800/50">
      <div class="grid lg:grid-cols-2 gap-16 items-center">
        <div>
          <h2 class="text-3xl sm:text-4xl font-bold tracking-tight text-zinc-900 dark:text-zinc-100 mb-4">Everything you need to master your community</h2>
          <p class="text-lg text-zinc-500 dark:text-zinc-400 mb-12 max-w-xl">A suite built for creators who want to understand their audience at scale — without reading thousands of comments.</p>
          <div class="space-y-9">
            <div class="flex gap-4">
              <p class="font-mono tabular-nums text-sm font-semibold text-brand-600 dark:text-brand-300 w-10 shrink-0 pt-0.5">01</p>
              <div>
                <h3 class="text-lg font-bold text-zinc-900 dark:text-zinc-100 mb-1 tracking-tight">Never Miss a Beat</h3>
                <p class="text-sm text-zinc-500 dark:text-zinc-400 leading-relaxed">Connect your channel once and new comments are pulled automatically every 30 minutes.</p>
              </div>
            </div>
            <div class="flex gap-4">
              <p class="font-mono tabular-nums text-sm font-semibold text-brand-600 dark:text-brand-300 w-10 shrink-0 pt-0.5">02</p>
              <div>
                <h3 class="text-lg font-bold text-zinc-900 dark:text-zinc-100 mb-1 tracking-tight">AI does the Heavy Lifting</h3>
                <p class="text-sm text-zinc-500 dark:text-zinc-400 leading-relaxed">Comments categorized into 7 insight types — video ideas, questions, edits — so the signal surfaces without manual reading.</p>
              </div>
            </div>
            <div class="flex gap-4">
              <p class="font-mono tabular-nums text-sm font-semibold text-brand-600 dark:text-brand-300 w-10 shrink-0 pt-0.5">03</p>
              <div>
                <h3 class="text-lg font-bold text-zinc-900 dark:text-zinc-100 mb-1 tracking-tight">Act on Real Insights</h3>
                <p class="text-sm text-zinc-500 dark:text-zinc-400 leading-relaxed">Track themes over time and get a weekly digest of what your audience is asking for, before you plan your next video.</p>
              </div>
            </div>
          </div>
        </div>

        <!-- Weekly digest panel: real product data, not abstract icons -->
        <div class="bg-white dark:bg-zinc-900 shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-6" aria-hidden="true">
          <div class="flex items-start justify-between mb-5">
            <div>
              <p class="text-sm font-semibold text-zinc-900 dark:text-zinc-100">Weekly Digest</p>
              <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">Jan 5 — Jan 11</p>
            </div>
            <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-emerald-50 text-emerald-700 ring-1 ring-inset ring-emerald-600/20 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-500/30">+18% engagement</span>
          </div>
          <div class="space-y-4">
            <div>
              <div class="flex items-center justify-between mb-1.5">
                <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-brand-50 text-brand-700 ring-1 ring-inset ring-brand-600/20 dark:bg-brand-950/40 dark:text-brand-300 dark:ring-brand-500/30">Video Idea</span>
                <span class="font-mono tabular-nums text-xs text-zinc-500">64</span>
              </div>
              <div class="h-1.5 w-full bg-zinc-100 dark:bg-zinc-800 rounded-full overflow-hidden">
                <div class="h-full bg-brand-500 rounded-full data-unslop-ignore" style="width: 100%"></div>
              </div>
            </div>
            <div>
              <div class="flex items-center justify-between mb-1.5">
                <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-600/20 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-500/30 data-unslop-ignore">Editing Tip</span>
                <span class="font-mono tabular-nums text-xs text-zinc-500">41</span>
              </div>
              <div class="h-1.5 w-full bg-zinc-100 dark:bg-zinc-800 rounded-full overflow-hidden">
                <div class="h-full bg-amber-500 rounded-full data-unslop-ignore" style="width: 64%"></div>
              </div>
            </div>
            <div>
              <div class="flex items-center justify-between mb-1.5">
                <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-emerald-50 text-emerald-700 ring-1 ring-inset ring-emerald-600/20 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-500/30">Question</span>
                <span class="font-mono tabular-nums text-xs text-zinc-500">27</span>
              </div>
              <div class="h-1.5 w-full bg-zinc-100 dark:bg-zinc-800 rounded-full overflow-hidden">
                <div class="h-full bg-emerald-500 rounded-full data-unslop-ignore" style="width: 42%"></div>
              </div>
            </div>
          </div>
          <div class="mt-5 border-t border-zinc-100 dark:border-zinc-800 pt-4">
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mb-1">Top request this week</p>
            <p class="text-sm font-semibold text-zinc-900 dark:text-zinc-100 font-mono tabular-nums">"AI tutorial series" · 64 mentions</p>
          </div>
        </div>
      </div>
    </section>

    <!-- 5. Testimonials -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-24">
       <div class="text-center mb-16">
          <h2 class="text-3xl sm:text-4xl font-bold tracking-tight text-zinc-900 dark:text-zinc-100">Loved by creators like you</h2>
       </div>
        <div class="grid lg:grid-cols-2 gap-8 items-stretch max-w-5xl mx-auto">
           <div class="bg-zinc-900 dark:bg-zinc-800 p-8 sm:p-10 rounded-xl flex flex-col justify-between shadow-lg">
              <div class="mb-6 h-5 flex gap-1 text-amber-400">
                 <svg v-for="i in 5" :key="i" class="w-5 h-5 fill-current" viewBox="0 0 20 20"><path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"></path></svg>
              </div>
              <p class="text-lg sm:text-xl text-zinc-100 font-medium leading-relaxed mb-8">"Before Noumena, I spent hours every Sunday trying to figure out what my audience wanted next. Now, I just check my dashboard."</p>
              <div class="flex items-center gap-4">
                 <div class="w-12 h-12 rounded-full bg-zinc-700"></div>
                 <div>
                    <p class="text-sm font-bold text-white">Sarah Jenkins</p>
                    <p class="text-xs text-zinc-400">Tech Reviewer • 450k Subs</p>
                 </div>
                 <span class="ml-auto font-mono tabular-nums text-xs text-zinc-400 hidden sm:inline">10+ hrs / wk saved</span>
              </div>
           </div>
           <div class="flex flex-col gap-8">
              <div class="flex-1 bg-white dark:bg-zinc-900 p-8 rounded-xl shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 flex flex-col justify-between">
                 <div class="mb-6 h-5 flex gap-1 text-amber-400">
                    <svg v-for="i in 5" :key="i" class="w-5 h-5 fill-current" viewBox="0 0 20 20"><path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"></path></svg>
                 </div>
                 <p class="text-zinc-700 dark:text-zinc-300 leading-relaxed mb-6">"The toxicity filter alone is worth the price. Being able to see genuine questions without the background noise has completely changed how I interact with my community."</p>
                 <div class="flex items-center gap-4">
                    <div class="w-12 h-12 rounded-full bg-zinc-200 dark:bg-zinc-700"></div>
                    <div>
                       <p class="text-sm font-bold text-zinc-900 dark:text-zinc-100">Marcus T.</p>
                       <p class="text-xs text-zinc-500 dark:text-zinc-400">Gaming Channel • 1.2M Subs</p>
                    </div>
                 </div>
              </div>
              <div class="flex-1 bg-white dark:bg-zinc-900 p-8 rounded-xl shadow-sm ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 flex flex-col justify-between">
                 <div class="mb-6 h-5 flex gap-1 text-amber-400">
                    <svg v-for="i in 5" :key="i" class="w-5 h-5 fill-current" viewBox="0 0 20 20"><path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"></path></svg>
                 </div>
                 <p class="text-zinc-700 dark:text-zinc-300 leading-relaxed mb-6">"Using Noumena's weekly reports helped me realize my audience was begging for a specific tutorial series. I made it, and it's my highest performing playlist ever."</p>
                 <div class="flex items-center gap-4">
                    <div class="w-12 h-12 rounded-full bg-zinc-200 dark:bg-zinc-700"></div>
                    <div>
                       <p class="text-sm font-bold text-zinc-900 dark:text-zinc-100">Elena R.</p>
                       <p class="text-xs text-zinc-500 dark:text-zinc-400">Finance Educator • 800k Subs</p>
                    </div>
                 </div>
              </div>
           </div>
        </div>
     </section>


    <!-- 6. FAQ Section -->
    <section class="relative z-10 max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-24">
       <div class="text-center mb-12">
          <h2 class="text-3xl font-bold tracking-tight text-zinc-900 dark:text-zinc-100">Frequently Asked Questions</h2>
       </div>
       <div class="space-y-4">
          <div v-for="(faq, idx) in faqs" :key="idx" class="bg-white dark:bg-zinc-900 rounded-xl ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 overflow-hidden transition-all duration-200" :class="openFaq === idx ? 'shadow-md shadow-zinc-200 dark:shadow-zinc-950' : 'shadow-sm'">
             <button @click="toggleFaq(idx)" class="w-full text-left px-6 py-4 flex items-center justify-between focus:outline-none cursor-pointer">
                <span class="font-bold text-zinc-900 dark:text-zinc-100">{{ faq.question }}</span>
                <svg class="w-5 h-5 text-zinc-500 dark:text-zinc-400 transform transition-transform duration-200" :class="openFaq === idx ? 'rotate-180' : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path></svg>
             </button>
             <div style="display: grid;" :class="openFaq === idx ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'" class="transition-all duration-300">
                <div class="overflow-hidden">
                   <p class="px-6 pb-4 text-zinc-500 dark:text-zinc-400 text-sm leading-relaxed">{{ faq.answer }}</p>
                </div>
             </div>
          </div>
       </div>
    </section>

    <!-- 7. Final Strong CTA -->
    <section class="relative z-10 max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-24">
        <div class="bg-zinc-900 dark:bg-zinc-800 rounded-[2.5rem] p-12 sm:p-20 text-center relative overflow-hidden shadow-2xl">
          <!-- Abstract Background for CTA -->
          <div class="absolute inset-0 opacity-20 bg-gradient-to-br from-brand-500 to-brand-700 blur-[80px]"></div>
          <div class="relative z-10">
             <h2 class="text-4xl sm:text-5xl font-bold text-white tracking-tight mb-6">Ready to understand your audience?</h2>
             <p class="text-lg text-zinc-300 mb-10 max-w-2xl mx-auto">Join hundreds of creators who are using AI to build a stronger connection with their viewers and grow their channels.</p>
             <button @click="getStarted" class="inline-flex justify-center items-center py-4 px-10 border border-transparent rounded-xl text-lg font-bold text-zinc-900 dark:text-zinc-100 bg-white hover:bg-zinc-50 dark:bg-zinc-950 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-white transition-colors duration-200 cursor-pointer">
                Start for Free
             </button>
             <p class="mt-4 text-sm text-zinc-400 dark:text-zinc-500 font-medium">Start your free 14-day trial today — no credit card required.</p>
          </div>
       </div>
    </section>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-zinc-200/50 dark:border-zinc-800/50 py-12 bg-white dark:bg-zinc-900 mt-12">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 flex flex-col sm:flex-row justify-between items-center gap-4">
        <div class="flex items-center gap-2">
          <div class="w-6 h-6 rounded-lg bg-zinc-900 dark:bg-zinc-100 flex items-center justify-center text-white dark:text-zinc-900 text-xs font-bold shadow-sm">
            N
          </div>
          <span class="text-sm font-bold tracking-tight text-zinc-900 dark:text-zinc-100">Noumena</span>
        </div>
        <div class="flex items-center gap-6">
          <a href="#" class="text-xs text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors">Privacy Policy</a>
          <a href="#" class="text-xs text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors">Terms of Service</a>
          <a href="mailto:support@noumena.ai" class="text-xs text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors">Support</a>
        </div>
        <p class="text-xs text-zinc-400 dark:text-zinc-500">© 2026 Noumena SaaS. All rights reserved.</p>
      </div>
    </footer>
  </div>
</template>
