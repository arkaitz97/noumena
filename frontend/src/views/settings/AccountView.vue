<script setup lang="ts">
import { ref } from 'vue'

const email = ref('alex@example.com')
const name = ref('Alex Creator')
const deleteModalOpen = ref(false)
const deleteStep = ref(1)
const confirmEmail = ref('')

const confirmDelete = () => {
  if (confirmEmail.value === email.value) {
    // Delete account logic
    alert('Account deletion requested.')
    deleteModalOpen.value = false
  }
}
</script>

<template>
  <div class="space-y-10">
    <div class="px-4 sm:px-0">
      <h3 class="text-2xl font-semibold leading-7 text-zinc-900 tracking-tight">Account Settings</h3>
      <p class="mt-2 text-sm text-zinc-500">Manage your profile details and account status.</p>
    </div>

    <!-- Profile Form -->
    <div class="bg-white shadow-sm ring-1 ring-zinc-950/5 sm:rounded-2xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h3 class="text-base font-medium leading-6 text-zinc-900">Profile</h3>
          <p class="mt-1 text-sm text-zinc-500 pr-4">Update your basic profile details to control how you appear on Noumena.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2">
          <form @submit.prevent>
            <div class="grid grid-cols-1 gap-6">
              <div>
                <label for="name" class="block text-sm font-medium text-zinc-700 mb-1.5">Display Name</label>
                <input type="text" name="name" id="name" v-model="name" class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm text-sm focus:outline-none focus:ring-2 focus:ring-zinc-900 focus:border-zinc-900 transition-colors bg-zinc-50/50 focus:bg-white text-zinc-900" />
              </div>
              <div>
                <label for="email" class="block text-sm font-medium text-zinc-700 mb-1.5">Email Address</label>
                <input type="email" name="email" id="email" disabled v-model="email" class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm text-sm bg-zinc-100 text-zinc-500 cursor-not-allowed" />
                <p class="mt-2 text-xs text-zinc-500">Email address cannot be changed. Please contact support if you need to transfer your account.</p>
              </div>
            </div>
            <div class="mt-8 flex justify-end">
              <button type="submit" class="inline-flex justify-center items-center py-2.5 px-5 border border-transparent rounded-xl shadow-sm text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 transition-all duration-200">
                Save changes
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>

    <!-- Danger Zone -->
    <div class="bg-rose-50/50 shadow-sm ring-1 ring-rose-950/10 sm:rounded-2xl p-6 sm:p-8 mt-10">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h3 class="text-base font-medium leading-6 text-rose-700">Danger Zone</h3>
          <p class="mt-1 text-sm text-rose-600/80 pr-4">Irreversible account actions.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2">
          <div class="text-sm text-rose-700/80 mb-5">
            <p>Once you delete your account, there is no going back. All tracked channels and insights will be permanently wiped.</p>
          </div>
          <div>
            <button @click="deleteModalOpen = true; deleteStep = 1; confirmEmail = ''" type="button" class="inline-flex items-center justify-center px-4 py-2.5 border border-transparent font-medium rounded-xl text-rose-600 bg-white shadow-sm ring-1 ring-rose-200 hover:bg-rose-50 hover:ring-rose-300 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-rose-500 focus:ring-offset-rose-50 sm:text-sm transition-all duration-200">
              Delete account
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Delete Account Modal -->
    <div v-if="deleteModalOpen" class="fixed z-50 inset-0 overflow-y-auto" aria-labelledby="modal-title" role="dialog" aria-modal="true">
      <div class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
        <div class="fixed inset-0 bg-zinc-900/60 backdrop-blur-sm transition-opacity" @click="deleteModalOpen = false" aria-hidden="true"></div>
        <span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>
        <div class="inline-block align-bottom bg-white rounded-2xl px-4 pt-5 pb-6 text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full sm:p-8">
          <div class="sm:flex sm:items-start">
            <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-rose-100 sm:mx-0 sm:h-10 sm:w-10">
              <svg class="h-6 w-6 text-rose-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" /></svg>
            </div>
            <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left w-full">
              <h3 class="text-xl leading-6 font-semibold text-zinc-900 mb-2" id="modal-title">Delete Account</h3>
              <div class="mt-2">
                <template v-if="deleteStep === 1">
                  <p class="text-sm text-zinc-500 leading-relaxed">
                    This will permanently remove all your tracked channels, personal insights, and account data. Are you absolutely sure?
                  </p>
                </template>
                <template v-else-if="deleteStep === 2">
                  <p class="text-sm text-zinc-500 mb-3">
                    Please type <strong class="text-zinc-900 font-semibold">{{ email }}</strong> to confirm.
                  </p>
                  <input type="email" v-model="confirmEmail" class="appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm text-sm focus:outline-none focus:ring-2 focus:ring-rose-500 focus:border-rose-500" placeholder="Enter your email" />
                </template>
              </div>
            </div>
          </div>
          <div class="mt-8 sm:flex sm:flex-row-reverse gap-3">
            <template v-if="deleteStep === 1">
              <button @click="deleteStep = 2" type="button" class="w-full inline-flex justify-center rounded-xl border border-transparent shadow-sm px-5 py-2.5 bg-rose-600 text-sm font-medium text-white hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-rose-500 sm:w-auto transition-colors">
                I understand, proceed
              </button>
            </template>
            <template v-else-if="deleteStep === 2">
              <button @click="confirmDelete" :disabled="confirmEmail !== email" type="button" class="w-full inline-flex justify-center rounded-xl border border-transparent shadow-sm px-5 py-2.5 bg-rose-600 text-sm font-medium text-white hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-rose-500 sm:w-auto disabled:opacity-50 disabled:cursor-not-allowed transition-colors">
                Confirm Deletion
              </button>
            </template>
            <button @click="deleteModalOpen = false" type="button" class="mt-3 sm:mt-0 w-full inline-flex justify-center rounded-xl border border-zinc-200 shadow-sm px-5 py-2.5 bg-white text-sm font-medium text-zinc-700 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 sm:w-auto transition-colors">
              Cancel
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
