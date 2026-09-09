// Failure paths: rate-limit banner, refresh-token loss, feed query
// timeouts, and server errors. These are the fragile edges of the app —
// each one is injected deterministically through the mock, never through
// timing luck.
import { expect, gotoAs, loginViaUi, test } from '../support/fixtures'
import { TEST_PASSWORD, VERIFIED_EMAIL } from '../support/seed'

test.describe('rate limiting', () => {
  test('429 on the feed surfaces the rate-limit banner with a countdown', async ({ page, mockState }) => {
    mockState.failNext = { urlPart: '/api/comments/feed', status: 429 }
    await gotoAs(page, mockState, 'verified', '/dashboard')

    // Banner renders with the Retry-After window from the response. The
    // countdown ticks down each second, so match any remaining value.
    await expect(page.getByRole('alert').filter({ hasText: 'Rate limit exceeded' })).toBeVisible()
    await expect(page.getByText(/Retry available in \d+s\./)).toBeVisible()

    // Dismissible.
    await page.getByRole('alert').getByRole('button', { name: 'Dismiss' }).click()
    await expect(page.getByRole('alert').filter({ hasText: 'Rate limit exceeded' })).toHaveCount(0)
  })
})

test.describe('token lifecycle', () => {
  test('an expired refresh token bounces to /login preserving the destination', async ({ page, mockState }) => {
    mockState.refreshFails = true
    await gotoAs(page, mockState, 'verified', '/dashboard')

    // Bootstrap's silent refresh failed → session reset → guard bounce.
    await expect(page).toHaveURL(/\/login\?next=\/dashboard$/)
    await expect(page.getByRole('heading', { name: 'Sign in to your account' })).toBeVisible()
  })

  test('a 401 mid-session triggers the single-flight refresh and retries', async ({ page, mockState }) => {
    await loginViaUi(page, mockState, VERIFIED_EMAIL, TEST_PASSWORD)
    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.getByRole('heading', { name: 'Insights feed' })).toBeVisible()

    // The next feed request returns 401 once; the interceptor refreshes
    // (mock refresh stays healthy) and retries — content keeps rendering.
    mockState.failNext = { urlPart: '/api/comments/feed', status: 401 }
    await page.getByLabel('Search comments').fill('gpu')

    await expect(page.getByText('Love the deep dive on the new GPU')).toBeVisible()
    // No forced logout happened — still on the dashboard, still signed in.
    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.getByText(VERIFIED_EMAIL, { exact: true }).first()).toBeVisible()
  })
})

test.describe('feed failures', () => {
  test('408 feed-query timeout keeps existing rows and warns the user', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')
    await expect(page.getByText('Love the deep dive on the new GPU')).toBeVisible()

    mockState.feedTimesOut = true
    // Trigger a refetch through the UI (search) — the 408 fires.
    await page.getByLabel('Search comments').fill('gpu')

    await expect(page.getByText('That query took too long. Try narrowing your filters.')).toBeVisible()
    // Per spec: already-rendered rows are preserved on timeout.
    await expect(page.getByText('Love the deep dive on the new GPU')).toBeVisible()
  })

  test('500 on the feed shows a generic error toast and an empty list', async ({ page, mockState }) => {
    mockState.feedServerError = true
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await expect(page.getByText('Failed to load comments. Please try again.')).toBeVisible()
    await expect(page.getByText('No comments match these filters.')).toBeVisible()
  })
})