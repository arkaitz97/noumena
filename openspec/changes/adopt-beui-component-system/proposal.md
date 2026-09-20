## Why

The Noumena dashboard's visual language is serviceable but flat: hand-rolled primitives (`PrimaryButton`, `TextInput`, `SegmentedControl`, `ConfirmModal`, `ToastHost`) carry duplicated Tailwind, inconsistent focus/error states, and only generic CSS transitions. The product needs a cohesive, premium, motion-aware component layer. beUI (beui.dev) provides exactly that interaction quality — spring-pressed buttons, morphing modals, animated tabs, swipe-dismiss toasts — but it ships as React/Next.js + Motion (framer-motion) source. We will adopt its design language by porting the relevant components to Vue via `motion-v` and adding a token layer so beUI's shadcn-semantic classes resolve against our existing zinc + brand palette.

## What Changes

- Add a **design token layer** in `frontend/src/style.css`: shadcn-compatible semantic CSS variables (`--color-card`, `--color-border`, `--color-primary`, `--color-muted-foreground`, `--color-ring`, etc.) mapped to the existing `zinc-*` and `brand-*` ramps, with dark-mode overrides via the existing `dark` class variant.
- Add a **Vue component library** under `frontend/src/components/ui/`, ported from beUI source (MIT) with `motion-v` for animation and `lucide-vue-next` for icons: Button, Input, Select, Checkbox, Switch, Radio Group, Tabs, Animated Badge, Tooltip, Loader (wave 1); Animated Toast Stack, Morphing/Center Modal, Popover, Animated Context Menu, Drawer (wave 2).
- Introduce a shared `cn()` class-merge utility (`clsx` + `tailwind-merge`).
- **Migrate existing call-sites** to the new primitives: replace native `<select>` (6 sites), native checkboxes, dark-mode toggle button, inline status badges, the filter popover, workspace/user menus, `SegmentedControl`, `FeedIntentTabs`, `ToastHost`, `ConfirmModal`, `KeyboardHelpModal`, and the export dialog.
- Remove superseded hand-rolled primitives (`forms/PrimaryButton.vue`, `forms/SecondaryButton.vue`, `forms/TextInput.vue`, `SegmentedControl.vue`, `ToastHost.vue`, `ConfirmModal.vue`, `KeyboardHelpModal.vue`) after migration.
- **Non-goals (deferred):** Animated Sidebar, view-transition Theme Toggle, Number/Text Animation, OTP input, 404 pages, Table, Bottom Sheet, landing shader/marquee/carousel, and all beUI AI-agent components.
- **BREAKING (internal only):** component prop/API surface of the replaced primitives changes; all in-repo call-sites are migrated in the same change. No backend or public API changes.

## Capabilities

### New Capabilities
- `frontend-design-system`: The shared design token contract and the ported Vue component library — token-to-palette mapping, theming/dark-mode compatibility, motion behavior, reduced-motion handling, and component accessibility (focus, keyboard, ARIA).

### Modified Capabilities
- `frontend-dashboard`: Add a requirement that interactive controls (buttons, inputs, selects, checkboxes, switches, tabs, badges, tooltips, toasts, modals, menus) are rendered from the shared design-system primitives and preserve existing keyboard/screen-reader and dark-mode-readability guarantees.

## Impact

- **New frontend dependencies:** `motion-v`, `clsx`, `tailwind-merge` (`frontend/package.json`).
- **New files:** `frontend/src/lib/cn.ts`; `frontend/src/components/ui/**` (ported components).
- **Modified files:**
  - `frontend/src/style.css` (semantic token layer).
  - `frontend/src/App.vue` (toast host wiring), `frontend/src/lib/toast.ts`.
  - `frontend/src/components/forms/*`, `SegmentedControl.vue`, `FeedIntentTabs.vue`, `ToastHost.vue`, `ConfirmModal.vue`, `KeyboardHelpModal.vue`, `WorkspaceSwitcher.vue`, `FilterBar.vue`, `UsageBar.vue`, `RateLimitBanner.vue`, `TokenExpiryBanner.vue`, `ChannelCard.vue`, `CommentCard.vue`, `PlanComparison.vue`.
  - `frontend/src/views/dashboard/DashboardLayout.vue`, `InsightsFeed.vue`, `SavedCommentsView.vue`.
  - `frontend/src/views/settings/AccountView.vue`, `BillingView.vue`, `ChannelsView.vue`, `MembersView.vue`.
  - `frontend/src/views/auth/*` (inputs, checkboxes, OAuth/verification surfaces).
- **Tests:** existing Vitest specs for replaced components must be updated; `vue-tsc`, `vitest run`, and Playwright e2e (`frontend/e2e`) must pass.
- **Docs:** `docs/beui-component-map.md` records the beUI → Vue mapping, relevance tiers, and deferred backlog.
- **No impact:** Go backend, database schema, API contracts, billing, ingestion.
