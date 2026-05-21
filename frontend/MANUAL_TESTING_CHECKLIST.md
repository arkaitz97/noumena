# Manual Testing Checklist — Frontend Dashboard Improvements

## 13.1 Onboarding progress indicator with sync-status polling (10s interval)

- [ ] Navigate to `/onboarding` as a new user
- [ ] Connect a YouTube channel and proceed to step 3 (Backfill)
- [ ] Verify the progress bar updates as sync progresses
- [ ] Open browser DevTools Network tab and confirm sync-status requests fire at ~10s intervals (not 5s)
- [ ] Verify polling pauses when tab is not visible (optional)

## 13.2 Sync complete notification and redirect to insights feed

- [ ] Complete onboarding until sync reaches 100%
- [ ] Verify a success toast notification appears: "Sync complete! Your insights are ready."
- [ ] Verify automatic redirect to `/dashboard` occurs ~1.2s after notification
- [ ] Verify no redirect occurs if sync is below 100%

## 13.3 Keyboard shortcuts

- [ ] Navigate to `/dashboard`
- [ ] Press `s` — verify the currently focused comment toggles star state
- [ ] Press `j` — verify focus moves to the next comment (ring highlight appears)
- [ ] Press `k` — verify focus moves to the previous comment
- [ ] Press `/` — verify focus moves to the search input in the FilterBar
- [ ] Type inside search input, then press `s`, `j`, `k` — verify shortcuts are NOT triggered while input is focused
- [ ] Press `?` — verify the keyboard help modal opens

## 13.4 Keyboard shortcut help modal

- [ ] Press `?` on the dashboard — verify modal opens with all shortcuts listed
- [ ] Verify modal shows: `s` (star), `j` (navigate down), `k` (navigate up), `/` (search), `?` (toggle help)
- [ ] Click outside the modal or press the X button — verify modal closes
- [ ] Press `?` again while modal is open — verify modal closes (toggle behavior)
- [ ] Verify shortcuts are disabled while the modal is open

## 13.5 Dark mode toggle with localStorage persistence

- [ ] Navigate to any dashboard page
- [ ] Click the moon icon in the header — verify dark mode activates
- [ ] Verify the icon changes to a sun icon
- [ ] Verify `document.documentElement` has the `dark` class
- [ ] Refresh the page — verify dark mode persists
- [ ] Check localStorage key `noumena-dark-mode` — verify it is set to `"true"`
- [ ] Click the sun icon — verify light mode returns
- [ ] Verify localStorage key `noumena-dark-mode` is set to `"false"`

## 13.6 System preference detection (prefers-color-scheme)

- [ ] Clear localStorage key `noumena-dark-mode`
- [ ] Set OS preference to dark mode
- [ ] Refresh the page — verify dark mode activates automatically
- [ ] Set OS preference to light mode
- [ ] Refresh the page — verify light mode activates automatically
- [ ] While the page is open, change OS preference — verify the UI updates without refresh
- [ ] Manually toggle dark mode — verify system preference is overridden (localStorage set)
- [ ] Clear localStorage — verify system preference takes over again

## 13.7 Rate limit error handling UI (429 banner with countdown)

- [ ] Trigger a 429 response (e.g., rapid API calls or backend rate limit test)
- [ ] Verify the amber banner appears at the top of the dashboard
- [ ] Verify the banner shows a countdown timer (e.g., "Retry available in 30s")
- [ ] Verify the countdown decrements every second
- [ ] Verify the banner auto-dismisses when countdown reaches 0
- [ ] Click the X button — verify banner dismisses immediately
- [ ] Verify the banner does not reappear after dismissal unless a new 429 occurs

## 13.8 Workspace switcher UI in header

- [ ] Navigate to the dashboard
- [ ] Verify the workspace switcher dropdown appears in the header (left side)
- [ ] Click the switcher — verify a dropdown lists all user's workspaces
- [ ] Verify the current active workspace is marked with a checkmark
- [ ] Click a different workspace — verify a success toast appears
- [ ] Verify the header updates to show the new workspace name
- [ ] Verify the checkmark moves to the newly selected workspace
- [ ] Verify role labels (admin/analyst/viewer) are shown next to each workspace

## 13.9 Role-based UI element visibility

- [ ] Log in as an Admin user
- [ ] Verify "Manage members" option appears in the user menu dropdown
- [ ] Verify the Export button is visible in the FilterBar
- [ ] Log in as an Analyst user
- [ ] Verify "Manage members" option is NOT visible in the user menu
- [ ] Verify the Export button IS visible in the FilterBar
- [ ] Log in as a Viewer user
- [ ] Verify "Manage members" option is NOT visible
- [ ] Verify the Export button is NOT visible

## 13.10 Integration testing

- [ ] Verify dark mode works correctly with all new components (workspace switcher, rate limit banner, keyboard help modal)
- [ ] Verify keyboard shortcuts work while workspace switcher dropdown is closed
- [ ] Verify keyboard shortcuts are disabled when workspace switcher dropdown is open
- [ ] Verify the onboarding flow works end-to-end with the new 10s polling interval
- [ ] Verify no console errors appear during any of the above tests
- [ ] Verify all new components are responsive on mobile viewports
