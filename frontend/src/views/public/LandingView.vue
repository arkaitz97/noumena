<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import AppLogo from '../../components/AppLogo.vue'

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
  <div class="min-h-screen bg-zinc-50 dark:bg-zinc-950 font-sans text-zinc-900 dark:text-zinc-100 overflow-hidden relative">
    <!-- Ambient Decorators -->
    <div class="absolute top-[-10rem] right-[-8rem] w-[36rem] h-[36rem] bg-gradient-to-br from-brand-500/30 to-brand-300/10 rounded-full blur-[110px] pointer-events-none"></div>
    <div class="absolute top-[46rem] left-[-12rem] w-[32rem] h-[32rem] bg-gradient-to-tr from-brand-600/15 via-emerald-400/10 to-transparent rounded-full blur-[110px] pointer-events-none"></div>

    <!-- Navigation (Minimal for conversion) -->
    <nav class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 flex items-center justify-between">
      <AppLogo size="md" show-text />
      <div class="flex items-center gap-4">
        <RouterLink
          to="/login"
          class="text-sm font-semibold text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-white transition-colors duration-200 rounded-lg px-3 py-2 hover:bg-zinc-100/80 dark:hover:bg-white/5 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-500/50"
          >Log in</RouterLink
        >
      </div>
    </nav>

    <!-- 1. Hero Section -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 pt-20 sm:pt-24 pb-28 grid lg:grid-cols-[1.05fr_0.95fr] gap-16 lg:gap-12 items-center">
      <div class="max-w-xl">
        <p class="rise inline-flex items-center gap-2.5 rounded-md bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 px-3.5 py-1.5 text-xs font-semibold uppercase tracking-[0.2em] text-zinc-500 dark:text-zinc-400 mb-7">
          <span class="relative flex h-1.5 w-1.5">
            <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-brand-400 opacity-60"></span>
            <span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-brand-500"></span>
          </span>
          Comment intelligence
        </p>
        <h1 class="rise d2 text-display-lg font-extrabold text-zinc-900 dark:text-white mb-6">
          Stop guessing what your YouTube audience wants. <span class="text-brand-600 dark:text-brand-400">Know for sure.</span>
        </h1>
        <p class="rise d3 text-lg text-zinc-500 dark:text-zinc-400 leading-relaxed mb-10 max-w-lg">
          Noumena ingests your channel's comments, filters the noise, and surfaces the video ideas, questions, and pain points your audience is actually asking for — without you reading a single thread.
        </p>
        <div class="rise d4 flex flex-wrap items-center gap-x-10 gap-y-6">
          <button
            @click="getStarted"
            class="inline-flex justify-center items-center py-4 px-9 rounded-xl text-lg font-bold text-white bg-zinc-900 hover:bg-zinc-800 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200 shadow-pop dark:shadow-card transition-all duration-300 ease-snappy hover:-translate-y-0.5 dark:hover:shadow-card active:translate-y-0 active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-zinc-50 dark:focus-visible:ring-offset-zinc-950 cursor-pointer"
          >
            Start for Free
          </button>
          <div class="flex flex-wrap gap-x-10 gap-y-4">
            <div>
              <p class="font-mono tabular text-2xl font-semibold tracking-tight text-zinc-900 dark:text-white">30 min</p>
              <p class="eyebrow mt-1.5">to first insights</p>
            </div>
            <div>
              <p class="font-mono tabular text-2xl font-semibold tracking-tight text-zinc-900 dark:text-white">7</p>
              <p class="eyebrow mt-1.5">insight categories</p>
            </div>
            <div>
              <p class="font-mono tabular text-2xl font-semibold tracking-tight text-zinc-900 dark:text-white">1-click</p>
              <p class="eyebrow mt-1.5">channel connect</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Real-data dashboard mockup -->
      <div class="rise d5 relative lg:translate-y-3" aria-hidden="true">
        <div class="absolute -inset-10 rounded-full bg-gradient-to-br from-brand-500/20 via-brand-400/10 to-transparent blur-3xl pointer-events-none"></div>
        <div class="relative bg-white dark:bg-zinc-900 rounded-xl shadow-pop ring-1 ring-zinc-950/5 dark:ring-white/10 overflow-hidden">
          <!-- Mock header -->
          <div class="h-12 border-b border-zinc-200/50 dark:border-white/5 flex items-center px-4 gap-2 bg-white/80 dark:bg-zinc-900/80">
            <div class="w-3 h-3 rounded-full bg-rose-400"></div>
            <div class="w-3 h-3 rounded-full bg-amber-400"></div>
            <div class="w-3 h-3 rounded-full bg-emerald-400"></div>
            <span class="ml-2 text-xs font-medium text-zinc-400 font-mono">Noumena · Insights Feed</span>
          </div>
          <!-- Mock body -->
          <div class="p-6 space-y-5">
            <!-- Metric row -->
            <div class="grid grid-cols-3 gap-4">
              <div class="bg-zinc-50/80 dark:bg-white/[0.04] ring-1 ring-zinc-950/5 dark:ring-white/5 rounded-xl p-4">
                <p class="text-[10px] font-medium uppercase tracking-wider text-zinc-400">Comments analyzed</p>
                <p class="mt-1 font-mono tabular-nums text-xl font-semibold text-zinc-900 dark:text-white">12,480</p>
              </div>
              <div class="bg-zinc-50/80 dark:bg-white/[0.04] ring-1 ring-zinc-950/5 dark:ring-white/5 rounded-xl p-4">
                <p class="text-[10px] font-medium uppercase tracking-wider text-zinc-400">Actionable</p>
                <p class="mt-1 font-mono tabular-nums text-xl font-semibold text-brand-600 dark:text-brand-400">312</p>
              </div>
              <div class="bg-zinc-50/80 dark:bg-white/[0.04] ring-1 ring-zinc-950/5 dark:ring-white/5 rounded-xl p-4">
                <p class="text-[10px] font-medium uppercase tracking-wider text-zinc-400">Noise filtered</p>
                <p class="mt-1 font-mono tabular-nums text-xl font-semibold text-emerald-600 dark:text-emerald-400">64%</p>
              </div>
            </div>
            <!-- Insight feed -->
            <div class="bg-zinc-50/80 dark:bg-white/[0.04] ring-1 ring-zinc-950/5 dark:ring-white/5 rounded-xl p-4 space-y-3">
            <p class="text-[10px] font-medium uppercase tracking-wider text-zinc-400">Top insights today</p>
            <div class="flex items-center gap-3">
              <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-brand-50 text-brand-700 ring-1 ring-inset ring-brand-600/20 dark:bg-brand-950/40 dark:text-brand-300 dark:ring-brand-500/30">Video Idea</span>
              <p class="text-sm text-zinc-700 dark:text-zinc-300 truncate">"You should cover the new AI feature rollout"</p>
              <span class="ml-auto font-mono tabular-nums text-xs text-zinc-400">+47</span>
            </div>
            <div class="flex items-center gap-3">
              <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-600/20 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-500/30">Editing Tip</span>
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
      </div>
    </section>

    <!-- 2. Social Proof / Logos -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12 border-t border-zinc-200/60 dark:border-white/5">
      <p class="text-center text-xs font-semibold text-zinc-400 dark:text-zinc-500 uppercase tracking-[0.2em] mb-10">Trusted by top creators growing their channels</p>
      <div class="flex flex-wrap justify-center items-center gap-x-12 gap-y-6 text-zinc-400 dark:text-zinc-500">
        <span class="flex items-center gap-2 text-[15px] font-semibold tracking-tight">
          <svg class="w-4 h-4 fill-current" viewBox="0 0 24 24"><path d="M12 2l10 18.5H2L12 2z" /></svg>
          CreatorStudio
        </span>
        <span class="flex items-center gap-2 text-[15px] font-semibold tracking-tight">
          <svg class="w-4 h-4 fill-current" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10" /></svg>
          VlogDaily
        </span>
        <span class="flex items-center gap-2 text-[15px] font-bold tracking-tight">
          <svg class="w-4 h-4 fill-current" viewBox="0 0 24 24"><path d="M13 2L5 14h5l-2 8 9-13.5h-5.5L13 2z" /></svg>
          StreamForce
        </span>
        <span class="flex items-center gap-2 text-[15px] font-semibold tracking-tight">
          <svg class="w-4 h-4 fill-current" viewBox="0 0 24 24"><rect x="6.5" y="6.5" width="11" height="11" rx="2" transform="rotate(45 12 12)" /></svg>
          MediaGroup
        </span>
        <span class="flex items-center gap-2 text-[15px] font-semibold tracking-[0.14em] uppercase">
          <svg class="w-4 h-4 fill-none stroke-current" stroke-width="2" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" /><circle cx="12" cy="12" r="3" fill="currentColor" /></svg>
          Syndicate
        </span>
      </div>
    </section>

    <!-- 3. Problem -> Solution Section -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-32 sm:py-40 border-t border-zinc-200/60 dark:border-white/5">
       <div class="grid grid-cols-1 lg:grid-cols-[6fr_5fr] gap-16 lg:gap-24 items-center">
          <div>
            <p class="eyebrow mb-4">The problem</p>
            <h2 class="text-display font-bold text-zinc-900 dark:text-white mb-6">
              You can't read 10,000 comments. <span class="text-brand-600 dark:text-brand-400">But AI can.</span>
            </h2>
            <p class="text-lg text-zinc-500 dark:text-zinc-400 leading-relaxed mb-10 max-w-xl">
              Every day, creators leave massive opportunities on the table because it's physically impossible to manually read, categorize, and analyze thousands of comments.
            </p>
            <ul class="space-y-4">
              <li class="flex items-start gap-3">
                 <div class="mt-1 w-6 h-6 rounded-full bg-rose-50 dark:bg-rose-500/10 flex items-center justify-center text-rose-500 dark:text-rose-400 shrink-0">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
                 </div>
                 <span class="text-zinc-700 dark:text-zinc-300 font-medium">Missing brilliant video ideas hidden deep in comment threads.</span>
              </li>
              <li class="flex items-start gap-3">
                 <div class="mt-1 w-6 h-6 rounded-full bg-rose-50 dark:bg-rose-500/10 flex items-center justify-center text-rose-500 dark:text-rose-400 shrink-0">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
                 </div>
                 <span class="text-zinc-700 dark:text-zinc-300 font-medium">Getting overwhelmed by spam, toxicity, and irrelevant noise.</span>
              </li>
              <li class="flex items-start gap-3">
                 <div class="mt-1 w-6 h-6 rounded-full bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center text-emerald-500 dark:text-emerald-400 shrink-0">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>
                 </div>
                 <span class="text-zinc-900 dark:text-white font-semibold">Noumena filters the noise and highlights exactly what your audience cares about.</span>
              </li>
            </ul>
          </div>
          <div class="bg-white dark:bg-zinc-900 rounded-xl ring-1 ring-zinc-950/5 dark:ring-white/10 p-8 shadow-card">
             <div class="space-y-6">
                <div class="flex items-start gap-4 pb-6 border-b border-zinc-100 dark:border-white/5">
                   <div class="w-10 h-10 rounded-xl bg-brand-50 dark:bg-brand-400/10 flex items-center justify-center text-brand-600 dark:text-brand-400 shrink-0 ring-1 ring-brand-600/10 dark:ring-brand-400/20">
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
                   </div>
                   <div>
                      <h4 class="font-bold text-zinc-900 dark:text-white">Outcome 1: Move Faster</h4>
                       <p class="text-sm text-zinc-500 dark:text-zinc-400 mt-1">Get instant insight extraction and categorization instead of spending hours reading manually.</p>
                   </div>
                </div>
                <div class="flex items-start gap-4 pb-6 border-b border-zinc-100 dark:border-white/5">
                   <div class="w-10 h-10 rounded-xl bg-brand-50 dark:bg-brand-400/10 flex items-center justify-center text-brand-600 dark:text-brand-400 shrink-0 ring-1 ring-brand-600/10 dark:ring-brand-400/20">
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"></path></svg>
                   </div>
                   <div>
                      <h4 class="font-bold text-zinc-900 dark:text-white">Outcome 2: Better Content Ideas</h4>
                      <p class="text-sm text-zinc-500 dark:text-zinc-400 mt-1">Automatically surface the most requested video topics and questions your audience is asking.</p>
                   </div>
                </div>
                <div class="flex items-start gap-4">
                   <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center text-emerald-600 dark:text-emerald-400 shrink-0 ring-1 ring-emerald-600/10 dark:ring-emerald-400/20">
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"></path></svg>
                   </div>
                   <div>
                      <h4 class="font-bold text-zinc-900 dark:text-white">Outcome 3: Peace of Mind</h4>
                      <p class="text-sm text-zinc-500 dark:text-zinc-400 mt-1">Toxic comments, spam, and unhelpful noise are filtered out automatically before you ever see them.</p>
                   </div>
                </div>
             </div>
          </div>
       </div>
    </section>

    <!-- 4. Outcome-focused Features -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-28 sm:py-36 bg-white/50 dark:bg-zinc-900/50 border-y border-zinc-200/50 dark:border-zinc-800/50">
      <div class="grid lg:grid-cols-2 gap-16 lg:gap-24 items-center">
        <div>
          <p class="eyebrow mb-4">The system</p>
          <h2 class="text-display font-bold text-zinc-900 dark:text-zinc-100 mb-5">Everything you need to master your community</h2>
          <p class="text-lg text-zinc-500 dark:text-zinc-400 mb-14 max-w-xl">A suite built for creators who want to understand their audience at scale — without reading thousands of comments.</p>
          <div class="space-y-10">
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
        <div class="bg-white dark:bg-zinc-900 shadow-pop ring-1 ring-zinc-950/5 dark:ring-zinc-100/10 rounded-xl p-6 sm:p-8 lg:mt-14" aria-hidden="true">
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
                <div class="h-full bg-brand-500 rounded-full" style="width: 100%"></div>
              </div>
            </div>
            <div>
              <div class="flex items-center justify-between mb-1.5">
                <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-600/20 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-500/30">Editing Tip</span>
                <span class="font-mono tabular-nums text-xs text-zinc-500">41</span>
              </div>
              <div class="h-1.5 w-full bg-zinc-100 dark:bg-zinc-800 rounded-full overflow-hidden">
                <div class="h-full bg-amber-500 rounded-full" style="width: 64%"></div>
              </div>
            </div>
            <div>
              <div class="flex items-center justify-between mb-1.5">
                <span class="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-semibold bg-emerald-50 text-emerald-700 ring-1 ring-inset ring-emerald-600/20 dark:bg-emerald-950/40 dark:text-emerald-300 dark:ring-emerald-500/30">Question</span>
                <span class="font-mono tabular-nums text-xs text-zinc-500">27</span>
              </div>
              <div class="h-1.5 w-full bg-zinc-100 dark:bg-zinc-800 rounded-full overflow-hidden">
                <div class="h-full bg-emerald-500 rounded-full" style="width: 42%"></div>
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
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-28 sm:py-36">
       <div class="max-w-3xl mx-auto text-center mb-16">
          <p class="eyebrow mb-4">Proof</p>
          <h2 class="text-display font-bold text-zinc-900 dark:text-zinc-100">Loved by creators like you</h2>
       </div>
        <div class="grid lg:grid-cols-2 gap-8 items-stretch max-w-5xl mx-auto">
           <div class="bg-zinc-900 dark:bg-zinc-800 p-8 sm:p-10 rounded-xl flex flex-col justify-between shadow-pop ring-1 ring-white/10">
              <div class="mb-6 h-5 flex gap-1 text-amber-400">
                 <svg v-for="i in 5" :key="i" class="w-5 h-5 fill-current" viewBox="0 0 20 20"><path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"></path></svg>
              </div>
              <p class="text-lg sm:text-xl text-zinc-100 font-medium leading-relaxed mb-8">"Before Noumena, I spent hours every Sunday trying to figure out what my audience wanted next. Now, I just check my dashboard."</p>
               <div class="flex items-center gap-4">
                  <div class="w-12 h-12 rounded-full bg-gradient-to-br from-brand-400 to-brand-600 flex items-center justify-center text-sm font-bold text-white ring-1 ring-white/20">SJ</div>
                  <div>
                     <p class="text-sm font-bold text-white">Sarah Jenkins</p>
                     <p class="text-xs text-zinc-400">Tech Reviewer • 450k Subs</p>
                  </div>
                  <span class="ml-auto font-mono tabular-nums text-xs text-zinc-400 hidden sm:inline">10+ hrs / wk saved</span>
               </div>
            </div>
            <div class="flex flex-col gap-8 lg:mt-14">
               <div class="flex-1 bg-white dark:bg-zinc-900 p-8 rounded-xl shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 flex flex-col justify-between">
                  <div class="mb-6 h-5 flex gap-1 text-amber-400">
                     <svg v-for="i in 5" :key="i" class="w-5 h-5 fill-current" viewBox="0 0 20 20"><path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"></path></svg>
                  </div>
                  <p class="text-zinc-700 dark:text-zinc-300 leading-relaxed mb-6">"The toxicity filter alone is worth the price. Being able to see genuine questions without the background noise has completely changed how I interact with my community."</p>
                  <div class="flex items-center gap-4">
                     <div class="w-12 h-12 rounded-full bg-gradient-to-br from-zinc-500 to-zinc-700 flex items-center justify-center text-sm font-bold text-white ring-1 ring-white/10">MT</div>
                     <div>
                        <p class="text-sm font-bold text-zinc-900 dark:text-white">Marcus T.</p>
                        <p class="text-xs text-zinc-500 dark:text-zinc-400">Gaming Channel • 1.2M Subs</p>
                     </div>
                  </div>
               </div>
               <div class="flex-1 bg-white dark:bg-zinc-900 p-8 rounded-xl shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 flex flex-col justify-between">
                  <div class="mb-6 h-5 flex gap-1 text-amber-400">
                     <svg v-for="i in 5" :key="i" class="w-5 h-5 fill-current" viewBox="0 0 20 20"><path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"></path></svg>
                  </div>
                  <p class="text-zinc-700 dark:text-zinc-300 leading-relaxed mb-6">"Using Noumena's weekly reports helped me realize my audience was begging for a specific tutorial series. I made it, and it's my highest performing playlist ever."</p>
                  <div class="flex items-center gap-4">
                     <div class="w-12 h-12 rounded-full bg-gradient-to-br from-emerald-500 to-teal-700 flex items-center justify-center text-sm font-bold text-white ring-1 ring-white/20">ER</div>
                     <div>
                        <p class="text-sm font-bold text-zinc-900 dark:text-white">Elena R.</p>
                        <p class="text-xs text-zinc-500 dark:text-zinc-400">Finance Educator • 800k Subs</p>
                     </div>
                  </div>
               </div>
            </div>
         </div>
      </section>


    <!-- 6. FAQ Section -->
    <section class="relative z-10 max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-28 sm:py-36">
       <div class="text-center mb-14">
          <p class="eyebrow mb-4">Answers</p>
          <h2 class="text-display font-bold text-zinc-900 dark:text-zinc-100">Frequently asked questions</h2>
       </div>
       <div class="space-y-4">
          <div v-for="(faq, idx) in faqs" :key="idx" class="bg-white dark:bg-zinc-900 rounded-xl ring-1 ring-zinc-950/5 dark:ring-white/10 overflow-hidden transition-all duration-300 ease-snappy" :class="openFaq === idx ? 'shadow-pop' : 'shadow-card'">
             <button @click="toggleFaq(idx)" class="w-full text-left px-6 sm:px-7 py-[1.15rem] flex items-center justify-between gap-4 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-500/40 cursor-pointer">
                <span class="font-semibold text-zinc-900 dark:text-white">{{ faq.question }}</span>
                <span class="w-6 h-6 rounded-full bg-zinc-100 dark:bg-white/5 flex items-center justify-center shrink-0 transition-all duration-300 ease-snappy" :class="openFaq === idx ? 'bg-brand-50 dark:bg-brand-400/10 rotate-180' : ''">
                  <svg class="w-3.5 h-3.5 text-zinc-500 dark:text-zinc-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path></svg>
                </span>
             </button>
             <div style="display: grid;" :class="openFaq === idx ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'" class="transition-all duration-300 ease-snappy">
                <div class="overflow-hidden">
                   <p class="px-6 sm:px-7 pb-5 text-zinc-500 dark:text-zinc-400 text-sm leading-relaxed max-w-[62ch]">{{ faq.answer }}</p>
                </div>
             </div>
          </div>
       </div>
    </section>

    <!-- 7. Final Strong CTA -->
    <section class="relative z-10 max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-24">
        <div class="bg-zinc-900 dark:bg-zinc-900 rounded-xl p-12 sm:p-20 text-center relative overflow-hidden shadow-pop ring-1 ring-white/10">
          <!-- Abstract Background for CTA -->
          <div class="absolute -top-24 left-1/2 -translate-x-1/2 w-[42rem] h-[24rem] opacity-25 bg-gradient-to-br from-brand-400 to-brand-600 rounded-full blur-[90px] pointer-events-none"></div>
          <div class="absolute -bottom-32 -right-20 w-80 h-80 opacity-15 bg-brand-500 rounded-full blur-[100px] pointer-events-none"></div>
          <div class="relative z-10">
             <p class="eyebrow mb-5 text-zinc-400">Begin</p>
             <h2 class="text-display-lg font-extrabold text-white tracking-tight mb-6">Ready to understand your audience?</h2>
             <p class="text-lg text-zinc-300 mb-10 max-w-2xl mx-auto">Join hundreds of creators who are using AI to build a stronger connection with their viewers and grow their channels.</p>
             <button
                @click="getStarted"
                class="inline-flex justify-center items-center py-4 px-10 rounded-xl text-lg font-bold text-zinc-900 bg-white hover:bg-zinc-100 shadow-pop transition-all duration-300 ease-snappy hover:-translate-y-0.5 active:translate-y-0 active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-brand-300 focus-visible:ring-offset-zinc-900 cursor-pointer"
             >
                Start for Free
             </button>
             <p class="mt-4 text-sm text-zinc-400 font-medium">Start your free 14-day trial — no credit card required.</p>
          </div>
       </div>
    </section>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-zinc-200/60 dark:border-white/5 py-12 bg-white dark:bg-zinc-900">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 flex flex-col sm:flex-row justify-between items-center gap-4">
        <AppLogo size="xs" show-text text-class="text-sm font-bold tracking-tight text-zinc-900 dark:text-white" />
        <div class="flex items-center gap-6">
          <a href="#" class="text-xs font-medium text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-white transition-colors">Privacy Policy</a>
          <a href="#" class="text-xs font-medium text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-white transition-colors">Terms of Service</a>
          <a href="mailto:support@noumena.ai" class="text-xs font-medium text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-white transition-colors">Support</a>
        </div>
        <p class="text-xs text-zinc-400 dark:text-zinc-500">© 2026 Noumena. All rights reserved.</p>
      </div>
    </footer>
  </div>
</template>

<style scoped>
/* Stagger utilities (.rise, .d2–.d5) are global — see style.css. */
</style>
