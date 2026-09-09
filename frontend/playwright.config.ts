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
// Running against the real backend instead: set E2E_REAL_API=1 and point
// PLAYWRIGHT_BASE_URL at a fully provisioned environment. The specs use
// env-driven credentials (E2E_TEST_EMAIL / E2E_TEST_PASSWORD) and seed
// nothing — they assume a verified, onboarded user exists.
import { defineConfig, devices } from '@playwright/test'

const REAL_API = process.env.E2E_REAL_API === '1'
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

  // The SPA is served by the Vite dev server. In mock mode no backend is
  // required: all `/api` and `/auth` traffic is intercepted in the browser
  // by the mock harness before it leaves the page.
  webServer: REAL_API
    ? {
        command: 'npm run dev -- --port 5173 --strictPort',
        url: BASE_URL,
        reuseExistingServer: !process.env.CI,
        timeout: 60_000,
      }
    : {
        command: 'npm run dev -- --port 5173 --strictPort',
        url: `${BASE_URL}/`,
        reuseExistingServer: !process.env.CI,
        timeout: 60_000,
      },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})