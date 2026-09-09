// /api/user/* surface — profile read, email change, GDPR export, account
// deletion. The patchable surface is currently `email` and (when backend
// support lands) `weekly_digest_opt_in`.

import apiClient from './api'
import type { ExportJob, UserProfile } from './types'

export async function me(): Promise<UserProfile> {
  const { data } = await apiClient.get<UserProfile & { email_verified?: boolean }>('/user/me')
  // Normalise email_verified → email_verified_at to match the auth store
  // contract.
  const verifiedAt =
    data.email_verified_at ?? (data.email_verified ? new Date().toISOString() : null)
  return {
    id: data.id,
    email: data.email,
    email_verified_at: verifiedAt,
    subscription_plan: data.subscription_plan,
    subscription_status: data.subscription_status,
    created_at: data.created_at,
    weekly_digest_opt_in: data.weekly_digest_opt_in ?? null,
    deletion_requested_at: data.deletion_requested_at ?? null,
    pending_email: data.pending_email ?? null,
  }
}

export async function patchProfile(
  patch: { email?: string; weekly_digest_opt_in?: boolean },
): Promise<UserProfile> {
  const { data } = await apiClient.patch<UserProfile & { email_verified?: boolean }>('/user/me', patch)
  const verifiedAt =
    data.email_verified_at ?? (data.email_verified ? new Date().toISOString() : null)
  return {
    id: data.id,
    email: data.email,
    email_verified_at: verifiedAt,
    subscription_plan: data.subscription_plan,
    subscription_status: data.subscription_status,
    created_at: data.created_at,
    weekly_digest_opt_in: data.weekly_digest_opt_in ?? null,
    deletion_requested_at: data.deletion_requested_at ?? null,
    pending_email: data.pending_email ?? null,
  }
}

export async function updateEmail(email: string): Promise<UserProfile> {
  return patchProfile({ email })
}

export async function requestExport(): Promise<ExportJob> {
  const { data } = await apiClient.post<ExportJob>('/user/data-export')
  return data
}

export async function exportStatus(jobId: string): Promise<ExportJob> {
  const { data } = await apiClient.get<ExportJob>(
    `/user/data-export/${encodeURIComponent(jobId)}/status`,
  )
  return data
}

export async function requestDeletion(emailConfirmation: string): Promise<void> {
  await apiClient.delete('/user/account', {
    data: { email_confirmation: emailConfirmation },
  })
}
