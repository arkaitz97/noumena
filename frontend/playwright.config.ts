// Playwright integration-test configuration for the Noumena SPA.
//
// Design goals:
//   1. Deterministic — every spec runs against a mocked `/api` + `/auth`
//      surface (see support/mock.ts), so tests never depend on the real
//      backend's database, Stripe, OAuth providers, or ingestion workers.
//   2. Self-healing — selectors are user-facing (roles, labels, text) and
//      every assertion is web-first (auto-retrying). No sleeps anywhere.
//   3. Isolated — a fresh MockState per test via the `test` fixture in
//      support/fixtures.ts. No shared cookies, no shared database.
//
// Running against a deployed environment instead: point
// PLAYWRIGHT_BASE_URL at it (specs assume a verified, onboarded user
// exists there — see E2E_TEST_EMAIL / E2E_TEST_PASSWORD in the specs).
import { defineConfig, devices } from '@playwright/test'

const PORT = Number(process.env.PLAYWRIGHT_PORT ?? 5173)
const BASE_URL = process.env.PLAYWRIGHT_BASE_URL ?? `http://localhost:${PORT}`

export default defineConfig({
  testDir: './e2e/specs',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? [['github'], ['list']] : 'list',

  // Playwright's own timeout budget. Specs that exercise polling loops
  // (comment export, GDPR export) override locally with a larger budget.
  timeout: 30_000,
  expect: { timeout: 7_000 },

  use: {
    baseURL: BASE_URL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },

  // The SPA is served by `vite preview` of the PRODUCTION build — tests
  // should exercise what ships, and Vite's dev-mode module transform can
  // behave nondeterministically under parallel workers.
  webServer: {
    command: 'npm run build && npx vite preview --port 5173 --strictPort --host 127.0.0.1',
    url: `${BASE_URL}/`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})