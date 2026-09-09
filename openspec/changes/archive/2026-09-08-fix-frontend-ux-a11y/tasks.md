## 1. Mobile navigation

- [x] 1.1 Add hamburger + slide-over drawer (< lg) with nav items
- [x] 1.2 Include workspace switcher + user menu in drawer
- [x] 1.3 Focus/open-close/Escape/focus-restore behavior
- [ ] 1.4 Manual test on mobile widths

## 2. Shared surface + dark mode

- [x] 2.1 Create `Card.vue` with dark variants; migrate ~12 duplicated card blocks
- [x] 2.2 Audit + fix missing `dark:` variants in settings/saved/landing/insights asides
- [x] 2.3 Grep-check `text-zinc-900/800` without dark counterpart
- [ ] 2.4 Visual test in dark + light mode

## 3. Feed correctness

- [x] 3.1 Independent offset ref in `stores/feed.ts`; unstar decrements correctly
- [x] 3.2 `FilterBar` local-date presets (no off-by-one)
- [x] 3.3 Format export expiry with shortDate
- [x] 3.4 Unit tests: pagination across star/unstar/load-more

## 4. Insights drill-down

- [x] 4.1 Insight card → feed filtered by type (+ channel)
- [x] 4.2 `type` query param + setFilter wiring
- [x] 4.3 Test: click insight → filtered feed

## 5. Trial + plan messaging

- [x] 5.1 Show trial countdown + upgrade CTA when trial active
- [x] 5.2 Align Landing/FAQ copy to real plans; remove false "14-day" claim if trials not implemented
- [x] 5.3 Centralize plan names in `lib/plans.ts`

## 6. Onboarding progress

- [x] 6.1 Poll until completed/failed (not ≥10%)
- [x] 6.2 Add "syncing in background" escape hatch → Channels
- [x] 6.3 Test: backfill completes, bar reaches 100%, toast fires

## 7. A11y pass

- [x] 7.1 ConfirmModal: real heading id + aria-labelledby
- [x] 7.2 CommentCard insight pill → button, aria-expanded, 44px target
- [x] 7.3 WorkspaceSwitcher keyboard nav (buttons + arrows + Escape)
- [x] 7.4 Avatar menu focus/Escape/restore
- [x] 7.5 KeyboardHelpModal focus trap + Escape
- [x] 7.6 TextInput/form errors aria-describedby + aria-invalid + role=alert
- [x] 7.7 h1 per view; correct hierarchy
- [x] 7.8 Contrast fixes (zinc-400 → zinc-500+) and 44px touch targets
- [x] 7.9 Tab components arrow-key nav or aria-pressed buttons
- [x] 7.10 prefers-reduced-motion handling
- [x] 7.11 Run axe scan on key views

## 8. Motion + cleanup

- [x] 8.1 Modal/list transitions (Transition, transition-group)
- [x] 8.2 Delete HelloWorld.vue, InlineError.vue, unused useWorkspaceSwitcher
- [x] 8.3 Per-view `<title>`

## 9. Discoverability

- [x] 9.1 Keyboard-shortcut hint chip in feed
- [x] 9.2 Quota upgrade nudge CTA

## 10. Verify

- [x] 10.1 `npm run build` + vitest green; axe clean on dashboard/settings
- [ ] 10.2 Manual: mobile nav, dark mode, drill-down, onboarding completion
