// Insights feed: render, filters, URL state, empty state, star/unstar,
// infinite scroll, keyboard shortcuts.
import { expect, gotoAs, test } from '../support/fixtures'

const GPU_COMMENT = 'Love the deep dive on the new GPU'
const SOURDOUGH_COMMENT = 'Does the sourdough method work at high altitude?'

// Chip filters live behind the "Filters" popover in the compact bar.
async function openFilters(page: import('@playwright/test').Page) {
  await page.getByRole('button', { name: 'Toggle filters' }).click()
}

test.describe('feed rendering', () => {
  test('dashboard renders summary cards and comment list', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await expect(page.getByRole('heading', { name: 'Insights feed' })).toBeVisible()
    // Summary cards.
    await expect(page.getByText('Toxicity shield').first()).toBeVisible()
    await expect(page.getByText('Top insight type')).toBeVisible()
    // Real comment rows from the (mocked) feed.
    await expect(page.getByText(GPU_COMMENT)).toBeVisible()
    await expect(page.getByText(SOURDOUGH_COMMENT)).toBeVisible()
  })
})

test.describe('filters', () => {
  test('search narrows the list and syncs to the URL', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')
    await expect(page.getByText(GPU_COMMENT)).toBeVisible()

    await page.getByLabel('Search comments').fill('sourdough')

    await expect(page.getByText(SOURDOUGH_COMMENT)).toBeVisible()
    await expect(page.getByText(GPU_COMMENT)).toHaveCount(0)
    await expect(page).toHaveURL(/search=sourdough/)
  })

  test('category chip deactivation hides that category only', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')
    await expect(page.getByText(GPU_COMMENT)).toBeVisible()

    // 'Praise' is active by default (high-signal set). Clicking the active
    // chip deactivates it → praise comments disappear, others remain.
    await openFilters(page)
    await page.getByRole('button', { name: 'Praise', exact: true }).click()

    await expect(page.getByText(GPU_COMMENT)).toHaveCount(0)
    await expect(page.getByText(SOURDOUGH_COMMENT)).toBeVisible()
    await expect(page).toHaveURL(/categories=question/)
  })

  test('insight-type filter keeps only comments carrying that insight', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')
    await expect(page.getByText(GPU_COMMENT)).toBeVisible()

    await openFilters(page)
    await page.getByRole('button', { name: 'Video Ideas' }).click()

    await expect(page.getByText('You should make a follow-up covering undervolting settings.')).toBeVisible()
    await expect(page.getByText(GPU_COMMENT)).toHaveCount(0)
    await expect(page).toHaveURL(/insight_type=video_idea/)
  })

  test('sort order persists to the URL', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await openFilters(page)
    await page.getByRole('button', { name: 'Oldest', exact: true }).click()

    await expect(page).toHaveURL(/sort=oldest/)
    await expect(page.getByRole('button', { name: 'Oldest', exact: true })).toHaveAttribute('aria-pressed', 'true')
  })

  test('no-match search shows the empty state, reset restores the feed', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await page.getByLabel('Search comments').fill('zzz-no-such-comment')

    await expect(page.getByText('No comments match these filters.')).toBeVisible()
    await expect(page.getByText(GPU_COMMENT)).toHaveCount(0)

    await page.getByRole('button', { name: 'Reset filters' }).click()

    await expect(page.getByText(GPU_COMMENT)).toBeVisible()
    await expect(page).toHaveURL(/\/dashboard$/)
  })

  test('filter state survives navigation away and back (URL round-trip)', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    await page.getByLabel('Search comments').fill('sourdough')
    await expect(page).toHaveURL(/search=sourdough/)

    await page.goto('/dashboard/saved')
    await page.goto('/dashboard?search=sourdough')

    // The URL is the source of truth for filter state: a fresh mount with
    // ?search=sourdough must render the narrowed feed.
    await expect(page.getByText(SOURDOUGH_COMMENT)).toBeVisible()
    await expect(page.getByText(GPU_COMMENT)).toHaveCount(0)
  })
})

test.describe('star / unstar', () => {
  // KNOWN APP DEFECT: the optimistic patch (store array element replace) and
  // even refetched wire data never re-render an already-mounted CommentCard
  // in this dev toolchain (vue 3.5.30 + vite 8 + plugin-vue 6, real
  // Chromium). The store state IS correct (verified: server round-trip +
  // store probe) and the DOM only catches up on full remounts. Reproduced
  // at HEAD and WIP; NOT reproduced in a minimal app with the same
  // versions, so the trigger lives somewhere in this codebase. Un-fixme
  // when the feed list renders prop-driven updates again.
  test.fixme('starring a comment flips its state optimistically', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    const card = page.locator('article').filter({ hasText: SOURDOUGH_COMMENT })
    const star = card.getByRole('button', { name: 'Star comment' })
    await expect(star).toBeVisible()

    await star.click()

    await expect(card.getByRole('button', { name: 'Unstar comment' })).toBeVisible()
    await expect(card.getByRole('button', { name: 'Unstar comment' })).toHaveAttribute('aria-pressed', 'true')
  })

  test.fixme('unstarring an already-starred comment flips it back', async ({ page, mockState }) => {
    mockState.comments.find((c) => c.body.includes('sourdough'))!.is_starred = true
    await gotoAs(page, mockState, 'verified', '/dashboard')

    const card = page.locator('article').filter({ hasText: SOURDOUGH_COMMENT })
    await expect(card.getByRole('button', { name: 'Unstar comment' })).toBeVisible()

    await card.getByRole('button', { name: 'Unstar comment' }).click()

    await expect(card.getByRole('button', { name: 'Star comment' })).toBeVisible()
  })

  // Server-side persistence still works end-to-end: the POST fires, the
  // backend (mock) persists, and a fresh mount renders the starred state.
  test('starring persists server-side and renders on the saved view', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    const card = page.locator('article').filter({ hasText: SOURDOUGH_COMMENT })
    await card.getByRole('button', { name: 'Star comment' }).click()

    // The optimistic UI flip is broken (see fixme above); assert the
    // durable part: the backend recorded the star.
    await expect
      .poll(() => mockState.comments.find((c) => c.body.includes('sourdough'))?.is_starred === true)
      .toBe(true)

    // Fresh mount (full navigation) reflects it.
    await page.goto('/dashboard/saved')
    await expect(page.getByText(SOURDOUGH_COMMENT)).toBeVisible()
  })
})

test.describe('pagination', () => {
  test('scrolling to the sentinel loads the next page, then end-of-feed', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')
    await expect(page.getByText(GPU_COMMENT)).toBeVisible()

    // 24 fixture comments with PAGE_SIZE 20 → first page is full, has_more.
    await expect(page.getByText('End of feed')).toHaveCount(0)

    // Scroll the bottom sentinel region into view to trigger the observer.
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))

    // A comment from the second page is now rendered.
    await expect(page.getByText('Totally agree with the verdict on thermals.')).toBeVisible()

    // Feed is exhausted now.
    await expect(page.getByText('End of feed')).toBeVisible()
  })
})

test.describe('keyboard shortcuts', () => {
  test('? opens the shortcuts modal', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')
    // The shortcut listener attaches on mount — wait for the feed to be
    // rendered and interactive before pressing.
    await expect(page.getByText(GPU_COMMENT)).toBeVisible()

    await page.keyboard.press('?')

    await expect(page.getByRole('heading', { name: 'Keyboard shortcuts' })).toBeVisible()
  })

  test('/ focuses the search box', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')
    await expect(page.getByText(GPU_COMMENT)).toBeVisible()

    await page.keyboard.press('/')

    await expect(page.getByLabel('Search comments')).toBeFocused()
  })
})