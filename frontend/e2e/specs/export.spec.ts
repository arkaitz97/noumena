// Comment export: sync (200 + signed URL) and async (202 + job polling)
// paths, plus the failure path. The mock completes async jobs on the first
// status poll, so the store's 5s poll converges within one tick.
import { expect, gotoAs, test } from '../support/fixtures'

async function openExportModal(page: import('@playwright/test').Page): Promise<void> {
  await page.getByRole('button', { name: 'Export', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Open export…' }).click()
  await expect(page.getByRole('dialog').getByRole('heading', { name: 'Export comments' })).toBeVisible()
}

test.describe('comment export', () => {
  test('sync export surfaces a ready download link', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await openExportModal(page)
    await page.getByRole('button', { name: 'Start export' }).click()

    // Sync path: the job completes inline — panel + toast + download link.
    await expect(page.getByText('Export Completed')).toBeVisible()
    await expect(page.getByRole('link', { name: 'Download' })).toBeVisible()
    await expect(page.getByText('Export ready — download below.')).toBeVisible()
  })

  test('json format is sent to the backend', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await openExportModal(page)
    await page.getByLabel('Format').selectOption('json')
    await page.getByRole('button', { name: 'Start export' }).click()

    await expect(page.getByText('Export Completed')).toBeVisible()

    const exportCall = mockState.calls.find((c) => c.path === '/api/comments/export')
    expect(exportCall).toBeTruthy()
    expect((exportCall!.body as { format?: string }).format).toBe('json')
  })

  test('async export polls to completion and offers the download', async ({ page, mockState }) => {
    mockState.exportMode = 'async'
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await openExportModal(page)
    await page.getByRole('button', { name: 'Start export' }).click()

    await expect(page.getByText('Export queued', { exact: false })).toBeVisible()
    // The store polls /comments/export/:id/status every 5s; the mock
    // completes on the first poll. Generous budget for CI.
    await expect(page.getByText('Export Completed')).toBeVisible({ timeout: 20_000 })
    await expect(page.getByRole('link', { name: 'Download' })).toBeVisible()
  })

  test('dismiss clears the export surface', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await openExportModal(page)
    await page.getByRole('button', { name: 'Start export' }).click()
    await expect(page.getByText('Export Completed')).toBeVisible()

    await page.getByRole('button', { name: 'Dismiss' }).click()

    await expect(page.getByText('Export Completed')).toHaveCount(0)
  })

  test('failed export surfaces an error toast and stays on the feed', async ({ page, mockState }) => {
    mockState.exportFails = true
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await openExportModal(page)
    await page.getByRole('button', { name: 'Start export' }).click()

    await expect(page.getByText('Could not start your export. Please try again.')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Insights feed' })).toBeVisible()
  })
})