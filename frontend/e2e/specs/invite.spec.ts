// Invite acceptance journeys (fix-workspace-invite-acceptance).
//
// The invite link is a public SPA deep link; acceptance happens over the
// authenticated API after login. Tokens: `valid` succeeds, `seatfull` is at
// the plan cap (409), anything else is the generic 410.
import { expect, gotoAs, test } from '../support/fixtures'
import { TEST_PASSWORD, VERIFIED_EMAIL } from '../support/seed'

const PENDING_KEY = 'noumena_pending_invite_token'

test.describe('invite acceptance', () => {
  test('guest invite link preserves the token and hands off to login', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'guest', '/invite?token=valid')

    await expect(page).toHaveURL(/\/login\?next=\/invite\?token=valid/)
    const token = await page.evaluate((key) => sessionStorage.getItem(key), PENDING_KEY)
    expect(token).toBe('valid')
  })

  test('existing user completes the invite after signing in', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'guest', '/invite?token=valid')
    await expect(page).toHaveURL(/\/login/)

    await page.getByLabel('Email').fill(VERIFIED_EMAIL)
    await page.getByLabel('Password', { exact: true }).fill(TEST_PASSWORD)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()

    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.getByRole('heading', { name: 'Insights feed' })).toBeVisible()
  })

  test('authenticated user accepts directly', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/invite?token=valid')

    await expect(page).toHaveURL(/\/dashboard$/)
  })

  test('seat cap surfaces the upgrade message', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/invite?token=seatfull')

    await expect(page).toHaveURL(/\/invite/)
    await expect(page.getByText('No free seats')).toBeVisible()
    const token = await page.evaluate((key) => sessionStorage.getItem(key), PENDING_KEY)
    expect(token).toBeNull()
  })

  test('expired or consumed invite is explained and the token cleared', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/invite?token=stale')

    await expect(page.getByText('expired or already used')).toBeVisible()
    const token = await page.evaluate((key) => sessionStorage.getItem(key), PENDING_KEY)
    expect(token).toBeNull()
  })
})
