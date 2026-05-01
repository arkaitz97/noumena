<script setup lang="ts">
const redirectToStripe = () => {
  // Hit the backend to create a billing portal session, then redirect to the returned URL
  window.location.href = '/api/billing/portal'
}
</script>

<template>
  <div class="space-y-10">
    <div class="px-4 sm:px-0">
      <h3 class="text-2xl font-semibold leading-7 text-zinc-900 tracking-tight">Billing & Subscription</h3>
      <p class="mt-2 text-sm text-zinc-500">Manage your subscription plan, payment methods, and billing history.</p>
    </div>

    <!-- Active Plan Card -->
    <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 sm:rounded-2xl overflow-hidden relative">
      <!-- Decorator -->
      <div class="absolute top-0 right-0 -mt-16 -mr-16 w-32 h-32 bg-gradient-to-br from-indigo-500 to-blue-400 rounded-full blur-[60px] opacity-30"></div>
      
      <div class="p-6 sm:p-8 relative z-10 flex flex-col sm:flex-row justify-between items-start sm:items-center border-b border-zinc-100">
        <div>
          <div class="flex items-center gap-3 mb-1">
            <h4 class="text-xl font-semibold text-zinc-900 tracking-tight">Creator Plan</h4>
            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold tracking-wide bg-emerald-50 text-emerald-700 ring-1 ring-inset ring-emerald-600/20">
              Active
            </span>
          </div>
          <p class="text-sm text-zinc-500">
            $29/month &bull; Up to 3 channels &amp; 50,000 comments/month
          </p>
        </div>
        <div class="mt-4 sm:mt-0 text-right">
          <p class="text-3xl font-bold tracking-tight text-zinc-900">$29<span class="text-base font-medium text-zinc-500">/mo</span></p>
        </div>
      </div>
      
      <div class="px-6 py-6 sm:p-8 bg-zinc-50/30 relative z-10 border-b border-zinc-100">
        <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-8 gap-y-8 text-sm">
          <div>
            <dt class="font-medium text-zinc-500 mb-1">Next billing date</dt>
            <dd class="text-zinc-900 font-semibold">April 24, 2026</dd>
          </div>
          <div>
            <dt class="font-medium text-zinc-500 mb-1">Payment method</dt>
            <dd class="flex items-center text-zinc-900 font-semibold gap-2">
              <svg class="h-6 w-auto" fill="none" viewBox="0 0 36 24"><rect width="36" height="24" rx="4" fill="#1A1F36"/><path d="M14.075 16H11.23L13.06 8H15.654L14.075 16ZM22.617 8H20.219C19.829 8 19.467 8.234 19.317 8.591L16.48 15.352H19.387L19.98 13.738H23.518C23.553 13.916 24.161 16 24.161 16H26.851L22.617 8ZM20.73 11.666L21.734 8.917L22.868 11.666H20.73ZM11.037 8L8.683 14.159V14.175H8.647L7.332 8.358C7.214 8.167 6.961 8 6.742 8H2V9.062H5.05L7.962 16H10.822L14.032 8H11.037ZM28.98 12.196L28.125 11.063C27.568 10.518 28.544 10.024 29.569 9.873C30.312 9.761 31.042 10.155 31.042 11.353C31.042 12.593 29.824 12.9 28.98 13.011C28.058 13.125 27.575 13.435 27.575 14.043C27.575 15.39 29.62 15.111 31.758 14.187L32 13.064C30.772 13.447 28.921 13.568 28.536 13.109C27.917 12.35 30.648 12.637 31.642 11.517C32.148 10.957 31.956 9.535 30.551 8.784C29.281 8.106 27.234 8.318 25.86 9.452C25.467 9.845 25.412 10.323 25.412 10.803C25.412 12.5 28.081 12.222 28.98 12.196Z" fill="#fff"/></svg>
              •••• 4242
            </dd>
          </div>
          <div class="sm:col-span-2">
            <dt class="font-medium text-zinc-500 mb-2">Usage this billing cycle</dt>
            <dd>
              <div class="flex items-center justify-between text-xs mb-1.5 font-medium text-zinc-900">
                <span>22,500 comments processed</span>
                <span class="text-zinc-500">50,000 limit</span>
              </div>
              <div class="w-full bg-zinc-100 rounded-full h-3 overflow-hidden border border-zinc-200/50">
                <div class="bg-indigo-500 h-full rounded-full transition-all duration-500 ease-out" style="width: 45%"></div>
              </div>
            </dd>
          </div>
        </dl>
      </div>

      <div class="px-6 py-5 sm:px-8 bg-white relative z-10 flex flex-col sm:flex-row gap-3 items-center justify-between">
        <p class="text-sm text-zinc-500">Payments are processed securely via Stripe.</p>
        <button @click="redirectToStripe" class="w-full sm:w-auto inline-flex justify-center items-center py-2.5 px-5 border border-zinc-200 rounded-xl shadow-sm text-sm font-medium text-zinc-700 bg-white hover:bg-zinc-50 hover:border-zinc-300 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 transition-all duration-200">
          <svg class="h-4 w-4 mr-2 text-zinc-400" fill="currentColor" viewBox="0 0 24 24"><path d="M13.5 22h-3v-6h3v6zM12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z"/></svg>
          Manage Subscription
        </button>
      </div>
    </div>
  </div>
</template>
