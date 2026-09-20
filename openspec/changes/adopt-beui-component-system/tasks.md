## 1. Foundation

- [ ] 1.1 Write `docs/beui-component-map.md` with the beUI inventory, beUI → Vue component mapping, relevance tiers (port / optional / skip + reasons), deferred backlog, and upstream MIT provenance.
- [ ] 1.2 Add `motion-v`, `clsx`, and `tailwind-merge` to `frontend/package.json` dependencies and install.
- [ ] 1.3 Add `frontend/src/lib/cn.ts` exporting `cn()` (clsx + tailwind-merge).
- [ ] 1.4 Add the semantic token layer to `frontend/src/style.css` `@theme` per design D2, including `.dark` overrides and `--radius` if needed by ported components.

## 2. Wave 1 primitives (`frontend/src/components/ui/`)

- [ ] 2.1 Port Button (variants + spring press) to `ui/button/` and render it against semantic tokens.
- [ ] 2.2 Port Input (label, icons, error row, shake) to `ui/input/`; migrate `forms/TextInput.vue`/`PasswordInput.vue`/`InlineError.vue` behavior.
- [ ] 2.3 Port Select to `ui/select/` (listbox, keyboard navigation, portal panel).
- [ ] 2.4 Port Checkbox and Radio Group to `ui/checkbox/` and `ui/radio/` (indeterminate + gliding indicator).
- [ ] 2.5 Port Switch to `ui/switch/` (spring thumb).
- [ ] 2.6 Port Tabs (pill / segment / underline with `layoutId` indicator) to `ui/tabs/`.
- [ ] 2.7 Port Animated Badge to `ui/badge/` (status tones + pulse).
- [ ] 2.8 Port Tooltip to `ui/tooltip/` (hover/focus, blur enter/exit).
- [ ] 2.9 Port Loader to `ui/loader/` and replace inline button spinners and view skeletons.
- [ ] 2.10 Add Vitest coverage for each wave 1 primitive: variants, `cn()` override, keyboard operation, ARIA (`aria-invalid`, `aria-describedby`, `tablist`/`tab`), and reduced-motion.

## 3. Wave 2 feedback and surface components

- [ ] 3.1 Port Animated Toast Stack to `ui/toast/` and adapt `frontend/src/lib/toast.ts` to its store.
- [ ] 3.2 Port Morphing/Center Modal to `ui/modal/`, preserving the existing focus-trap, initial-focus, Escape, and focus-restore behavior from `ConfirmModal.vue`.
- [ ] 3.3 Port Popover to `ui/popover/` for the filter panel and workspace menu.
- [ ] 3.4 Port Animated Context Menu to `ui/context-menu/` for user and workspace menus.
- [ ] 3.5 Port Drawer to `ui/drawer/` for the mobile navigation drawer (body scroll lock, backdrop blur, Escape).
- [ ] 3.6 Add Vitest coverage for wave 2: open/close motion states, focus trap and restore, Escape, menu arrow-key navigation and `menu`/`menuitem` roles.

## 4. Migrate call-sites

- [ ] 4.1 Replace native `<select>` in `AccountView.vue`, `ChannelsView.vue`, `MembersView.vue`, `OnboardingView.vue`, and `InsightsFeed.vue` export dialog with `ui/select/`.
- [ ] 4.2 Replace native checkboxes in `AccountView.vue`, `LoginView.vue`, and `FilterBar.vue` with `ui/checkbox/`.
- [ ] 4.3 Replace `forms/PrimaryButton.vue` and `forms/SecondaryButton.vue` usages with `ui/button/`, then delete the old components.
- [ ] 4.4 Replace `SegmentedControl.vue` and `FeedIntentTabs.vue` with `ui/tabs/`; delete the old components.
- [ ] 4.5 Replace inline status badges (UsageBar, MembersView seats, AccountView verification, ChannelCard, banners) with `ui/badge/`.
- [ ] 4.6 Replace `ToastHost.vue` wiring in `App.vue` with `ui/toast/`; delete `ToastHost.vue`.
- [ ] 4.7 Replace `ConfirmModal.vue`, `KeyboardHelpModal.vue`, and the `InsightsFeed.vue` export dialog with `ui/modal/`; delete the old components.
- [ ] 4.8 Replace `WorkspaceSwitcher.vue` and DashboardLayout user menu with `ui/popover/` + `ui/context-menu/`.
- [ ] 4.9 Replace the FilterBar filter popover and its checkboxes/search with `ui/popover/` + `ui/checkbox/` + `ui/input/`.
- [ ] 4.10 Replace the dark-mode toggle button in `DashboardLayout.vue` (and landing) with `ui/switch/` or a `ui/button/` icon variant.
- [ ] 4.11 Migrate auth views (`LoginView`, `SignupView`, `ForgotPasswordView`, `ResetPasswordView`, `AcceptInviteView`, `OnboardingView`, `VerifyEmailView`) to `ui/input/` + `ui/button/` + `ui/checkbox/`.

## 5. Verification

- [ ] 5.1 Update/remove Vitest specs for deleted components under `frontend/src/components/__tests__/` and fix any view-level tests.
- [ ] 5.2 Run `npm run build` (`vue-tsc -b && vite build`) and resolve type errors.
- [ ] 5.3 Run `npm run test` and ensure the full Vitest suite passes.
- [ ] 5.4 Run `npm run test:e2e` and ensure Playwright passes.
- [ ] 5.5 Manual pass in light and dark mode across dashboard, settings, and auth views; confirm no dark-on-dark text or mismatched surfaces.
- [ ] 5.6 Verify `prefers-reduced-motion: reduce` collapses animation while controls remain functional.
- [ ] 5.7 Verify keyboard-only operation of tabs, menus, modals, selects, and the toast dismiss control.
- [ ] 5.8 Record frontend bundle size before/after and confirm no static marketing page regressions.

## 6. Cleanup

- [ ] 6.1 Delete all superseded hand-rolled primitives and confirm no dangling imports.
- [ ] 6.2 Add MIT attribution/comment header noting beUI origin to ported `ui/` components.
- [ ] 6.3 Update `frontend/STYLEGUIDE.md` to point at the semantic token layer and `ui/` primitives.
