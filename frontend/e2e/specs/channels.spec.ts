// Channel management: card rendering, pause/resume, backfill window,
// remove-with-confirm, and the plan-limit gate on the connect button.
import { expect, gotoAs, test } from '../support/fixtures'

test.describe('channels', () => {
  test('lists connected channels with their status detail', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/channels')

    await expect(page.getByRole('heading', { name: 'Connected channels' })).toBeVisible()
    await expect(page.getByText('Tech Reviews', { exact: true })).toBeVisible()
    await expect(page.getByText('Cooking Lab', { exact: true })).toBeVisible()
  })

  test('pause toggles to resume and back', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/channels')

    const card = page.locator('article').filter({ hasText: 'Tech Reviews' })
    await card.getByRole('button', { name: 'Pause', exact: true }).click()

    await expect(page.getByText('Channel paused.')).toBeVisible()
    await expect(card.getByRole('button', { name: 'Resume', exact: true })).toBeVisible()

    await card.getByRole('button', { name: 'Resume', exact: true }).click()

    await expect(page.getByText('Channel resumed.')).toBeVisible()
    await expect(card.getByRole('button', { name: 'Pause', exact: true })).toBeVisible()
  })

  test('backfill window change persists via PATCH', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/channels')

    const card = page.locator('article').filter({ hasText: 'Tech Reviews' })
    await card.getByLabel('Backfill window in days').selectOption('180')

    await expect(page.getByText('Backfill window updated.')).toBeVisible()
    expect(mockState.channels.find((c) => c.display_name === 'Tech Reviews')!.backfill_days).toBe(180)
  })

  test('removing a channel requires confirmation and removes the card', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/channels')

    const card = page.locator('article').filter({ hasText: 'Tech Reviews' })
    await card.getByRole('button', { name: 'Remove channel' }).click()

    // Confirm modal shows the channel name on its confirm button.
    const dialog = page.getByRole('dialog')
    await expect(dialog.getByRole('heading', { name: 'Remove channel' })).toBeVisible()
    await dialog.getByRole('button', { name: 'Remove Tech Reviews' }).click()

    await expect(page.getByText('Channel removed.')).toBeVisible()
    await expect(page.getByText('Tech Reviews', { exact: true })).toHaveCount(0)
    expect(mockState.channels.find((c) => c.display_name === 'Tech Reviews')).toBeUndefined()
  })

  test('cancelling the remove modal keeps the channel', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/channels')

    const card = page.locator('article').filter({ hasText: 'Cooking Lab' })
    await card.getByRole('button', { name: 'Remove channel' }).click()
    await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()

    await expect(page.getByText('Cooking Lab', { exact: true })).toBeVisible()
  })

  test('connect modal offers the backfill window picker', async ({ page, mockState }) => {
    // Lift the plan limit so the connect button is enabled.
    mockState.billing.channels_limit = 5
    await gotoAs(page, mockState, 'verified', '/settings/channels')

    await page.getByRole('button', { name: 'Connect YouTube channel' }).click()

    const dialog = page.getByRole('dialog')
    await expect(dialog.getByRole('heading', { name: 'Connect YouTube channel' })).toBeVisible()
    await expect(dialog.getByLabel('Backfill window')).toBeVisible()

    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await expect(dialog).toHaveCount(0)
  })

  test('channel limit reached disables connect and links to billing', async ({ page, mockState }) => {
    // Default seed: 2 channels used against a 1-channel free-plan limit.
    await gotoAs(page, mockState, 'verified', '/settings/channels')

    await expect(page.getByRole('button', { name: 'Connect YouTube channel' })).toBeDisabled()
    await expect(page.getByText('Channel limit reached.')).toBeVisible()
    await expect(page.getByRole('link', { name: 'Upgrade your plan' })).toBeVisible()
  })
})