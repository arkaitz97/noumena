// useAccountStore — profile read + email update + GDPR export polling +
// account deletion. Polling lifecycle (start/stop) is owned by the view
// since it must tear down on navigation.

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { AxiosError } from 'axios'
import * as accountApi from '../services/account'
import { toast } from '../lib/toast'
import type { ApiErrorBody, ExportJob, UserProfile } from '../services/types'

export interface EmailUpdateError {
  status: number
  code?: string
  message: string
}

export const useAccountStore = defineStore('account', () => {
  const profile = ref<UserProfile | null>(null)
  const loading = ref(false)
  const exportJob = ref<ExportJob | null>(null)
  const exportLoading = ref(false)
  const updatingEmail = ref(false)

  async function fetchProfile(): Promise<void> {
    loading.value = true
    try {
      profile.value = await accountApi.me()
    } catch {
      toast.error('Could not load your account profile.')
    } finally {
      loading.value = false
    }
  }

  async function updateEmail(newEmail: string): Promise<EmailUpdateError | null> {
    updatingEmail.value = true
    try {
      const updated = await accountApi.updateEmail(newEmail)
      profile.value = updated
      toast.success('Verification email sent to your new address.')
      return null
    } catch (err) {
      if (err instanceof AxiosError && err.response) {
        const status = err.response.status
        const body = err.response.data as ApiErrorBody | undefined
        if (status === 409) {
          return { status, code: body?.code, message: 'That email is already in use.' }
        }
        if (status === 429) {
          return { status, code: body?.code, message: 'You can change your email once per day.' }
        }
        if (status === 400) {
          return { status, code: body?.code, message: body?.message ?? 'That email looks invalid.' }
        }
      }
      return { status: 0, message: 'Could not update your email. Please try again.' }
    } finally {
      updatingEmail.value = false
    }
  }

  async function setDigestOptIn(value: boolean): Promise<void> {
    try {
      const updated = await accountApi.patchProfile({ weekly_digest_opt_in: value })
      profile.value = updated
    } catch {
      toast.error('Could not update your preference.')
    }
  }

  async function requestExport(): Promise<void> {
    exportLoading.value = true
    try {
      exportJob.value = await accountApi.requestExport()
    } catch {
      toast.error('Could not start your data export.')
    } finally {
      exportLoading.value = false
    }
  }

  async function pollExport(): Promise<ExportJob | null> {
    if (!exportJob.value) return null
    try {
      const next = await accountApi.exportStatus(exportJob.value.job_id)
      exportJob.value = next
      return next
    } catch {
      return exportJob.value
    }
  }

  function clearExport(): void {
    exportJob.value = null
  }

  async function requestDeletion(emailConfirmation: string): Promise<{
    ok: boolean
    status?: number
    message?: string
  }> {
    try {
      await accountApi.requestDeletion(emailConfirmation)
      return { ok: true }
    } catch (err) {
      if (err instanceof AxiosError && err.response) {
        const status = err.response.status
        if (status === 400) return { ok: false, status, message: 'That email does not match your account.' }
        if (status === 423)
          return { ok: false, status, message: 'Your account is already scheduled for deletion.' }
      }
      return { ok: false, status: 0, message: 'Could not schedule deletion. Please try again.' }
    }
  }

  const isVerified = computed(() => profile.value?.email_verified_at !== null && profile.value?.email_verified_at !== undefined)
  const supportsDigestOptIn = computed(
    () => profile.value !== null && profile.value.weekly_digest_opt_in !== null && profile.value.weekly_digest_opt_in !== undefined,
  )

  return {
    profile,
    loading,
    updatingEmail,
    exportJob,
    exportLoading,
    isVerified,
    supportsDigestOptIn,
    fetchProfile,
    updateEmail,
    setDigestOptIn,
    requestExport,
    pollExport,
    clearExport,
    requestDeletion,
  }
})
