// Auth flows: route guards, login, signup, session persistence, logout.
//
// These tests drive the real UI (forms, buttons, router) against the mock
// auth surface. Assertions target user-visible outcomes so markup churn
// (class names, icon swaps) never breaks them.
import { expect, gotoAs, loginViaUi, test } from '../support/fixtures'
import { TEST_PASSWORD, UNVERIFIED_EMAIL, VERIFIED_EMAIL } from '../support/seed'

test.describe('route guards', () => {
  test('guest hitting /dashboard is bounced to /login with next preserved', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'guest', '/dashboard')

    await expect(page).toHaveURL(/\/login\?next=\/dashboard$/)
    await expect(page.getByRole('heading', { name: 'Sign in to your account' })).toBeVisible()
  })

  test('unverified user is pinned to /verify-email', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'unverified', '/dashboard')

    await expect(page).toHaveURL(/\/verify-email/)
    await expect(page.getByRole('heading', { name: 'Check your inbox' })).toBeVisible()
  })

  test('verified user without channels is pinned to /onboarding', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'no-channels', '/dashboard')

    await expect(page).toHaveURL(/\/onboarding/)
    await expect(page.getByRole('heading', { name: 'Welcome to Noumena' })).toBeVisible()
  })

  test('authenticated user hitting /login is bounced to /dashboard', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/login')

    await expect(page).toHaveURL(/\/dashboard$/)
  })
})

test.describe('login', () => {
  test('rejects invalid credentials with a visible error', async ({ page, mockState }) => {
    await loginViaUi(page, mockState, VERIFIED_EMAIL, 'WrongPass1')

    // The view surfaces the backend's error message verbatim.
    await expect(page.getByText('invalid credentials')).toBeVisible()
    await expect(page).toHaveURL(/\/login/)
  })

  test('signs in a verified onboarded user into the dashboard', async ({ page, mockState }) => {
    await loginViaUi(page, mockState, VERIFIED_EMAIL, TEST_PASSWORD)

    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.getByRole('heading', { name: 'Insights feed' })).toBeVisible()
    // Session is live — the top bar exposes the signed-in identity.
    await expect(page.getByText(VERIFIED_EMAIL, { exact: true }).first()).toBeVisible()
  })

  test('signs in an unverified user into the verify-email wall', async ({ page, mockState }) => {
    await loginViaUi(page, mockState, UNVERIFIED_EMAIL, TEST_PASSWORD)

    await expect(page).toHaveURL(/\/verify-email/)
    await expect(page.getByRole('heading', { name: 'Check your inbox' })).toBeVisible()
  })

  test('lands on the originally requested page after login (?next)', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'guest', '/dashboard/saved')
    await expect(page).toHaveURL(/\/login\?next=/)

    await page.getByLabel('Email').fill(VERIFIED_EMAIL)
    await page.getByLabel('Password', { exact: true }).fill(TEST_PASSWORD)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()

    await expect(page).toHaveURL(/\/dashboard\/saved$/)
    await expect(page.getByRole('heading', { name: 'Saved comments' })).toBeVisible()
  })

  test('session survives a hard reload (silent refresh recovery)', async ({ page, mockState }) => {
    await loginViaUi(page, mockState, VERIFIED_EMAIL, TEST_PASSWORD)
    await expect(page).toHaveURL(/\/dashboard$/)

    await page.reload()

    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.getByRole('heading', { name: 'Insights feed' })).toBeVisible()
  })

  test('logout from the user menu clears the session', async ({ page, mockState }) => {
    await loginViaUi(page, mockState, VERIFIED_EMAIL, TEST_PASSWORD)
    await expect(page).toHaveURL(/\/dashboard$/)

    await page.getByRole('button', { name: VERIFIED_EMAIL }).click()
    await page.getByRole('menuitem', { name: 'Log out', exact: true }).click()

    await expect(page).toHaveURL(/\/login/)
    await expect(page.getByRole('heading', { name: 'Sign in to your account' })).toBeVisible()
  })
})

test.describe('signup', () => {
  test('weak passwords keep the submit button disabled', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'guest', '/signup')

    const submit = page.getByRole('button', { name: 'Create account' })
    // Empty form: the button is enabled (the browser required-field check
    // handles submission), but a present-and-weak password disables it.
    await expect(submit).toBeEnabled()

    await page.getByLabel('Email').fill('new@noumena.test')
    await page.getByLabel('Password', { exact: true }).fill('weak')

    await expect(submit).toBeDisabled()
  })

  test('valid signup redirects to the verify-email wall', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'guest', '/signup')

    await page.getByLabel('Email').fill('new@noumena.test')
    await page.getByLabel('Password', { exact: true }).fill('Str0ngPass!')
    await page.getByRole('button', { name: 'Create account' }).click()

    await expect(page).toHaveURL(/\/verify-email/)
    await expect(page.getByRole('heading', { name: 'Check your inbox' })).toBeVisible()
  })
})