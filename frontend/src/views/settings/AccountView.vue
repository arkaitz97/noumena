<script setup lang="ts">
// Account (/settings/account) — profile, preferences, GDPR export, deletion.
//
// Export polling: 10 s up to 6 stable polls, then 60 s. Deletion is a
// two-step flow inside ConfirmModal (custom footer), guarded by an
// email-match check (case-insensitive, trimmed).

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { AxiosError } from 'axios'
import { Download, Mail, ShieldCheck, ShieldAlert } from 'lucide-vue-next'
import { useAuthStore } from '../../stores/auth'
import { useAccountStore } from '../../stores/account'
import { forgotPassword } from '../../services/auth'
import { recentExports } from '../../services/comments'
import { validatePassword } from '../../lib/validation'
import type { ApiErrorBody, ExportJob } from '../../services/types'
import { toast } from '../../lib/toast'
import { shortDate } from '../../lib/relativeTime'
import ConfirmModal from '../../components/ConfirmModal.vue'
import TextInput from '../../components/forms/TextInput.vue'
import PasswordInput from '../../components/forms/PasswordInput.vue'
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

const pendingEmail = computed(() => account.profile?.pending_email ?? null)

const verificationBadgeClass = computed(() =>
  account.isVerified
    ? 'bg-emerald-50 text-emerald-700 ring-emerald-600/20 dark:bg-emerald-400/10 dark:text-emerald-300 dark:ring-emerald-400/20'
    : 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-400/10 dark:text-amber-300 dark:ring-amber-400/20', // unslop-ignore: semantic pending state
)

const resendingChange = ref(false)

async function resendEmailChange(): Promise<void> {
  resendingChange.value = true
  try {
    await auth.resendVerification()
    toast.success('Verification email sent to your new address.')
  } catch {
    toast.error('Could not resend. Please try again later.')
  } finally {
    resendingChange.value = false
  }
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

// ---- password change ------------------------------------------------------
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const changingPassword = ref(false)
const passwordError = ref<string | null>(null)

// Same policy + rule labels as the reset form (lib/validation) — shown
// inline via the strength meter before any submit.
const newPasswordValidation = computed(() => validatePassword(newPassword.value))
const passwordsMatch = computed(
  () => confirmPassword.value.length > 0 && confirmPassword.value === newPassword.value,
)
const canSubmitPassword = computed(
  () =>
    currentPassword.value.length > 0 &&
    newPasswordValidation.value.valid &&
    passwordsMatch.value,
)

async function submitPasswordChange(): Promise<void> {
  passwordError.value = null
  if (!currentPassword.value) {
    passwordError.value = 'Enter your current password.'
    return
  }
  if (!newPasswordValidation.value.valid) {
    passwordError.value =
      newPasswordValidation.value.firstError || 'Password does not meet requirements.'
    return
  }
  if (!passwordsMatch.value) {
    passwordError.value = 'Passwords do not match.'
    return
  }
  changingPassword.value = true
  try {
    await auth.changePassword(currentPassword.value, newPassword.value)
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    toast.success('Password updated.')
  } catch (err) {
    if (err instanceof AxiosError && err.response) {
      const status = err.response.status
      const body = err.response.data as ApiErrorBody | undefined
      if (status === 401) {
        passwordError.value = 'Current password is incorrect.'
      } else if (status === 400) {
        passwordError.value =
          body?.error ?? body?.message ?? 'New password does not meet the requirements.'
      } else {
        passwordError.value = 'Could not update your password. Please try again.'
      }
    } else {
      passwordError.value = 'Could not update your password. Please try again.'
    }
  } finally {
    changingPassword.value = false
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
  void loadRecentExports()
})

onBeforeUnmount(() => {
  clearPoll()
})

// ---- recent comment exports (fix-frontend-plumbing) ----------------------
const recent = ref<ExportJob[]>([])
const recentLoading = ref(false)

async function loadRecentExports(): Promise<void> {
  recentLoading.value = true
  try {
    recent.value = await recentExports()
  } catch {
    recent.value = []
  } finally {
    recentLoading.value = false
  }
}

function recentDownloadUrl(j: ExportJob): string | null {
  return j.download_url ?? j.export_url ?? null
}

function downloadRecent(j: ExportJob): void {
  const url = recentDownloadUrl(j)
  if (url) window.location.assign(url)
}
</script>

<template>
  <div class="max-w-5xl mx-auto py-8 px-4 sm:px-6 lg:px-8 space-y-8">
    <div>
      <h1 class="text-2xl font-semibold leading-7 text-zinc-900 dark:text-white tracking-tight">Account</h1>
      <p class="mt-2 text-sm text-zinc-500 dark:text-zinc-400 dark:text-zinc-400">Manage your profile, preferences, and account lifecycle.</p>
    </div>

    <!-- Profile / email -->
    <section class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h2 class="text-base font-semibold text-zinc-900 dark:text-white">Profile</h2>
          <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400 pr-4">Your sign-in email and verification status.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2 space-y-6">
          <div>
            <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider">Current email</p>
            <div class="mt-1 flex items-center gap-2">
              <Mail class="w-4 h-4 text-zinc-400 dark:text-zinc-500" />
              <p class="text-sm font-semibold text-zinc-900 dark:text-white">{{ auth.user?.email ?? '—' }}</p>
              <span
                v-if="account.profile"
                :class="[
                  'inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset',
                  verificationBadgeClass,
                ]"
              >
                <ShieldCheck v-if="account.isVerified" class="w-3 h-3" />
                <ShieldAlert v-else class="w-3 h-3" />
                {{ account.isVerified ? 'Verified' : 'Unverified' }}
              </span>
            </div>
          </div>

          <div v-if="pendingEmail" class="rounded-xl bg-amber-50 dark:bg-amber-400/10 ring-1 ring-amber-600/20 dark:ring-amber-400/20 p-4"> <!-- unslop-ignore -->
            <p class="text-sm font-medium text-amber-800 dark:text-amber-200">
              Verification pending for
              <span class="font-semibold">{{ pendingEmail }}</span>
            </p>
            <p class="text-xs text-amber-700 dark:text-amber-300 mt-1">
              Check your inbox (and spam) for the verification link. Your current email stays active until the new one is confirmed.
            </p>
            <button
              type="button"
              :disabled="resendingChange"
              @click="resendEmailChange"
              class="mt-2 inline-flex items-center px-3 py-1.5 rounded-lg text-xs font-semibold text-amber-800 dark:text-amber-200 bg-amber-100 dark:bg-amber-400/15 hover:bg-amber-200 dark:hover:bg-amber-400/25 disabled:opacity-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-500" data-unslop-ignore
            >
              {{ resendingChange ? 'Sending…' : 'Resend verification' }}
            </button>
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

          <div class="border-t border-zinc-200/70 dark:border-white/5 pt-4">
            <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wider mb-3">Password</p>
            <form class="space-y-3" @submit.prevent="submitPasswordChange">
              <div v-if="passwordError" role="alert" class="bg-rose-50 dark:bg-rose-500/10 p-4 rounded-xl text-rose-700 dark:text-rose-300 text-sm border border-rose-100 dark:border-rose-500/20">
                {{ passwordError }}
              </div>

              <PasswordInput
                v-model="currentPassword"
                label="Current password"
                placeholder="••••••••"
                autocomplete="current-password"
                required
              />

              <PasswordInput
                v-model="newPassword"
                label="New password"
                placeholder="••••••••"
                autocomplete="new-password"
                show-strength
                required
              />

              <PasswordInput
                v-model="confirmPassword"
                label="Confirm new password"
                placeholder="••••••••"
                autocomplete="new-password"
                :error="confirmPassword.length > 0 && !passwordsMatch ? 'Passwords do not match' : null"
                required
              />

              <div class="flex flex-wrap items-center justify-between gap-3 pt-1">
                <button
                  type="button"
                  :disabled="sendingReset"
                  @click="sendPasswordReset"
                  class="inline-flex items-center px-3 py-2 rounded-xl text-sm font-medium text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {{ sendingReset ? 'Sending…' : 'Send password reset email' }}
                </button>
                <PrimaryButton type="submit" :loading="changingPassword" :disabled="!canSubmitPassword">
                  Update password
                </PrimaryButton>
              </div>
            </form>
          </div>
        </div>
      </div>
    </section>

    <!-- Preferences -->
    <section class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h2 class="text-base font-semibold text-zinc-900 dark:text-white">Preferences</h2>
          <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400 pr-4">Notification and digest options.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2">
          <label class="flex items-start justify-between gap-4">
            <span class="flex-1">
              <span class="block text-sm font-medium text-zinc-900 dark:text-white">Weekly digest email</span>
              <span class="block text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">
                Get a Monday summary of your top ideas, questions, and actionable insights.
              </span>
              <span
                v-if="!account.supportsDigestOptIn"
                class="inline-block mt-1 text-[10px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400 bg-zinc-100 dark:bg-zinc-800 px-1.5 py-0.5 rounded-md"
              >
                Coming soon
              </span>
            </span>
            <input
              type="checkbox"
              :disabled="!account.supportsDigestOptIn"
              :checked="!!account.profile?.weekly_digest_opt_in"
              @change="onDigestToggle"
              class="h-5 w-5 rounded border-zinc-300 dark:border-zinc-600 text-zinc-900 dark:text-zinc-100 focus-visible:ring-2 focus-visible:ring-brand-500/60 focus-visible:border-brand-500/70 disabled:opacity-40 cursor-pointer"
              aria-label="Weekly digest opt-in"
            />
          </label>
        </div>
      </div>
    </section>

    <!-- GDPR data export -->
    <section class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h2 class="text-base font-semibold text-zinc-900 dark:text-white">Your data</h2>
          <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400 pr-4">Download a portable copy of everything we hold for you.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2 space-y-3">
          <div v-if="!account.exportJob">
            <button
              type="button"
              :disabled="account.exportLoading"
              @click="onRequestExport"
              class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Download class="w-4 h-4" />
              {{ account.exportLoading ? 'Requesting…' : 'Export my data (GDPR)' }}
            </button>
            <p class="text-xs text-zinc-500 dark:text-zinc-400 mt-2">
              We'll prepare a .zip archive. Larger accounts may take a few minutes.
            </p>
          </div>

          <div v-else class="rounded-xl ring-1 ring-zinc-200/70 dark:ring-white/5 p-4 bg-zinc-50/40 dark:bg-zinc-800/50">
            <p class="text-sm font-medium text-zinc-900 dark:text-white">
              Status:
              <span class="capitalize">{{ account.exportJob.status }}</span>
            </p>
            <p v-if="account.exportJob.error" class="text-xs text-rose-600 dark:text-rose-400 mt-1">
              {{ account.exportJob.error }}
            </p>

            <div v-if="account.exportJob.status === 'completed' && exportDownloadUrl" class="mt-3">
              <button
                type="button"
                @click="downloadExport"
                class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium text-white bg-zinc-900 hover:bg-zinc-700 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200 shadow-card dark:shadow-none transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
              >
                <Download class="w-4 h-4" /> Download (.zip)
              </button>
              <p v-if="account.exportJob.expires_at" class="text-[11px] text-zinc-500 dark:text-zinc-400 mt-1.5">
                Link expires {{ shortDate(account.exportJob.expires_at) }}
              </p>
            </div>

            <div v-else-if="account.exportJob.status === 'failed'" class="mt-3 flex items-center gap-2">
              <button
                type="button"
                @click="onRequestExport"
                class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
              >
                Retry
              </button>
              <button
                type="button"
                @click="account.clearExport()"
                class="text-xs text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:text-zinc-300"
              >
                Dismiss
              </button>
            </div>

            <div v-else class="mt-3 flex items-center gap-2 text-xs text-zinc-500 dark:text-zinc-400">
              <span class="inline-block h-2 w-2 rounded-full bg-amber-500 animate-pulse"></span>
              We'll keep checking…
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Recent comment exports -->
    <section class="bg-white dark:bg-zinc-900 shadow-card ring-1 ring-zinc-950/5 dark:ring-white/10 rounded-xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h2 class="text-base font-semibold text-zinc-900 dark:text-white">Recent exports</h2>
          <p class="mt-1 text-sm text-zinc-500 dark:text-zinc-400 pr-4">Comment exports you've requested from the feed.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2">
          <p v-if="recentLoading" class="text-sm text-zinc-500 dark:text-zinc-400">Loading…</p>
          <ul v-else-if="recent.length > 0" class="divide-y divide-zinc-200/70 dark:divide-white/5">
            <li
              v-for="j in recent"
              :key="j.job_id"
              class="py-2.5 flex items-center gap-3"
            >
              <div class="flex-1 min-w-0">
                <p class="text-sm text-zinc-900 dark:text-white">
                  {{ (j.format ?? 'comments').toUpperCase() }} export
                  <span class="capitalize text-xs text-zinc-500 dark:text-zinc-400">· {{ j.status }}</span>
                </p>
                <p v-if="j.created_at" class="text-[11px] text-zinc-400 dark:text-zinc-500">{{ j.created_at }}</p>
                <p v-if="j.status === 'failed' && j.error" class="text-[11px] text-rose-600 dark:text-rose-400">{{ j.error }}</p>
              </div>
              <button
                v-if="j.status === 'completed' && recentDownloadUrl(j)"
                type="button"
                @click="downloadRecent(j)"
                class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
              >
                <Download class="w-3.5 h-3.5" /> Download
              </button>
            </li>
          </ul>
          <p v-else class="text-sm text-zinc-500 dark:text-zinc-400">No comment exports yet.</p>
        </div>
      </div>
    </section>

    <!-- Danger zone -->
    <section class="bg-rose-50/50 dark:bg-rose-500/[0.06] ring-1 ring-rose-200 dark:ring-rose-500/30 rounded-xl p-6 sm:p-8">
      <div class="md:grid md:grid-cols-3 md:gap-8">
        <div class="md:col-span-1">
          <h2 class="text-base font-semibold text-rose-700 dark:text-rose-300">Danger zone</h2>
          <p class="mt-1 text-sm text-rose-600/80 dark:text-rose-400 pr-4">Irreversible account actions.</p>
        </div>
        <div class="mt-5 md:mt-0 md:col-span-2">
          <p class="text-sm text-rose-700/80 dark:text-rose-300 mb-4">
            Deleting your account schedules permanent removal of your channels, saved comments, and personal data.
          </p>
          <button
            type="button"
            @click="openDelete"
            class="inline-flex items-center px-4 py-2.5 rounded-xl text-sm font-medium text-rose-600 dark:text-rose-400 bg-white dark:bg-zinc-900 shadow-card ring-1 ring-rose-200 dark:ring-rose-500/30 hover:bg-rose-50 dark:hover:bg-rose-500/10 hover:ring-rose-300 dark:hover:ring-rose-400/40 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-rose-500 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
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
        <p class="text-sm text-zinc-700 dark:text-zinc-300 leading-relaxed">
          This will permanently remove:
        </p>
        <ul class="mt-2 text-sm text-zinc-600 dark:text-zinc-400 list-disc pl-5 space-y-1">
          <li>All your tracked channels and their settings</li>
          <li>Your saved comments and starred items</li>
          <li>Your weekly digest history</li>
          <li>Your account profile and login</li>
        </ul>
        <p class="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
          Comments and analytics for channels shared with other users are kept on their accounts.
        </p>
      </template>
      <template v-else>
        <p class="text-sm text-zinc-700 dark:text-zinc-300">
          Type your email
          <strong class="text-zinc-900 dark:text-white font-semibold">{{ auth.user?.email }}</strong>
          to confirm.
        </p>
        <input
          v-model="deleteEmailInput"
          type="email"
          autocomplete="off"
          class="mt-3 appearance-none block w-full px-4 py-3 border border-zinc-200/70 dark:border-white/5 rounded-xl shadow-sm text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-rose-500 focus-visible:border-rose-500 bg-zinc-50/50 focus:bg-white dark:bg-zinc-800 dark:focus:bg-zinc-800 dark:text-white"
          placeholder="Enter your email"
        />
        <p v-if="deleteError" role="alert" class="mt-2 text-xs text-rose-600 dark:text-rose-400 font-medium">{{ deleteError }}</p>
      </template>

      <template #footer>
        <div class="w-full flex flex-col sm:flex-row-reverse gap-3">
          <button
            v-if="deleteStep === 1"
            type="button"
            @click="deleteStep = 2"
            class="w-full sm:w-auto inline-flex justify-center items-center py-2.5 px-5 rounded-xl text-sm font-medium text-white bg-rose-600 hover:bg-rose-700 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-rose-500"
          >
            I understand, continue
          </button>
          <button
            v-else
            type="button"
            :disabled="!deleteEmailMatches || deleteSubmitting"
            @click="submitDelete"
            class="w-full sm:w-auto inline-flex justify-center items-center py-2.5 px-5 rounded-xl text-sm font-medium text-white bg-rose-600 hover:bg-rose-700 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-rose-500"
          >
            {{ deleteSubmitting ? 'Deleting…' : 'Permanently delete my account' }}
          </button>
          <button
            type="button"
            data-modal-cancel="true"
            @click="closeDelete"
            class="w-full sm:w-auto inline-flex justify-center items-center py-2.5 px-5 rounded-xl text-sm font-medium text-zinc-700 dark:text-zinc-200 bg-white dark:bg-zinc-800/60 border border-zinc-200 dark:border-white/10 hover:border-zinc-300 hover:bg-zinc-50 dark:hover:border-white/20 dark:hover:bg-zinc-800 transition-all duration-200 ease-snappy active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/80 focus-visible:ring-offset-2 focus-visible:ring-offset-white dark:focus-visible:ring-offset-zinc-900"
          >
            Cancel
          </button>
        </div>
      </template>
    </ConfirmModal>
  </div>
</template>
