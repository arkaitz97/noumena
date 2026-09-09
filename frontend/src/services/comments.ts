// /api/comments/* surface — feed, saved, star/unstar, export request +
// status. The export download is a signed URL — the caller navigates to it
// directly (no XHR), so we expose a helper to produce the absolute URL.

import apiClient, { apiBaseURL } from './api'
import type {
  Comment,
  ExportFormat,
  ExportJob,
  FeedFilters,
  FeedResponse,
} from './types'

const PAGE_SIZE = 20

export interface FeedPageOpts {
  offset: number
  limit?: number
}

function filtersToParams(filters: Partial<FeedFilters>): Record<string, string> {
  const out: Record<string, string> = {}
  if (filters.channels && filters.channels.length > 0) out.channels = filters.channels.join(',')
  if (filters.categories && filters.categories.length > 0) out.categories = filters.categories.join(',')
  if (filters.insight_type) out.insight_type = filters.insight_type
  if (filters.date_from) out.date_from = filters.date_from
  if (filters.date_to) out.date_to = filters.date_to
  if (filters.search && filters.search.trim().length > 0) out.search = filters.search.trim()
  if (filters.include_toxic === true) out.include_toxic = 'true'
  if (filters.sort && filters.sort !== 'newest') out.sort = filters.sort
  return out
}

interface RawFeedResponse {
  comments?: Comment[] | null
  total?: number
  has_more?: boolean
}

function normaliseFeedResponse(raw: RawFeedResponse | Comment[] | null): FeedResponse {
  if (Array.isArray(raw)) {
    return { comments: raw, total: raw.length, has_more: raw.length === PAGE_SIZE }
  }
  return {
    comments: raw?.comments ?? [],
    total: raw?.total ?? 0,
    has_more: Boolean(raw?.has_more),
  }
}

export async function feed(
  filters: Partial<FeedFilters>,
  page: FeedPageOpts,
): Promise<FeedResponse> {
  const params = {
    ...filtersToParams(filters),
    offset: String(page.offset),
    limit: String(page.limit ?? PAGE_SIZE),
  }
  const { data } = await apiClient.get<RawFeedResponse | Comment[]>('/comments/feed', { params })
  return normaliseFeedResponse(data)
}

export async function saved(
  filters: Partial<FeedFilters>,
  page: FeedPageOpts,
): Promise<FeedResponse> {
  // Saved view ignores sort + include_toxic per spec.
  const f = { ...filters }
  delete f.sort
  delete f.include_toxic
  const params = {
    ...filtersToParams(f),
    offset: String(page.offset),
    limit: String(page.limit ?? PAGE_SIZE),
  }
  const { data } = await apiClient.get<RawFeedResponse | Comment[]>('/comments/saved', { params })
  return normaliseFeedResponse(data)
}

export async function star(id: string): Promise<void> {
  await apiClient.post(`/comments/${encodeURIComponent(id)}/star`)
}

export async function unstar(id: string): Promise<void> {
  await apiClient.delete(`/comments/${encodeURIComponent(id)}/star`)
}

export interface ExportRequestResponse {
  status: 'sync' | 'async'
  download_url?: string
  expires_at?: string
  job_id?: string
}

export async function requestExport(
  filters: Partial<FeedFilters>,
  format: ExportFormat,
): Promise<ExportRequestResponse> {
  const body = { ...filtersToParams(filters), format }
  // axios surfaces non-2xx as throws by default. Use validateStatus to
  // accept 200 and 202.
  const res = await apiClient.post<{
    download_url?: string
    expires_at?: string
    job_id?: string
  }>('/comments/export', body, { validateStatus: (s) => s === 200 || s === 202 })
  if (res.status === 200) {
    return {
      status: 'sync',
      download_url: res.data.download_url,
      expires_at: res.data.expires_at,
    }
  }
  return { status: 'async', job_id: res.data.job_id }
}

export async function exportStatus(jobId: string): Promise<ExportJob> {
  const { data } = await apiClient.get<ExportJob>(
    `/comments/export/${encodeURIComponent(jobId)}/status`,
  )
  return data
}

export async function recentExports(): Promise<ExportJob[]> {
  const { data } = await apiClient.get<ExportJob[] | null>('/comments/export/recent')
  return data ?? []
}

export function exportDownloadUrl(jobId: string): string {
  // Browser navigation; we just need an absolute URL. apiBaseURL may be
  // either `/api` or a fully qualified origin.
  return `${apiBaseURL}/comments/export/${encodeURIComponent(jobId)}/download`
}
