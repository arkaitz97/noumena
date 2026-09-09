## Why

A dedicated UI/UX + a11y audit found a top-severity blocker and a wide set of polish/accessibility defects:

1. **Mobile navigation absent (blocker)** — `frontend/src/views/dashboard/DashboardLayout.vue:92,142` sidebar + workspace switcher are `hidden lg:flex`; on mobile, users cannot reach Saved/Channels/Billing/Account or switch workspaces. No hamburger/drawer anywhere.
2. **Dark mode broken on several pages** — settings/saved/landing headings are `text-zinc-900` with no `dark:` variant (`BillingView.vue:113`, `AccountView.vue:177`, `ChannelsView.vue:82`, `SavedCommentsView.vue:105`) → dark-on-dark, unreadable; card surfaces inconsistent (`InsightsFeed.vue:230` dark vs white aside panels).
3. **Pagination/date bugs** — saved-view optimistic unstar splices a row → next-page offset drifts (`stores/feed.ts:159-165`); `FilterBar.vue:122` date presets use `toISOString()` (UTC) → off-by-one day; `AccountView.vue:316-318` shows raw ISO `expires_at`.
4. **Insights not drillable** — `ActionableInsightsPanel.vue:51-64` top insights aren't clickable; users can't see which comments fired an insight.
5. **Trial promise vs reality** — `LandingView.vue:67` promises "14-day free trial" with no trial UI/countdown; FAQ references a nonexistent "Pro" plan (plans are Creator/Studio/Agency).
6. **Onboarding backfill progress freezes** — `OnboardingView.vue:76-79` stops polling once progress ≥10% → bar frozen, "sync complete" never fires.
7. **A11y gaps** — `ConfirmModal.vue:112` `:aria-labelledby` binds a text value not an id; `CommentCard.vue:124-130` insight pill is a non-focusable `<span>`; `WorkspaceSwitcher.vue:85-97` menu is clickable `<div>`s with no keyboard support; avatar menu no focus/Escape; `KeyboardHelpModal` no focus trap/Escape; `TextInput.vue:46-49` errors not `aria-describedby`/`aria-invalid`; no `role="alert"` on form errors; no h1 in dashboard views; `text-zinc-400` contrast fails AA; icon buttons ~32px < 44px target; `role="tablist"` without arrow-key nav; no `prefers-reduced-motion`.
8. **Motion/consistency** — modals open with no transition; list add/remove has no `<transition-group>`; card styles duplicated ~12× with drift; insight-type label maps duplicated 4×.
9. **Dead code** — `HelloWorld.vue`, `InlineError.vue`, unused `useWorkspaceSwitcher.ts`; `<title>frontend</title>` placeholder.
10. **Discoverability** — keyboard shortcuts only discoverable by pressing `?`; no upgrade nudge at quota.

## What Changes

- Add mobile navigation (drawer/hamburger) including workspace switcher.
- Systematic dark-mode audit across all views; extract a shared `Card`/`Surface` component; dedupe insight-type labels into one lib.
- Fix pagination offset (track independently of list length); local-date filter presets; format export expiry.
- Make insights clickable → drill into the feed filtered by insight type.
- Add trial state/countdown to billing UI + align marketing copy (Landing/FAQ).
- Fix onboarding backfill polling to run until 100%/completion.
- A11y pass: real ids + aria, keyboard-accessible menus/pills/tabs, focus traps + Escape, `role="alert"`, h1 per view, contrast, 44px targets, `prefers-reduced-motion`.
- Add modal/list transitions; remove dead components; set real `<title>`.
- Add a keyboard-shortcut hint and an upgrade/plan nudge at quota.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `frontend-dashboard`: Mobile navigation; keyboard- and screen-reader-accessible components; readable dark mode; insights drill-down; accurate trial and quota feedback.

## Impact

- `frontend/src/views/dashboard/DashboardLayout.vue`, `InsightsFeed.vue`, `ActionableInsightsPanel.vue`, `WeeklyDigestCard.vue`, `PlanComparison.vue`, `BillingView.vue`, `ChannelsView.vue`, `AccountView.vue`, `SavedCommentsView.vue`, `OnboardingView.vue`, `LandingView.vue`, `SignupView.vue`, `LoginView.vue`
- `frontend/src/components/*` (CommentCard, FilterBar, ConfirmModal, KeyboardHelpModal, WorkspaceSwitcher, TextInput, RateLimitBanner, forms/*)
- `frontend/src/stores/feed.ts`, `frontend/src/composables/*`, `frontend/src/lib/*`
- New: `Card.vue`/`Surface`, insight-type label lib, mobile nav component
- Tests: vitest specs for feed pagination, filters, onboarding polling; component a11y tests
