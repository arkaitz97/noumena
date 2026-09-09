// Custom Playwright fixture: wires a fresh MockState + mock API onto every
// test's page. Also ships the navigation/session helpers the specs use so
// test bodies stay declarative.
import { test as base, type Page } from '@playwright/test'
import { installMockApi } from './mock'
import { applyProfile, createMockState, type MockState, type SessionProfile } from './seed'

export interface AppFixtures {
  mockState: MockState
}

export const test = base.extend<AppFixtures>({
  // Fresh, isolated backend state per test — installed as route handlers
  // BEFORE the app loads so bootstrap (POST /auth/refresh) is already mocked.
  mockState: async ({ page }, use) => {
    const state = createMockState()
    await installMockApi(page, state)
    // Surface browser-side failures in the test output — a silent console
    // error from the app is usually the real reason an interaction "did
    // nothing".
    page.on('console', (msg) => {
      if (msg.type() === 'error') console.error('[app-console]', msg.text())
    })
    page.on('pageerror', (err) => console.error('[app-pageerror]', err.message))
    await use(state)
  },
})

export { expect } from '@playwright/test'

/**
 * Navigate to `path` with the mock session configured to one of the guard
 * archetypes. The SPA bootstraps on load (silent refresh → profile →
 * channels → role) exactly like a real page load, so guard redirects and
 * first-paint state are exercised for real.
 */
export async function gotoAs(
  page: Page,
  state: MockState,
  profile: SessionProfile,
  path = '/dashboard',
): Promise<void> {
  applyProfile(state, profile)
  await page.goto(path)
}

/** Log in through the real UI form (used by auth.spec — everything else
 *  relies on gotoAs for speed). */
export async function loginViaUi(
  page: Page,
  state: MockState,
  email: string,
  password: string,
): Promise<void> {
  applyProfile(state, 'guest')
  await page.goto('/login')
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
}