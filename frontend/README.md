# Noumena Frontend (Vue 3 + TypeScript + Vite)

The web SPA for Noumena. Built with Vue 3 `<script setup>`, Pinia, Vue Router 4, Axios, and Tailwind v4.

## Development

```bash
npm install
npm run dev      # starts Vite at http://localhost:5173
npm run build    # type-checks (vue-tsc) + production build
npm run preview  # serves the production build locally
```

The dev server proxies `/api` and `/auth` to `http://localhost:8080` (the Go backend) so cookies set by `/auth/*` are first-party.

## Environment variables

| Variable             | Default | Purpose                                                                                                                                            |
| -------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VITE_API_BASE_URL`  | `/api`  | Base URL prefixed onto every `apiClient` request. The shared client also exposes `authBaseURL` (`<base>/../auth` when relative, otherwise `/auth`). |

In development the default empty/`/api` value plus the Vite proxy is enough. In production set both `/api` and `/auth` to point at the same origin (recommended) so the refresh-token cookie travels first-party.

## Auth model

- Access token lives only in the Pinia `auth` store memory — never in `localStorage` / `sessionStorage`.
- Refresh token is set by the backend as `HttpOnly; Secure; SameSite=Lax` and travels automatically on `POST /auth/refresh`.
- The Axios response interceptor performs single-flight refresh on `401` and redirects to `/verify-email` on `403 { code: "email_not_verified" }`.
- See `STYLEGUIDE.md` for visual conventions.

## Testing

The suite is run by [Vitest](https://vitest.dev) (^4.x, the first major that
supports Vite 8 as a peer). Tests live next to the code under
`src/**/__tests__/*.test.ts` (or `*.spec.ts`) and run in a `happy-dom`
environment by default.

```bash
npm test            # one-shot run, exits non-zero on any failure (CI)
npm run test:watch  # watch mode for local development
npm run test:ui     # Vitest browser UI
npm run test:coverage  # one-shot run + V8 coverage report (text + HTML in coverage/)
```

CI entrypoint: `bash scripts/test-frontend.sh` from the repo root runs
`npm ci && npm test` inside `frontend/` and exits non-zero if either step
fails.

### Coverage thresholds (aspirational, not enforced)

The project targets the following coverage levels. They are **not**
currently enforced by `npm run test:coverage` — the suite is too young
for enforcement to be meaningful, and `vitest.config.ts` deliberately
omits `coverage.thresholds.*`. Tightening them is a one-line config flip
once coverage is broad enough.

| Metric     | Target |
| ---------- | ------ |
| Statements | ≥ 70%  |
| Branches   | ≥ 60%  |
| Functions  | ≥ 70%  |
| Lines      | ≥ 70%  |
