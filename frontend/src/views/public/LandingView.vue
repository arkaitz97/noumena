<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()

const getStarted = () => {
  router.push('/signup')
}

const faqs = [
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
    answer: "Our Pro plan supports tracking multiple channels from a single dashboard, with independent insights for each."
  }
]
const openFaq = ref<number | null>(null)
const toggleFaq = (index: number) => {
  openFaq.value = openFaq.value === index ? null : index
}
</script>

<template>
  <div class="min-h-screen bg-zinc-50 font-sans text-zinc-900 overflow-hidden relative selection:bg-indigo-100 selection:text-indigo-900">
    <!-- Ambient Decorators -->
    <div class="absolute top-0 right-0 -mt-32 -mr-32 w-96 h-96 bg-gradient-to-br from-indigo-500 to-purple-400 rounded-full blur-[100px] opacity-20 pointer-events-none"></div>
    <div class="absolute bottom-0 left-0 -mb-32 -ml-32 w-[30rem] h-[30rem] bg-gradient-to-tr from-emerald-500 to-teal-400 rounded-full blur-[100px] opacity-20 pointer-events-none"></div>

    <!-- Navigation (Minimal for conversion) -->
    <nav class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 flex items-center justify-between">
      <div class="flex items-center gap-2">
        <div class="w-8 h-8 rounded-xl bg-gradient-to-br from-indigo-500 to-purple-500 flex items-center justify-center text-white font-bold shadow-sm">
          N
        </div>
        <span class="text-xl font-bold tracking-tight text-zinc-900">Noumena</span>
      </div>
      <div class="flex items-center gap-4">
        <RouterLink to="/login" class="text-sm font-medium text-zinc-500 hover:text-zinc-900 transition-colors">Log in</RouterLink>
      </div>
    </nav>

    <!-- 1. Hero Section -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 pt-20 pb-20 flex flex-col items-center text-center">
      <h1 class="text-5xl sm:text-7xl font-bold tracking-tight text-zinc-900 mb-6 max-w-4xl leading-[1.1]">
        Stop guessing what your YouTube audience wants. <span class="text-transparent bg-clip-text bg-gradient-to-r from-indigo-500 to-purple-500 leading-[1.1]">Know for sure.</span>
      </h1>
      <p class="text-lg sm:text-xl text-zinc-500 leading-relaxed mb-10 max-w-2xl">
        Noumena automatically ingests, analyzes, and categorizes your channel's comments using state-of-the-art AI. Stop skimming through thousands of comments and let AI find the hidden gold.
      </p>
      
      <div class="flex flex-col items-center w-full sm:w-auto">
        <button @click="getStarted" class="w-full sm:w-auto inline-flex justify-center items-center py-4 px-10 border border-transparent rounded-xl shadow-md text-lg font-bold text-white bg-zinc-900 hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 transition-all duration-200 cursor-pointer">
          Start for Free
        </button>
        <p class="mt-4 text-sm text-zinc-500 font-medium">14-day free trial. No credit card required.</p>
      </div>

      <!-- Hero Mockup Abstract -->
      <div class="mt-20 w-full max-w-5xl mx-auto bg-white rounded-2xl shadow-2xl ring-1 ring-zinc-950/5 overflow-hidden flex flex-col items-center justify-center aspect-[16/9] sm:aspect-[21/9] relative">
         <div class="absolute inset-0 bg-zinc-50/50 flex flex-col">
            <!-- Mock Header -->
            <div class="h-12 border-b border-zinc-200/50 flex items-center px-4 gap-2 bg-white/80">
               <div class="w-3 h-3 rounded-full bg-rose-400"></div>
               <div class="w-3 h-3 rounded-full bg-amber-400"></div>
               <div class="w-3 h-3 rounded-full bg-emerald-400"></div>
            </div>
            <!-- Mock Body -->
            <div class="flex-1 p-6 flex gap-6">
                <!-- Sidebar -->
                <div class="w-48 hidden sm:flex flex-col gap-3">
                   <div class="h-8 bg-zinc-200/50 rounded-lg w-full"></div>
                   <div class="h-8 bg-zinc-200/50 rounded-lg w-3/4"></div>
                   <div class="h-8 bg-zinc-200/50 rounded-lg w-5/6"></div>
                </div>
                <!-- Main Content -->
                <div class="flex-1 flex flex-col gap-4">
                   <!-- Top metric row -->
                   <div class="flex gap-4 h-24">
                      <div class="flex-1 bg-white ring-1 ring-zinc-950/5 rounded-xl shadow-sm p-4 flex flex-col justify-between">
                         <div class="h-4 bg-zinc-100 rounded w-1/3"></div>
                         <div class="h-8 bg-indigo-50 rounded w-1/2"></div>
                      </div>
                      <div class="flex-1 bg-white ring-1 ring-zinc-950/5 rounded-xl shadow-sm p-4 flex flex-col justify-between">
                         <div class="h-4 bg-zinc-100 rounded w-1/3"></div>
                         <div class="h-8 bg-emerald-50 rounded w-1/2"></div>
                      </div>
                      <div class="flex-1 bg-white ring-1 ring-zinc-950/5 rounded-xl shadow-sm p-4 flex flex-col justify-between hidden md:flex">
                         <div class="h-4 bg-zinc-100 rounded w-1/3"></div>
                         <div class="h-8 bg-purple-50 rounded w-1/2"></div>
                      </div>
                   </div>
                   <!-- Feed block -->
                   <div class="flex-1 bg-white ring-1 ring-zinc-950/5 rounded-xl shadow-sm p-4 flex flex-col gap-3">
                      <div class="h-12 bg-zinc-50 rounded-lg w-full"></div>
                      <div class="h-12 bg-zinc-50 rounded-lg w-full"></div>
                      <div class="h-12 bg-zinc-50 rounded-lg w-3/4"></div>
                   </div>
                </div>
            </div>
         </div>
      </div>
    </section>

    <!-- 2. Social Proof / Logos -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-10 border-t border-zinc-200/50">
      <p class="text-center text-sm font-semibold text-zinc-400 uppercase tracking-wider mb-8">Trusted by top creators growing their channels</p>
      <div class="flex flex-wrap justify-center items-center gap-8 sm:gap-16 opacity-50 grayscale">
         <span class="text-xl font-bold font-serif">CreatorStudio</span>
         <span class="text-xl font-bold tracking-tighter">VlogDaily.</span>
         <span class="text-xl font-black italic">StreamForce</span>
         <span class="text-xl font-bold text-zinc-600">MediaGroup</span>
         <span class="text-xl font-medium tracking-widest hidden sm:block">SYNDICATE</span>
      </div>
    </section>

    <!-- 3. Problem -> Solution Section -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-24 border-t border-zinc-200/50">
       <div class="grid grid-cols-1 lg:grid-cols-2 gap-16 items-center">
          <div>
            <h2 class="text-3xl sm:text-4xl font-bold tracking-tight text-zinc-900 mb-6">You can't read 10,000 comments. <span class="text-indigo-600">But AI can.</span></h2>
            <p class="text-lg text-zinc-500 leading-relaxed mb-8">
              Every day, creators leave massive opportunities on the table because it's physically impossible to manually read, categorize, and analyze thousands of comments.
            </p>
            <ul class="space-y-4">
              <li class="flex items-start gap-3">
                 <div class="mt-1 w-6 h-6 rounded-full bg-rose-50 flex items-center justify-center text-rose-500 shrink-0">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
                 </div>
                 <span class="text-zinc-700 font-medium">Missing brilliant video ideas hidden deep in comment threads.</span>
              </li>
              <li class="flex items-start gap-3">
                 <div class="mt-1 w-6 h-6 rounded-full bg-rose-50 flex items-center justify-center text-rose-500 shrink-0">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
                 </div>
                 <span class="text-zinc-700 font-medium">Getting overwhelmed by spam, toxicity, and irrelevant noise.</span>
              </li>
              <li class="flex items-start gap-3">
                 <div class="mt-1 w-6 h-6 rounded-full bg-emerald-50 flex items-center justify-center text-emerald-500 shrink-0">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>
                 </div>
                 <span class="text-zinc-900 font-semibold">Noumena filters the noise and highlights exactly what your audience cares about.</span>
              </li>
            </ul>
          </div>
          <div class="bg-white rounded-2xl ring-1 ring-zinc-950/5 p-8 shadow-sm">
             <div class="space-y-6">
                <div class="flex items-start gap-4 pb-6 border-b border-zinc-100">
                   <div class="w-10 h-10 rounded-full bg-indigo-50 flex items-center justify-center text-indigo-600 shrink-0">
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
                   </div>
                   <div>
                      <h4 class="font-bold text-zinc-900">Outcome 1: Move Faster</h4>
                       <p class="text-sm text-zinc-500 mt-1">Get instant insight extraction and categorization instead of spending hours reading manually.</p>
                   </div>
                </div>
                <div class="flex items-start gap-4 pb-6 border-b border-zinc-100">
                   <div class="w-10 h-10 rounded-full bg-purple-50 flex items-center justify-center text-purple-600 shrink-0">
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"></path></svg>
                   </div>
                   <div>
                      <h4 class="font-bold text-zinc-900">Outcome 2: Better Content Ideas</h4>
                      <p class="text-sm text-zinc-500 mt-1">Automatically surface the most requested video topics and questions your audience is asking.</p>
                   </div>
                </div>
                <div class="flex items-start gap-4">
                   <div class="w-10 h-10 rounded-full bg-emerald-50 flex items-center justify-center text-emerald-600 shrink-0">
                      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"></path></svg>
                   </div>
                   <div>
                      <h4 class="font-bold text-zinc-900">Outcome 3: Peace of Mind</h4>
                      <p class="text-sm text-zinc-500 mt-1">Toxic comments, spam, and unhelpful noise are filtered out automatically before you ever see them.</p>
                   </div>
                </div>
             </div>
          </div>
       </div>
    </section>

    <!-- 4. Outcome-focused Features Grid -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-24 bg-white/50 border-y border-zinc-200/50">
      <div class="text-center mb-16">
        <h2 class="text-3xl sm:text-4xl font-bold tracking-tight text-zinc-900 mb-4">Everything you need to master your community</h2>
        <p class="text-lg text-zinc-500 max-w-2xl mx-auto">We've built a comprehensive suite of tools designed specifically for creators who want to understand their audience at scale.</p>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
        <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-8 hover:shadow-md transition-shadow duration-300 group">
          <div class="w-12 h-12 rounded-xl bg-indigo-50 flex items-center justify-center text-indigo-600 mb-6 group-hover:scale-110 transition-transform duration-300">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12"></path></svg>
          </div>
          <h3 class="text-xl font-bold text-zinc-900 mb-2 tracking-tight">Never Miss a Beat</h3>
          <p class="text-sm text-zinc-500 leading-relaxed">Connect your YouTube channel once and we'll automatically pull in new comments every 30 minutes seamlessly.</p>
        </div>

        <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-8 hover:shadow-md transition-shadow duration-300 group">
          <div class="w-12 h-12 rounded-xl bg-purple-50 flex items-center justify-center text-purple-600 mb-6 group-hover:scale-110 transition-transform duration-300">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z"></path></svg>
          </div>
          <h3 class="text-xl font-bold text-zinc-900 mb-2 tracking-tight">AI does the Heavy Lifting</h3>
          <p class="text-sm text-zinc-500 leading-relaxed">Our LLM engine categorizes comments, extracts actionable insights, and surfaces questions so you don't have to read them manually.</p>
        </div>

        <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-8 hover:shadow-md transition-shadow duration-300 group">
          <div class="w-12 h-12 rounded-xl bg-emerald-50 flex items-center justify-center text-emerald-600 mb-6 group-hover:scale-110 transition-transform duration-300">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path></svg>
          </div>
          <h3 class="text-xl font-bold text-zinc-900 mb-2 tracking-tight">Act on Real Insights</h3>
          <p class="text-sm text-zinc-500 leading-relaxed">Track actionable insight trends, get weekly digest emails, and discover common themes instantly.</p>
        </div>
      </div>
    </section>

    <!-- 5. Testimonials -->
    <section class="relative z-10 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-24">
       <div class="text-center mb-16">
          <h2 class="text-3xl sm:text-4xl font-bold tracking-tight text-zinc-900">Loved by creators like you</h2>
       </div>
       <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
          <div class="bg-white p-8 rounded-2xl shadow-sm ring-1 ring-zinc-950/5 flex flex-col justify-between hover:shadow-md transition-shadow">
             <div class="mb-6 h-5 flex gap-1 text-amber-400">
                <svg v-for="i in 5" :key="i" class="w-5 h-5 fill-current" viewBox="0 0 20 20"><path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"></path></svg>
             </div>
             <p class="text-zinc-700 italic mb-8">"Before Noumena, I spent hours every Sunday trying to figure out what my audience wanted next. Now, I just check my dashboard. It's saved me easily 10+ hours a week."</p>
             <div class="flex items-center gap-4">
                <div class="w-12 h-12 rounded-full bg-zinc-200"></div>
                <div>
                   <p class="text-sm font-bold text-zinc-900">Sarah Jenkins</p>
                   <p class="text-xs text-zinc-500">Tech Reviewer • 450k Subs</p>
                </div>
             </div>
          </div>
          <div class="bg-white p-8 rounded-2xl shadow-sm ring-1 ring-zinc-950/5 flex flex-col justify-between hover:shadow-md transition-shadow">
             <div class="mb-6 h-5 flex gap-1 text-amber-400">
                <svg v-for="i in 5" :key="i" class="w-5 h-5 fill-current" viewBox="0 0 20 20"><path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"></path></svg>
             </div>
             <p class="text-zinc-700 italic mb-8">"The toxicity filter alone is worth the price. Being able to see genuine questions without the background noise has completely changed how I interact with my community."</p>
             <div class="flex items-center gap-4">
                <div class="w-12 h-12 rounded-full bg-zinc-200"></div>
                <div>
                   <p class="text-sm font-bold text-zinc-900">Marcus T.</p>
                   <p class="text-xs text-zinc-500">Gaming Channel • 1.2M Subs</p>
                </div>
             </div>
          </div>
          <div class="bg-white p-8 rounded-2xl shadow-sm ring-1 ring-zinc-950/5 flex flex-col justify-between hover:shadow-md transition-shadow">
             <div class="mb-6 h-5 flex gap-1 text-amber-400">
                <svg v-for="i in 5" :key="i" class="w-5 h-5 fill-current" viewBox="0 0 20 20"><path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"></path></svg>
             </div>
             <p class="text-zinc-700 italic mb-8">"Using Noumena's weekly reports helped me realize my audience was begging for a specific tutorial series. I made it, and it's my highest performing playlist ever."</p>
             <div class="flex items-center gap-4">
                <div class="w-12 h-12 rounded-full bg-zinc-200"></div>
                <div>
                   <p class="text-sm font-bold text-zinc-900">Elena R.</p>
                   <p class="text-xs text-zinc-500">Finance Educator • 800k Subs</p>
                </div>
             </div>
          </div>
       </div>
    </section>

    <!-- 6. FAQ Section -->
    <section class="relative z-10 max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-24">
       <div class="text-center mb-12">
          <h2 class="text-3xl font-bold tracking-tight text-zinc-900">Frequently Asked Questions</h2>
       </div>
       <div class="space-y-4">
          <div v-for="(faq, idx) in faqs" :key="idx" class="bg-white rounded-xl ring-1 ring-zinc-950/5 overflow-hidden transition-all duration-200" :class="openFaq === idx ? 'shadow-md shadow-zinc-200' : 'shadow-sm'">
             <button @click="toggleFaq(idx)" class="w-full text-left px-6 py-4 flex items-center justify-between focus:outline-none cursor-pointer">
                <span class="font-bold text-zinc-900">{{ faq.question }}</span>
                <svg class="w-5 h-5 text-zinc-500 transform transition-transform duration-200" :class="openFaq === idx ? 'rotate-180' : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"></path></svg>
             </button>
             <div style="display: grid;" :class="openFaq === idx ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'" class="transition-all duration-300">
                <div class="overflow-hidden">
                   <p class="px-6 pb-4 text-zinc-500 text-sm leading-relaxed">{{ faq.answer }}</p>
                </div>
             </div>
          </div>
       </div>
    </section>

    <!-- 7. Final Strong CTA -->
    <section class="relative z-10 max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-24">
       <div class="bg-zinc-900 rounded-[2.5rem] p-12 sm:p-20 text-center relative overflow-hidden shadow-2xl">
          <!-- Abstract Background for CTA -->
          <div class="absolute inset-0 opacity-20 bg-gradient-to-br from-indigo-500 to-emerald-500 blur-[80px]"></div>
          <div class="relative z-10">
             <h2 class="text-4xl sm:text-5xl font-bold text-white tracking-tight mb-6">Ready to understand your audience?</h2>
             <p class="text-lg text-zinc-300 mb-10 max-w-2xl mx-auto">Join hundreds of creators who are using AI to build a stronger connection with their viewers and grow their channels.</p>
             <button @click="getStarted" class="inline-flex justify-center items-center py-4 px-10 border border-transparent rounded-xl shadow-[0_0_20px_rgba(255,255,255,0.1)] text-lg font-bold text-zinc-900 bg-white hover:bg-zinc-50 hover:scale-[1.02] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-white transition-all duration-200 cursor-pointer">
               Start your 14-day Free Trial
             </button>
             <p class="mt-4 text-sm text-zinc-400 font-medium">No credit card required to start.</p>
          </div>
       </div>
    </section>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-zinc-200/50 py-12 bg-white mt-12">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 flex flex-col sm:flex-row justify-between items-center gap-4">
        <div class="flex items-center gap-2">
          <div class="w-6 h-6 rounded-lg bg-zinc-900 flex items-center justify-center text-white text-xs font-bold shadow-sm">
            N
          </div>
          <span class="text-sm font-bold tracking-tight text-zinc-900">Noumena</span>
        </div>
        <div class="flex items-center gap-6">
          <a href="#" class="text-xs text-zinc-500 hover:text-zinc-900 transition-colors">Privacy Policy</a>
          <a href="#" class="text-xs text-zinc-500 hover:text-zinc-900 transition-colors">Terms of Service</a>
          <a href="mailto:support@noumena.ai" class="text-xs text-zinc-500 hover:text-zinc-900 transition-colors">Support</a>
        </div>
        <p class="text-xs text-zinc-400">© 2026 Noumena SaaS. All rights reserved.</p>
      </div>
    </footer>
  </div>
</template>
