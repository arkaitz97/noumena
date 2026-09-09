// Billing: plan summary, usage alerts, trial countdown, upgrade → Stripe
// Checkout, portal gating on free plans.
import { expect, gotoAs, test } from '../support/fixtures'

test.describe('billing', () => {
  test('shows the current plan and usage alert when at/near limits', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/billing')

    await expect(page.getByRole('heading', { name: 'Billing & subscription' })).toBeVisible()
    // Default seed: free plan, 2/1 channels → usage alert with channel copy.
    await expect(page.getByRole('alert').filter({ hasText: "You're approaching your plan limit." })).toBeVisible()
    await expect(page.getByText("You've reached your channel limit (2/1).")).toBeVisible()
    await expect(page.getByText('free', { exact: true })).toBeVisible()
  })

  test('portal button is disabled on the free plan', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/billing')

    await expect(page.getByRole('button', { name: 'Manage payment & invoices' })).toBeDisabled()
  })

  test('trial countdown banner is shown for active trials', async ({ page, mockState }) => {
    mockState.billing.trial = {
      active: true,
      ends_at: new Date(Date.now() + 5 * 86_400_000).toISOString(),
      days_remaining: 5,
    }
    await gotoAs(page, mockState, 'verified', '/settings/billing')

    await expect(page.getByText('Free trial ends in 5 days')).toBeVisible()
    await expect(
      page.getByRole('status').getByRole('button', { name: 'Upgrade', exact: true }),
    ).toBeVisible()
  })

  test('upgrading to Creator opens Stripe Checkout with the chosen interval', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/billing')

    await page.getByRole('button', { name: 'Upgrade', exact: true }).first().click()

    // The checkout request is fired against /api/billing/checkout.
    await expect
      .poll(() => mockState.calls.some((c) => c.path === '/api/billing/checkout'), {
        message: 'expected a /api/billing/checkout call',
      })
      .toBe(true)
    const call = mockState.calls.find((c) => c.path === '/api/billing/checkout')!
    expect(call.body).toMatchObject({ plan: 'creator', interval: 'monthly' })
  })

  test('switching to annual billing keeps plan upgrade calls on the same interval', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/billing')

    await page.getByRole('tab', { name: 'Annual' }).click()
    await page.getByRole('button', { name: 'Upgrade', exact: true }).first().click()

    await expect
      .poll(() => mockState.calls.some((c) => c.path === '/api/billing/checkout'))
      .toBe(true)
    const call = mockState.calls.find((c) => c.path === '/api/billing/checkout')!
    expect(call.body).toMatchObject({ plan: 'creator', interval: 'annual' })
  })
})