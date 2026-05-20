// Vitest configuration for the Noumena frontend.
//
// Vitest version: ^4.1.5 — chosen because Vitest 4.x is the first major
// release that lists Vite ^8.0.0 in its peer-dependency range (it accepts
// `^6.0.0 || ^7.0.0 || ^8.0.0`). The project pins Vite ^8.0.1, so any
// Vitest <4 would fail peer-dep resolution against Vite 8.
//
// We import the real `vite.config.ts` and merge a `test` block onto it via
// Vitest's `mergeConfig` helper so that `resolve.alias` (`@` → `src/`) and
// the `@vitejs/plugin-vue` plugin are inherited verbatim from the build
// pipeline. This eliminates the "passes in tests, fails in build" alias
// drift class of bug.
import { fileURLToPath } from 'node:url'
import { mergeConfig } from 'vite'
import { defineConfig } from 'vitest/config'
import viteConfig from './vite.config'

export default mergeConfig(
  viteConfig,
  defineConfig({
    resolve: {
      alias: {
        // Vite resolves `@/...` via tsconfig paths in this project; mirror
        // it explicitly here so Vitest's resolver matches the build path.
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    test: {
      environment: 'happy-dom',
      globals: true,
      include: ['src/**/*.{test,spec}.{ts,tsx}'],
      coverage: {
        provider: 'v8',
        reporter: ['text', 'html'],
        reportsDirectory: './coverage',
        // NOTE: thresholds are documented in README.md (statements 70 /
        // branches 60 / functions 70 / lines 70) but intentionally NOT
        // enforced here. They become a one-line config flip once the
        // suite is large enough that enforcement is meaningful.
      },
    },
  }),
)
