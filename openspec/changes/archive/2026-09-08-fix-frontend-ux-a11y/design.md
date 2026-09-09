## Context

Vue 3 + Tailwind v4 SPA. Strong core flows (optimistic star with rollback, single-flight polling, OAuth popup) but layout/accessibility polish is inconsistent: mobile gets nothing, dark mode is half-implemented, and several small bugs (offset drift, frozen onboarding progress) degrade trust.

## Goals / Non-Goals

**Goals:**
- Full mobile navigation.
- Readable dark mode everywhere.
- Keyboard + SR accessible interactive components.
- Correct pagination/dates; drillable insights; honest trial/plan messaging.

**Non-Goals:**
- Not redesigning the visual identity.
- Not adding i18n.
- Not rewriting the feed virtualization approach beyond what is cheap (window on long lists).

## Decisions

### 1. Mobile navigation

Add a slide-over drawer toggled from a hamburger in the top bar (visible `<lg`), containing the same nav items + workspace switcher + user menu. Reuse the existing transitions (slide + fade). Focus the drawer on open, Escape/backdrop closes, focus restored on close. Keep desktop sidebar unchanged.

### 2. Shared surface + dark-mode audit

Extract `Card.vue` (white/dark variants, ring, radius) and replace the ~12 duplicated `bg-white … ring-1 rounded-2xl` blocks. Audit every view for missing `dark:` variants (headings, secondary text, borders, asides). Add a quick automated check: grep views for `text-zinc-900`/`text-zinc-800` without a `dark:` counterpart.

### 3. Feed correctness

- Track `offset` as its own ref (not `list.length`) in `stores/feed.ts`; on optimistic unstar, decrement offset and keep page math stable.
- `FilterBar.vue` date presets: build from local-date boundaries (`startOfDay` in local tz) instead of `toISOString()`.
- Format `exportJob.expires_at` with `shortDate`.

### 4. Insights drill-down

Make each actionable insight card a link → feed view with a filter on that insight type (+ optional channel). Reuse existing feed filter plumbing; add a `type` query param and `setFilter` in `stores/feed.ts`.

### 5. Trial + plan messaging

- Backend billing already has `trial_ends_at` in the model (write path is a separate gap — see `implement-missing-product-features`); frontend: if a trial date exists, show a countdown banner + upgrade CTA in BillingView and on quota; if not, remove/fix the "14-day free trial" claim.
- Fix Landing FAQ "Pro plan" → actual plan names; keep plan table source-of-truth in `lib/plans.ts`.

### 6. Onboarding progress

Change the polling condition: continue while job is `pending/running` regardless of progress ≥10%; stop on `completed`/`failed`; show a "syncing in background" escape hatch with a link to Channels. This fixes the frozen-bar bug.

### 7. A11y pass (component by component)

- `ConfirmModal`: give the heading a real `id`, bind `aria-labelledby` to it (already has focus trap/Escape).
- `CommentCard` insight pill: `<button>` + `aria-expanded`; 44px hit area.
- `WorkspaceSwitcher`: real `<button>`s, arrow-key + Escape navigation, `role="menu"/"menuitem"`.
- Avatar menu: focus into menu, Escape/arrow nav, focus restore.
- `KeyboardHelpModal`: focus trap + Escape + focus restore (reuse ConfirmModal trap).
- `TextInput`/forms: `aria-describedby` to error id, `aria-invalid`, `role="alert"` on form error containers.
- One `h1` per view (page title), correct h2/h3 below.
- Contrast: bump `zinc-400` → `zinc-500`+ where used for text.
- Tabs (`PlanComparison`, `ActionableInsightsPanel`): arrow-key nav or convert to `aria-pressed` button groups.
- Add `@media (prefers-reduced-motion: reduce)` to disable animations.

### 8. Motion + cleanup

Wrap modals in `<Transition>`, lists in `<transition-group>` (load-more append, unstar remove). Delete `HelloWorld.vue`, `InlineError.vue`, unused `useWorkspaceSwitcher.ts`; set `<title>` per view via a route meta/`useHead`-style helper.

### 9. Discoverability

Add a subtle hint chip for keyboard shortcuts (j/k/s//) in the feed; show an upgrade/plan nudge when the user approaches quota (from billing store) with a CTA to Billing.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Shared Card refactor churns many files | Incremental: introduce Card, migrate view-by-view; visual snapshots in vitest |
| Offset change affects feed correctness | Unit-test the pagination reducer across star/unstar/load-more sequences |
| A11y keyboard work is fiddly | Follow ConfirmModal pattern already present; test with axe + keyboard-only script |
| Removing "14-day trial" claim without trials implemented | Align copy to reality; trials feature is separate change |
