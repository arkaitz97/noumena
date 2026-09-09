// Onboarding wizard: guard gate, step flow, popup-blocked error path, and
// the backfill-polling escape hatch.
import { expect, gotoAs, test } from '../support/fixtures'

test.describe('onboarding', () => {
  test('verified user without channels lands on the welcome step', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'no-channels', '/dashboard')

    await expect(page).toHaveURL(/\/onboarding/)
    await expect(page.getByRole('heading', { name: 'Welcome to Noumena' })).toBeVisible()
    await expect(page.getByText('Step 1 of 4 · Welcome')).toBeVisible()
  })

  test('getting started advances to the connect step', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'no-channels', '/onboarding')

    await page.getByRole('button', { name: 'Get started' }).click()

    await expect(page.getByRole('heading', { name: 'Connect your YouTube channel' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Continue with YouTube' })).toBeVisible()
  })

  test('blocked popups surface a clear error and stay on the wizard', async ({ page, mockState }) => {
    // Simulate a popup blocker: window.open returns null → the OAuth helper
    // rejects with popup_blocked → the wizard shows the friendly error.
    // MUST be installed before the page loads.
    await page.addInitScript(() => {
      window.open = () => null as unknown as Window
    })
    await gotoAs(page, mockState, 'no-channels', '/onboarding')
    await page.getByRole('button', { name: 'Get started' }).click()

    await page.getByRole('button', { name: 'Continue with YouTube' }).click()

    await expect(
      page.getByText('Pop-up blocked. Please allow pop-ups and try again.'),
    ).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Connect your YouTube channel' })).toBeVisible()
  })

/** The popup helper attaches its message listener asynchronously (after the
 *  auth-URL fetch resolves), so deliver the oauth-code message with a
 *  bounded retry until the wizard advances to the sync step. The wizard
 *  generates its own state token (sessionStorage) — echo it back. */
async function deliverOauthCode(page: import('@playwright/test').Page): Promise<void> {
  for (let attempt = 0; attempt < 20; attempt++) {
    const onStep3 = await page
      .getByRole('heading', { name: 'Syncing your channel' })
      .isVisible()
      .catch(() => false)
    if (onStep3) return
    await page.evaluate(() => {
      const state = sessionStorage.getItem('oauth_state_youtube') ?? ''
      window.dispatchEvent(
        new MessageEvent('message', {
          origin: window.location.origin,
          data: { type: 'oauth-code', provider: 'youtube', code: 'mock-code', state },
        }),
      )
    })
    await page.waitForTimeout(250)
  }
  throw new Error('onboarding never reached the sync step — oauth message not accepted')
}

test('backfill progress below the threshold keeps Continue locked', async ({ page, mockState }) => {
    mockState.backfillProgress = 5

    // Stub the OAuth popup: window.open returns a fake window and the
    // wizard's popup helper receives the code via postMessage.
    await page.addInitScript(() => {
      window.open = () => ({ closed: false, location: { href: '' }, close: () => {} }) as unknown as Window
    })

    await gotoAs(page, mockState, 'no-channels', '/onboarding')
    await page.getByRole('button', { name: 'Get started' }).click()
    await page.getByRole('button', { name: 'Continue with YouTube' }).click()

    await deliverOauthCode(page)

    await expect(page.getByRole('button', { name: /Syncing…/ })).toBeDisabled()
  })

  test('completed backfill unlocks Continue and lands on the dashboard', async ({ page, mockState }) => {
    // Default mock progress is 100% → first poll completes the sync.
    await page.addInitScript(() => {
      window.open = () => ({ closed: false, location: { href: '' }, close: () => {} }) as unknown as Window
    })

    await gotoAs(page, mockState, 'no-channels', '/onboarding')
    await page.getByRole('button', { name: 'Get started' }).click()
    await page.getByRole('button', { name: 'Continue with YouTube' }).click()

    await deliverOauthCode(page)

    // The mock exchange registered the new channel, so after the sync
    // completes the wizard auto-navigates into the real dashboard.
    await expect(page.getByText('Sync complete! Your insights are ready.')).toBeVisible()
    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.getByRole('heading', { name: 'Insights feed' })).toBeVisible()
  })
})