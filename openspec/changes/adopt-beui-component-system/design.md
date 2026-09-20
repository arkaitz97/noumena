## Context

The Noumena frontend is a Vue 3 (Composition API) + Vite + TypeScript + Tailwind v4 SPA (`openspec/project.md`). It currently has no component-library dependency: primitives such as `forms/PrimaryButton.vue`, `forms/TextInput.vue`, `SegmentedControl.vue`, `ConfirmModal.vue`, `ToastHost.vue`, and `KeyboardHelpModal.vue` are hand-written with duplicated Tailwind utilities and generic CSS transitions. Native `<select>` and `<input type="checkbox">` elements appear across `AccountView`, `ChannelsView`, `MembersView`, `OnboardingView`, `LoginView`, `FilterBar`, and `InsightsFeed`.

beUI (beui.dev, MIT) is a copy-paste registry of animated components built on React, Next.js, Motion (framer-motion), and shadcn/ui semantic tokens. Its interaction quality (spring-pressed buttons, morphing modals, animated tab indicators, swipe-dismiss toasts) is the target aesthetic, but its source is `.tsx` and cannot be dropped into a Vue app.

Two hard constraints shape the design:
1. **Runtime mismatch** — the app is Vue, beUI is React.
2. **Token mismatch** — beUI styles use shadcn semantic tokens (`bg-card`, `text-muted-foreground`, `border-border`, `ring-ring`); Noumena uses raw `zinc-*` and custom `brand-*` classes defined in `frontend/src/style.css` `@theme`. No semantic tokens exist today.

## Goals / Non-Goals

**Goals:**
- Port beUI's relevant component source to Vue SFCs under `frontend/src/components/ui/`, preserving its animation character and props API where practical.
- Add a semantic token layer so ported source stays close to upstream and theming lives in one place.
- Migrate core primitives (wave 1) and feedback/surface components (wave 2) into all existing call-sites.
- Preserve accessibility: focus trapping/restoration, Escape-to-close, ARIA roles, and `prefers-reduced-motion`.
- Keep the change frontend-only: no backend, API, schema, or billing changes.

**Non-Goals:**
- Migrating the app to React.
- Porting beUI AI-agent components, finance charts, or exotic decorations (shader background, marquee, cylinder carousel, dock, wheel picker).
- Animated Sidebar, view-transition Theme Toggle, Number/Text Animation, OTP input, 404 pages, Table, or Bottom Sheet (deferred backlog).
- Changing backend behavior or public API contracts.

## Decisions

### D1: Port to Vue with `motion-v`, not CSS-only or a React rewrite
Use `motion-v` (motiondivision/motion-vue, MIT) — the official Motion for Vue — as the animation engine. It exposes the same `motion.*` component and supports `layout`/`layoutId` shared-layout transitions, so beUI's spring indicators (tabs pill, switch thumb, morphing modal) translate directly.

- **Alternative — CSS/`<Transition>` only:** lighter bundle, but loses shared-layout morphs and spring physics; tabs/modals would degrade. Rejected.
- **Alternative — migrate frontend to React:** verbatim beUI copies, but rewrites the entire Vue SPA. Rejected as disproportionate.

### D2: Semantic token layer mapped to the existing palette
Add shadcn-compatible variables to `@theme` in `style.css`, resolved from the existing ramps, with `.dark` overrides. Components consume semantic utilities only.

| Token | Light | Dark | Source |
|---|---|---|---|
| `--color-background` | `#f7f7fb` | `#14151e` | zinc-50 / zinc-950 |
| `--color-foreground` | `#1c1e2a` | `#eeeff5` | zinc-900 / zinc-100 |
| `--color-card` | `#ffffff` | `#1c1e2a` | white / zinc-900 |
| `--color-card-foreground` | `#1c1e2a` | `#eeeff5` | zinc-900 / zinc-100 |
| `--color-popover` | `#ffffff` | `#1c1e2a` | white / zinc-900 |
| `--color-primary` | `#2d7f9c` | `#7cc3da` | brand-600 / brand-400 |
| `--color-primary-foreground` | `#ffffff` | `#14151e` | white / zinc-950 |
| `--color-secondary` | `#eeeff5` | `#2c2f3f` | zinc-100 / zinc-800 |
| `--color-muted` | `#eeeff5` | `#2c2f3f` | zinc-100 / zinc-800 |
| `--color-muted-foreground` | `#6f7389` | `#a3a6bb` | zinc-500 / zinc-400 |
| `--color-accent` | `#e4f5fa` | `#1c4757` | brand-100 / brand-900 |
| `--color-destructive` | `#e11d48` | `#fb7185` | rose-600 / rose-400 |
| `--color-border` | `#e1e2ec` | `#2c2f3f` | zinc-200 / zinc-800 |
| `--color-input` | `#e1e2ec` | `#2c2f3f` | zinc-200 / zinc-800 |
| `--color-ring` | `#57acc8` | `#7cc3da` | brand-500 / brand-400 |

- **Alternative — rewrite every ported class to `zinc-*`/`brand-*`:** diverges from upstream, making future beUI updates manual. Rejected.
- Note: the existing `STYLEGUIDE.md` references IBM Plex while `style.css` loads Geist; the port follows `style.css` (Geist). Styleguide drift is out of scope but flagged.

### D3: Component layout and file ownership
- `frontend/src/components/ui/<component>/` — one folder per primitive with an `index.ts` barrel, matching shadcn conventions, so ported source imports remain recognizable.
- `frontend/src/lib/cn.ts` — `cn()` via `clsx` + `tailwind-merge`.
- Icons: replace beUI's shadcn/React icons with `lucide-vue-next` (already a dependency).
- Upstream provenance and the full beUI → Vue mapping live in `docs/beui-component-map.md`.

### D4: Porting translation rules
- JSX → SFC `<template>`; `motion.div` → `<motion.div>` from `motion-v`; React props → typed `defineProps`; `useState` → `ref`/`computed`; slots replace `children`.
- React compound components → Vue components with named slots or `provide`/`inject` for context (e.g., Tabs, Select, Toast).
- Keep class strings verbatim except shadcn semantic tokens (which now resolve) and motion prop syntax.
- Controlled/uncontrolled duality collapses to `v-model` plus an optional uncontrolled mode where the existing call-site needs it.

### D5: Incremental migration with call-site compatibility
Migrate view-by-view behind the existing component names where possible, then delete superseded files. Existing Vitest specs for replaced components are rewritten against the new primitives. Accessibility behavior is treated as an invariant (spec `Component Accessibility`), not a nice-to-have.

### D6: Deferred backlog is recorded, not silently dropped
`docs/beui-component-map.md` records wave 1/2 (in scope), optional components, and skip decisions with reasons, so future work has a starting point.

## Risks / Trade-offs

- **[Bundle growth from `motion-v`]** → Import only used motion primitives, tree-shake, and keep animated components off static marketing pages per beUI's own guidance. Measure bundle before/after.
- **[Porting drift from upstream beUI]** → Keep class/token usage faithful and record provenance; accept that re-syncing future upstream changes is manual.
- **[Design regression during migration]** → Token layer lands first so before/after is a controlled visual diff; migrate component-by-component with tests.
- **[Accessibility regressions in ported overlays]** → Treat focus trap/restore, Escape, and ARIA as required scenarios with explicit tests; reuse the existing focus-trap logic pattern from `ConfirmModal`.
- **[Vue `motion-v` API differences from React Motion]** → Validate `layoutId` shared-layout behavior in a spike (tabs indicator) before porting the morphing modal.
- **[Dark-mode mismatches]** → Add a dark-mode readability check to the verification tasks for each migrated surface.
- **[Test churn]** → Expect updates to `frontend/src/components/__tests__/*`; keep public behavior stable to minimize changes to view-level tests.

## Migration Plan

1. Land tokens + `cn()` + dependencies; no visible change.
2. Port and test wave 1 primitives in `components/ui/`.
3. Migrate call-sites view-by-view (forms → dashboard chrome → settings), deleting superseded components as each is fully replaced.
4. Port wave 2 feedback/surface components; migrate `ToastHost`, modals, popover, menus, drawer.
5. Verify: `vue-tsc -b`, `vitest run`, Playwright `test:e2e`, dark-mode and reduced-motion passes.
6. Rollback: the change is isolated to the frontend; reverting the frontend diff restores the prior primitives with no backend impact.

## Open Questions

- Should the toast host expose action buttons and swipe-dismiss in the first pass, or is status + dismiss sufficient for current call-sites?
- Do we keep `SegmentedControl` as a thin wrapper over the new Tabs primitive, or fully replace its call-sites?
- Is a `motion-v` shared-layout spike required before committing to the Morphing Modal, or is the simpler Center Morph Modal sufficient?
