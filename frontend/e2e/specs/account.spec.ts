// Account: profile, password change (error + success), two-step account
// deletion guarded by email match, and the GDPR data-export surface.
import { expect, gotoAs, loginViaUi, test } from '../support/fixtures'
import { TEST_PASSWORD, VERIFIED_EMAIL } from '../support/seed'

const NEW_PASSWORD = 'NewSup3rSecret!'

test.describe('password change', () => {
  test('rejects a wrong current password with an inline error', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/account')

    await page.getByLabel('Current password', { exact: true }).fill('WrongCurrent1')
    await page.getByLabel('New password', { exact: true }).fill(NEW_PASSWORD)
    await page.getByLabel('Confirm new password', { exact: true }).fill(NEW_PASSWORD)
    await page.getByRole('button', { name: 'Update password' }).click()

    await expect(page.getByRole('alert').getByText('Current password is incorrect.')).toBeVisible()
  })

  test('mismatched confirmation blocks submission', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/account')

    await page.getByLabel('Current password', { exact: true }).fill(TEST_PASSWORD)
    await page.getByLabel('New password', { exact: true }).fill(NEW_PASSWORD)
    await page.getByLabel('Confirm new password', { exact: true }).fill('SomethingElse1')

    await expect(page.getByText('Passwords do not match')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Update password' })).toBeDisabled()
  })

  test('a valid change succeeds and the new password works afterwards', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/account')

    await page.getByLabel('Current password', { exact: true }).fill(TEST_PASSWORD)
    await page.getByLabel('New password', { exact: true }).fill(NEW_PASSWORD)
    await page.getByLabel('Confirm new password', { exact: true }).fill(NEW_PASSWORD)
    await page.getByRole('button', { name: 'Update password' }).click()

    await expect(page.getByText('Password updated.')).toBeVisible()

    // The mock now enforces the rotated credential: old password fails,
    // new one signs in.
    await page.getByRole('button', { name: VERIFIED_EMAIL }).click()
    await page.getByRole('menuitem', { name: 'Log out', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Sign in to your account' })).toBeVisible()

    await page.getByLabel('Email').fill(VERIFIED_EMAIL)
    await page.getByLabel('Password', { exact: true }).fill(TEST_PASSWORD)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
    await expect(page.getByText('invalid credentials')).toBeVisible()

    await page.getByLabel('Password', { exact: true }).fill(NEW_PASSWORD)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
    await expect(page).toHaveURL(/\/dashboard$/)
  })
})

test.describe('account deletion', () => {
  test('two-step flow requires the exact email before confirming', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/account')

    await page.getByRole('button', { name: 'Delete account' }).click()

    const dialog = page.getByRole('dialog')
    await expect(dialog.getByRole('heading', { name: 'Delete your account?' })).toBeVisible()
    await dialog.getByRole('button', { name: 'I understand, continue' }).click()

    await expect(dialog.getByRole('heading', { name: 'Confirm account deletion' })).toBeVisible()

    // Mismatched email keeps the destructive button disabled.
    await dialog.getByPlaceholder('Enter your email').fill('wrong@noumena.test')
    await expect(dialog.getByRole('button', { name: 'Permanently delete my account' })).toBeDisabled()

    // Exact email match enables it.
    await dialog.getByPlaceholder('Enter your email').fill(VERIFIED_EMAIL)
    await expect(dialog.getByRole('button', { name: 'Permanently delete my account' })).toBeEnabled()
  })

  test('confirmed deletion signs the user out to the login page', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/account')

    await page.getByRole('button', { name: 'Delete account' }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByRole('button', { name: 'I understand, continue' }).click()
    await dialog.getByPlaceholder('Enter your email').fill(VERIFIED_EMAIL)
    await dialog.getByRole('button', { name: 'Permanently delete my account' }).click()

    await expect(page.getByText('Your account is scheduled for deletion.')).toBeVisible()
    await expect(page).toHaveURL(/\/login$/)
    await expect(mockState.sessionUserEmail).toBeNull()
  })
})

test.describe('GDPR data export', () => {
  test('requesting an export surfaces the download once ready', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/settings/account')

    await page.getByRole('button', { name: 'Export my data (GDPR)' }).click()

    // The mock answers the job as completed immediately, so the store's
    // poller converges on the first tick and the download button appears.
    await expect(page.getByText('Status: completed')).toBeVisible({ timeout: 20_000 })
    await expect(page.getByRole('button', { name: 'Download (.zip)' })).toBeVisible()
  })
})