<script setup lang="ts">
// Account (/settings/account) — profile, preferences, GDPR export, deletion.
//
// Export polling: 10 s up to 6 stable polls, then 60 s. Deletion is a
// two-step flow inside ConfirmModal (custom footer), guarded by an
// email-match check (case-insensitive, trimmed).

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Download, Mail, ShieldCheck, ShieldAlert } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'
import { useAccountStore } from '../../stores/account'
import { forgotPassword } from '../../services/auth'
import { toast } from '../../lib/toast'
import ConfirmModal from '../../components/ConfirmModal.vue'
import TextInput from '../../components/forms/TextInput.vue'
import PrimaryButton from '../../components/forms/PrimaryButton.vue'

const auth = useAuthStore()
const account = useAccountStore()
const router = useRouter()

// ---- email change --------------------------------------------------------
const newEmail = ref('')
const emailError = ref<string | null>(null)
// RFC-5322 simplified — same shape as backend lower bound.
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const emailFormatError = computed<string | null>(() => {
  if (!newEmail.value) return null
  if (!EMAIL_RE.test(newEmail.value.trim())) return 'That email address looks invalid.'
  return null
})

async function submitEmailChange(): Promise<void> {
  emailError.value = null
  if (!newEmail.value || emailFormatError.value) {
    emailError.value = emailFormatError.value
    return
  }
  const err = await account.updateEmail(newEmail.value.trim())
  if (err) {
    emailError.value = err.message
    return
  }
  newEmail.value = ''
}

// ---- digest opt-in -------------------------------------------------------
async function onDigestToggle(e: Event): Promise<void> {
  const next = (e.target as HTMLInputElement).checked
  await account.setDigestOptIn(next)
}

// ---- password reset shortcut --------------------------------------------
const sendingReset = ref(false)
async function sendPasswordReset(): Promise<void> {
  if (!auth.user?.email) return
  sendingReset.value = true
  try {
    await forgotPassword(auth.user.email)
    toast.success('Password reset link sent. Check your inbox.')
  } catch {
    toast.error('Could not send the reset link. Please try again.')
  } finally {
    sendingReset.value = false
  }
}

// ---- GDPR export polling -------------------------------------------------
let pollTimer: number | null = null
let stablePolls = 0

function clearPoll(): void {
  if (pollTimer !== null) {
    window.clearTimeout(pollTimer)
    pollTimer = null
  }
}

function schedulePoll(): void {
  clearPoll()
  const delay = stablePolls >= 6 ? 60_000 : 10_000
  pollTimer = window.setTimeout(async () => {
    pollTimer = null
    if (!account.exportJob) return
    const prev = account.exportJob.status
    const next = await account.pollExport()
    if (!next) return
    if (next.status === 'completed' || next.status === 'failed') return
    if (next.status === prev) stablePolls++
    schedulePoll()
  }, delay)
}

watch(
  () => account.exportJob,
  (job) => {
    clearPoll()
    stablePolls = 0
    if (!job) return
    if (job.status === 'completed' || job.status === 'failed') return
    schedulePoll()
  },
)

async function onRequestExport(): Promise<void> {
  await account.requestExport()
}

const exportDownloadUrl = computed(() => {
  const j = account.exportJob
  if (!j) return null
  return j.download_url ?? j.export_url ?? null
})

function downloadExport(): void {
  const url = exportDownloadUrl.value
  if (!url) return
  window.location.assign(url)
}

// ---- deletion modal ------------------------------------------------------
const deleteOpen = ref(false)
const deleteStep = ref<1 | 2>(1)
const deleteEmailInput = ref('')
const deleteSubmitting = ref(false)
const deleteError = ref<string | null>(null)

const deleteEmailMatches = computed(() => {
  const target = (auth.user?.email ?? '').trim().toLowerCase()
  return target.length > 0 && deleteEmailInput.value.trim().toLowerCase() === target
})

function openDelete(): void {
  deleteOpen.value = true
  deleteStep.value = 1
  deleteEmailInput.value = ''
  deleteError.value = null
}

function closeDelete(): void {
  deleteOpen.value = false
  deleteStep.value = 1
  deleteEmailInput.value = ''
  deleteError.value = null
}

async function submitDelete(): Promise<void> {
  if (!auth.user?.email || !deleteEmailMatches.value) return
  deleteSubmitting.value = true
  deleteError.value = null
  const res = await account.requestDeletion(auth.user.email)
  deleteSubmitting.value = false
  if (!res.ok) {
    deleteError.value = res.message ?? 'Could not schedule deletion.'
    return
  }
  toast.success('Your account is scheduled for deletion.')
  await auth.logout()
  closeDelete()
  router.push('/login')
}

// ---- mount ---------------------------------------------------------------
onMounted(() => {
  void account.fetchProfile()
})

onBeforeUnmount(() => {
  clearPoll()
})
</script>

<template>
  <div class="max-w-5xl mx-auto py-8 px-4 sm:px-6 lg:px-8 space-y-8">
    <div>
      <h3 class="text-2xl font-semibold leading-7 text-zinc-900 tracking-tight">Account</h3>
      <p class="mt-2 text-sm text-zinc-500">Manage your profile, preferences, and account lifecycle.</p>
    </div>

    <!-- Profile / email -->
    <section class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h4 class="text-base font-semibold text-zinc-900">Profile</h4>
          <p class="mt-1 text-sm text-zinc-500 pr-4">Your sign-in email and verification status.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2 space-y-6">
          <div>
            <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider">Current email</p>
            <div class="mt-1 flex items-center gap-2">
              <Mail class="w-4 h-4 text-zinc-400" />
              <p class="text-sm font-semibold text-zinc-900">{{ auth.user?.email ?? '—' }}</p>
              <span
                v-if="account.profile"
                :class="[
                  'inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset',
                  account.isVerified
                    ? 'bg-emerald-50 text-emerald-700 ring-emerald-600/20'
                    : 'bg-amber-50 text-amber-700 ring-amber-600/20',
                ]"
              >
                <ShieldCheck v-if="account.isVerified" class="w-3 h-3" />
                <ShieldAlert v-else class="w-3 h-3" />
                {{ account.isVerified ? 'Verified' : 'Unverified' }}
              </span>
            </div>
          </div>

          <form class="space-y-3" @submit.prevent="submitEmailChange">
            <TextInput
              v-model="newEmail"
              label="Change email"
              type="email"
              autocomplete="email"
              placeholder="new-address@example.com"
              :error="emailError ?? emailFormatError"
              hint="We'll send a verification link to your new address."
            />
            <div class="flex justify-end">
              <PrimaryButton type="submit" :loading="account.updatingEmail" :disabled="!newEmail || !!emailFormatError">
                Send verification
              </PrimaryButton>
            </div>
          </form>

          <div class="border-t border-zinc-100 pt-4">
            <p class="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">Password</p>
            <button
              type="button"
              :disabled="sendingReset"
              @click="sendPasswordReset"
              class="inline-flex items-center px-3 py-2 rounded-xl text-sm font-medium text-zinc-700 bg-white border border-zinc-200 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 disabled:opacity-50 transition-colors"
            >
              {{ sendingReset ? 'Sending…' : 'Send password reset email' }}
            </button>
          </div>
        </div>
      </div>
    </section>

    <!-- Preferences -->
    <section class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h4 class="text-base font-semibold text-zinc-900">Preferences</h4>
          <p class="mt-1 text-sm text-zinc-500 pr-4">Notification and digest options.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2">
          <label class="flex items-start justify-between gap-4">
            <span class="flex-1">
              <span class="block text-sm font-medium text-zinc-900">Weekly digest email</span>
              <span class="block text-xs text-zinc-500 mt-0.5">
                Get a Monday summary of your top ideas, questions, and actionable insights.
              </span>
              <span
                v-if="!account.supportsDigestOptIn"
                class="inline-block mt-1 text-[10px] font-semibold uppercase tracking-wider text-zinc-500 bg-zinc-100 px-1.5 py-0.5 rounded-md"
              >
                Coming soon
              </span>
            </span>
            <input
              type="checkbox"
              :disabled="!account.supportsDigestOptIn"
              :checked="!!account.profile?.weekly_digest_opt_in"
              @change="onDigestToggle"
              class="h-5 w-5 rounded border-zinc-300 text-zinc-900 focus:ring-zinc-900 disabled:opacity-40 cursor-pointer"
              aria-label="Weekly digest opt-in"
            />
          </label>
        </div>
      </div>
    </section>

    <!-- GDPR data export -->
    <section class="bg-white shadow-sm ring-1 ring-zinc-950/5 rounded-2xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h4 class="text-base font-semibold text-zinc-900">Your data</h4>
          <p class="mt-1 text-sm text-zinc-500 pr-4">Download a portable copy of everything we hold for you.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2 space-y-3">
          <div v-if="!account.exportJob">
            <button
              type="button"
              :disabled="account.exportLoading"
              @click="onRequestExport"
              class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium text-zinc-700 bg-white border border-zinc-200 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900 disabled:opacity-50"
            >
              <Download class="w-4 h-4" />
              {{ account.exportLoading ? 'Requesting…' : 'Export my data (GDPR)' }}
            </button>
            <p class="text-xs text-zinc-500 mt-2">
              We'll prepare a .zip archive. Larger accounts may take a few minutes.
            </p>
          </div>

          <div v-else class="rounded-xl ring-1 ring-zinc-200 p-4 bg-zinc-50/40">
            <p class="text-sm font-medium text-zinc-900">
              Status:
              <span class="capitalize">{{ account.exportJob.status }}</span>
            </p>
            <p v-if="account.exportJob.error" class="text-xs text-rose-600 mt-1">
              {{ account.exportJob.error }}
            </p>

            <div v-if="account.exportJob.status === 'completed' && exportDownloadUrl" class="mt-3">
              <button
                type="button"
                @click="downloadExport"
                class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900"
              >
                <Download class="w-4 h-4" /> Download (.zip)
              </button>
              <p v-if="account.exportJob.expires_at" class="text-[11px] text-zinc-500 mt-1.5">
                Link expires {{ account.exportJob.expires_at }}
              </p>
            </div>

            <div v-else-if="account.exportJob.status === 'failed'" class="mt-3 flex items-center gap-2">
              <button
                type="button"
                @click="onRequestExport"
                class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium text-zinc-700 bg-white border border-zinc-200 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900"
              >
                Retry
              </button>
              <button
                type="button"
                @click="account.clearExport()"
                class="text-xs text-zinc-500 hover:text-zinc-700"
              >
                Dismiss
              </button>
            </div>

            <div v-else class="mt-3 flex items-center gap-2 text-xs text-zinc-500">
              <span class="inline-block h-2 w-2 rounded-full bg-amber-500 animate-pulse"></span>
              We'll keep checking…
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Danger zone -->
    <section class="bg-rose-50/50 ring-1 ring-rose-950/10 rounded-2xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h4 class="text-base font-semibold text-rose-700">Danger zone</h4>
          <p class="mt-1 text-sm text-rose-600/80 pr-4">Irreversible account actions.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2">
          <p class="text-sm text-rose-700/80 mb-4">
            Deleting your account schedules permanent removal of your channels, saved comments, and personal data.
          </p>
          <button
            type="button"
            @click="openDelete"
            class="inline-flex items-center px-4 py-2.5 rounded-xl text-sm font-medium text-rose-600 bg-white shadow-sm ring-1 ring-rose-200 hover:bg-rose-50 hover:ring-rose-300 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-rose-500 transition-colors"
          >
            Delete account
          </button>
        </div>
      </div>
    </section>

    <!-- Delete modal — two-step, custom footer -->
    <ConfirmModal
      v-model:open="deleteOpen"
      :title="deleteStep === 1 ? 'Delete your account?' : 'Confirm account deletion'"
      :destructive="true"
      :custom-footer="true"
      @cancel="closeDelete"
    >
      <template v-if="deleteStep === 1">
        <p class="text-sm text-zinc-700 leading-relaxed">
          This will permanently remove:
        </p>
        <ul class="mt-2 text-sm text-zinc-600 list-disc pl-5 space-y-1">
          <li>All your tracked channels and their settings</li>
          <li>Your saved comments and starred items</li>
          <li>Your weekly digest history</li>
          <li>Your account profile and login</li>
        </ul>
        <p class="mt-3 text-sm text-zinc-500">
          Comments and analytics for channels shared with other users are kept on their accounts.
        </p>
      </template>
      <template v-else>
        <p class="text-sm text-zinc-700">
          Type your email
          <strong class="text-zinc-900 font-semibold">{{ auth.user?.email }}</strong>
          to confirm.
        </p>
        <input
          v-model="deleteEmailInput"
          type="email"
          autocomplete="off"
          class="mt-3 appearance-none block w-full px-4 py-3 border border-zinc-200 rounded-xl shadow-sm text-sm focus:outline-none focus:ring-2 focus:ring-rose-500 focus:border-rose-500"
          placeholder="Enter your email"
        />
        <p v-if="deleteError" class="mt-2 text-xs text-rose-600 font-medium">{{ deleteError }}</p>
      </template>

      <template #footer>
        <div class="w-full flex flex-col sm:flex-row-reverse gap-3">
          <button
            v-if="deleteStep === 1"
            type="button"
            @click="deleteStep = 2"
            class="w-full sm:w-auto inline-flex justify-center items-center py-2.5 px-5 rounded-xl text-sm font-medium text-white bg-rose-600 hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-rose-500 transition-colors"
          >
            I understand, continue
          </button>
          <button
            v-else
            type="button"
            :disabled="!deleteEmailMatches || deleteSubmitting"
            @click="submitDelete"
            class="w-full sm:w-auto inline-flex justify-center items-center py-2.5 px-5 rounded-xl text-sm font-medium text-white bg-rose-600 hover:bg-rose-700 disabled:opacity-50 disabled:cursor-not-allowed focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-rose-500 transition-colors"
          >
            {{ deleteSubmitting ? 'Deleting…' : 'Permanently delete my account' }}
          </button>
          <button
            type="button"
            data-modal-cancel="true"
            @click="closeDelete"
            class="w-full sm:w-auto inline-flex justify-center items-center py-2.5 px-5 rounded-xl text-sm font-medium text-zinc-700 bg-white border border-zinc-200 hover:bg-zinc-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-zinc-900"
          >
            Cancel
          </button>
        </div>
      </template>
    </ConfirmModal>
  </div>
</template>
