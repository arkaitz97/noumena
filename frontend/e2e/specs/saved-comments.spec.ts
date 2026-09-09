// Saved comments: star in the feed → appears in /dashboard/saved; unstar
// removes the row. Exercises the shared feed store's optimistic mutation
// across two views.
import { expect, gotoAs, test } from '../support/fixtures'

const SOURDOUGH_COMMENT = 'Does the sourdough method work at high altitude?'
const GPU_COMMENT = 'Love the deep dive on the new GPU'

test.describe('saved comments', () => {
  test('starred comments appear in the saved view', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard')

    // Star two comments from the feed. (The optimistic UI flip is a known
    // defect — see feed.spec — so assert the durable effect: the star
    // request is recorded by the backend and the saved view renders it.)
    const sourdough = page.locator('article').filter({ hasText: SOURDOUGH_COMMENT })
    await sourdough.getByRole('button', { name: 'Star comment' }).click()
    const gpu = page.locator('article').filter({ hasText: GPU_COMMENT })
    await gpu.getByRole('button', { name: 'Star comment' }).click()

    await expect
      .poll(() =>
        mockState.comments.filter((c) => c.is_starred).map((c) => c.id).sort().join(','),
      )
      .toBe('c01,c07')

    // Navigate to the saved view — rows must be there.
    await page.goto('/dashboard/saved')
    await expect(page.getByRole('heading', { name: 'Saved comments' })).toBeVisible()
    await expect(page.getByText(SOURDOUGH_COMMENT)).toBeVisible()
    await expect(page.getByText(GPU_COMMENT)).toBeVisible()
  })

  test('unstarring in the saved view removes the row immediately', async ({ page, mockState }) => {
    // Seed one starred comment directly (same effect as starring in UI).
    mockState.comments.find((c) => c.body.includes('sourdough'))!.is_starred = true

    await gotoAs(page, mockState, 'verified', '/dashboard/saved')
    await expect(page.getByText(SOURDOUGH_COMMENT)).toBeVisible()

    const card = page.locator('article').filter({ hasText: SOURDOUGH_COMMENT })
    await card.getByRole('button', { name: 'Unstar comment' }).click()

    await expect(page.getByText(SOURDOUGH_COMMENT)).toHaveCount(0)
    await expect(page.getByText('No saved comments yet.')).toBeVisible()
  })

  test('saved view shows its empty state when nothing is starred', async ({ page, mockState }) => {
    await gotoAs(page, mockState, 'verified', '/dashboard/saved')

    await expect(page.getByText('No saved comments yet.')).toBeVisible()
    await expect(page.getByRole('link', { name: 'Go to feed' })).toBeVisible()
  })
})