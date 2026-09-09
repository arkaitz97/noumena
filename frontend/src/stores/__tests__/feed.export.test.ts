// Feed export-result regression (fix-frontend-plumbing §6): an async export
// request stores the job, polls until completion, and surfaces the download
// link; polling stops on completion.

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import * as commentsApi from '../../services/comments'
import { useFeedStore } from '../feed'

vi.mock('../../services/comments')
vi.mock('../../lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))

describe('feed store export result surface', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.useFakeTimers()
  })

  it('queues an async export, polls to completion, and exposes the download link', async () => {
    vi.mocked(commentsApi.requestExport).mockResolvedValue({ status: 'async', job_id: 'j1' })
    vi.mocked(commentsApi.exportStatus)
      .mockResolvedValueOnce({ job_id: 'j1', status: 'queued' })
      .mockResolvedValueOnce({
        job_id: 'j1',
        status: 'completed',
        download_url: '/api/comments/export/j1/download',
        expires_at: '2026-09-09T00:00:00Z',
      })

    const feed = useFeedStore()
    await feed.requestExport('csv')
    expect(feed.exportJob?.status).toBe('queued')
    expect(feed.exportPolling).toBe(true)

    // First poll tick: still queued (no-op), keeps polling.
    await vi.advanceTimersByTimeAsync(5000)
    expect(feed.exportJob?.status).toBe('queued')
    expect(feed.exportPolling).toBe(true)

    // Second tick: completed → download link surfaced, polling stops.
    await vi.advanceTimersByTimeAsync(5000)
    expect(feed.exportJob?.status).toBe('completed')
    expect(feed.exportJob?.download_url).toBe('/api/comments/export/j1/download')
    expect(feed.exportPolling).toBe(false)
    await vi.advanceTimersByTimeAsync(15000)
    expect(commentsApi.exportStatus).toHaveBeenCalledTimes(2)
  })

  it('sync exports surface the download link immediately without polling', async () => {
    vi.mocked(commentsApi.requestExport).mockResolvedValue({
      status: 'sync',
      download_url: '/api/comments/export/j2/download',
      expires_at: '2026-09-09T00:00:00Z',
    })
    const feed = useFeedStore()
    await feed.requestExport('json')
    expect(feed.exportJob?.status).toBe('completed')
    expect(feed.exportJob?.download_url).toBe('/api/comments/export/j2/download')
    expect(feed.exportPolling).toBe(false)
  })

  it('clearExportJob dismisses the surface and stops polling', async () => {
    vi.mocked(commentsApi.requestExport).mockResolvedValue({ status: 'async', job_id: 'j3' })
    const feed = useFeedStore()
    await feed.requestExport('csv')
    expect(feed.exportJob).toBeTruthy()
    feed.clearExportJob()
    expect(feed.exportJob).toBeNull()
    expect(feed.exportPolling).toBe(false)
  })
})