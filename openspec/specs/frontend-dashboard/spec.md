# Frontend Dashboard Specification

## Purpose
Provides the Vue 3 SPA for user-facing features: authentication, onboarding, channel management, comment feed, insights visualization, billing, and account settings.

## Requirements

### Requirement: Authentication UI
The system SHALL provide login, registration, and password reset interfaces.

#### Scenario: Registration Form
- **GIVEN** a visitor on /signup
- **WHEN** the user enters email and password meeting policy (>=8 chars, >=1 uppercase, >=1 digit)
- **THEN** the frontend SHALL validate in real-time, submit to POST /auth/register, store the access token in memory (Pinia), and redirect to /verify-email holding page

#### Scenario: Login Form
- **GIVEN** a visitor on /login
- **WHEN** the user enters email and password
- **THEN** the frontend SHALL submit to POST /auth/login, store the access token in memory, rely on HttpOnly cookie for refresh token, and redirect to /dashboard

#### Scenario: Google SSO Button
- **GIVEN** a visitor on /signup or /login
- **WHEN** the user clicks "Continue with Google"
- **THEN** the frontend SHALL open a popup, call GET /auth/google/url to get the consent URL + state, and handle the callback via POST /auth/google/callback with code and state

#### Scenario: Session Expired Redirect
- **GIVEN** a logged-in user whose access token has expired and refresh token is also invalid
- **WHEN** the frontend receives a 401 from /auth/refresh
- **THEN** the frontend SHALL clear session state and redirect to /login with "Your session has expired. Please log in again."

### Requirement: Email Verification Gate
The system SHALL restrict product access to verified users.

#### Scenario: Unverified User Redirect
- **GIVEN** a logged-in user with email_verified=false
- **WHEN** the user navigates to any product route
- **THEN** the frontend SHALL redirect to /verify-email with a "Resend verification" link

### Requirement: Onboarding Wizard
The system SHALL guide new users through their first channel connection.

#### Scenario: First-Time Onboarding
- **GIVEN** a newly registered user (via Google SSO, auto-verified)
- **WHEN** the user lands on /onboarding
- **THEN** the frontend SHALL display the channel connection wizard, initiate YouTube OAuth flow, and show "Syncing your channel... this may take up to 24 hours" after connection

#### Scenario: Empty Channel State
- **GIVEN** a user with no connected channels
- **WHEN** the user views the dashboard
- **THEN** the frontend SHALL display an empty state prompting channel connection

### Requirement: Comment Feed UI
The system SHALL display a real-time feed of high-signal comments with filtering.

#### Scenario: Feed Display
- **GIVEN** an authenticated user with processed comments
- **WHEN** the user views the dashboard
- **THEN** the frontend SHALL fetch GET /api/comments/feed, display comments with category badges, actionable insights (if any), and topic labels, and support infinite scroll

#### Scenario: Feed Filtering
- **GIVEN** a user viewing the feed
- **WHEN** the user applies filters (channel, category, date range, insight_type)
- **THEN** the frontend SHALL combine all filters, request the feed, and update results within 2 seconds (p95)

#### Scenario: Star a Comment
- **GIVEN** a user viewing a comment in the feed
- **WHEN** the user clicks the star icon
- **THEN** the frontend SHALL call POST /api/comments/{id}/star and update the UI to show the starred state

#### Scenario: Saved View
- **GIVEN** a user with starred comments
- **WHEN** the user navigates to the "Saved" view
- **THEN** the frontend SHALL fetch GET /api/comments/saved and display starred comments sorted by most recently starred

### Requirement: Insights Visualization
The system SHALL render actionable insights panels, toxicity metrics, and topic distributions.

#### Scenario: Actionable Insights Panel
- **GIVEN** a user with >= 10 processed comments for a channel
- **WHEN** the user views the insights dashboard
- **THEN** the frontend SHALL fetch GET /api/insights/actionable-insights/top and render panels grouped by insight type (top video ideas, top editing tips, etc.) with confidence indicators

#### Scenario: Toxicity Shield Display
- **GIVEN** a user viewing the insights dashboard
- **WHEN** the frontend loads
- **THEN** the system SHALL fetch GET /api/insights/toxicity-shield and display the % of comments filtered as toxic or spam

#### Scenario: Trends Display
- **GIVEN** a user viewing the insights dashboard
- **WHEN** the frontend loads
- **THEN** the system SHALL fetch GET /api/insights/trends and display top topics and category distributions

#### Scenario: Insight Type Distribution Chart
- **GIVEN** a user viewing the insights dashboard
- **WHEN** the frontend loads
- **THEN** the system SHALL fetch GET /api/insights/actionable-insights/distribution and render a bar or pie chart showing the frequency of each insight type

### Requirement: Export UI
The system SHALL allow users to export filtered comment data.

#### Scenario: Small Export (Sync)
- **GIVEN** a user with a filtered comment set under the sync threshold
- **WHEN** the user clicks "Export"
- **THEN** the frontend SHALL call POST /api/comments/export, receive the download_url immediately, and trigger a file download

#### Scenario: Large Export (Async)
- **GIVEN** a user with a filtered comment set exceeding the sync threshold
- **WHEN** the user clicks "Export"
- **THEN** the frontend SHALL call POST /api/comments/export, receive job_id with status=queued, poll GET /api/comments/export/{job_id}/status, and notify the user when completed with a download link

### Requirement: Billing UI
The system SHALL display subscription status and allow plan upgrades.

#### Scenario: Billing Status Display
- **GIVEN** an authenticated user
- **WHEN** the user views the billing page
- **THEN** the frontend SHALL fetch GET /api/billing/status and display plan, status, channels used/limit, comments used/limit, and overage cost

#### Scenario: Upgrade Flow
- **GIVEN** a user on Free plan hitting their limit
- **WHEN** the user clicks "Upgrade"
- **THEN** the frontend SHALL call POST /api/billing/checkout, open Stripe Checkout in a new window, and redirect to dashboard on return with a success banner

#### Scenario: Customer Portal
- **GIVEN** a subscribed user
- **WHEN** the user clicks "Manage Subscription"
- **THEN** the frontend SHALL call GET /api/billing/portal and open the Stripe Customer Portal

### Requirement: Account Settings
The system SHALL provide account management including deletion and data export.

#### Scenario: Request Data Export
- **GIVEN** an authenticated user
- **WHEN** the user clicks "Export My Data" in account settings
- **THEN** the frontend SHALL call POST /api/user/data-export, display the job status, and show a download link when completed

#### Scenario: Request Account Deletion
- **GIVEN** an authenticated user in account settings
- **WHEN** the user clicks "Delete Account", confirms via two-step modal (warning + email typing)
- **THEN** the frontend SHALL call DELETE /api/user/account with email_confirmation, display the reversible_until timestamp, and sign the user out

#### Scenario: Cancel Pending Deletion
- **GIVEN** a user who requested deletion within the reversible window
- **WHEN** the user contacts support and the ops team runs the cancel_deletion CLI
- **THEN** the user SHALL be able to log in again (deletion_requested_at cleared)

### Requirement: Onboarding Progress Indicator
The system SHALL display a progress indicator during the onboarding channel sync process.

#### Scenario: Show Sync Progress
- **GIVEN** a user who just connected a channel
- **WHEN** the user views the onboarding or dashboard page
- **THEN** the frontend SHALL poll GET /api/channels/{id}/sync-status every 10 seconds and display a progress bar with backfill_progress_percent

#### Scenario: Sync Complete Notification
- **GIVEN** a user waiting for channel sync to complete
- **WHEN** backfill_progress reaches 100% or the sync completes
- **THEN** the frontend SHALL show a "Your channel is ready!" notification and redirect to the insights feed

### Requirement: Keyboard Shortcuts
The system SHALL support keyboard shortcuts for power users.

#### Scenario: Star Comment with Shortcut
- **WHEN** the user focuses a comment card and presses 's'
- **THEN** the frontend SHALL toggle the star state for that comment

#### Scenario: Navigate Feed with Shortcuts
- **WHEN** the user presses 'j' or 'k' while viewing the feed
- **THEN** the frontend SHALL move focus to the next or previous comment card

#### Scenario: Open Search with Shortcut
- **WHEN** the user presses '/' while on any dashboard page
- **THEN** the frontend SHALL focus the global search input

#### Scenario: Keyboard Shortcut Help
- **WHEN** the user presses '?'
- **THEN** the frontend SHALL display a modal listing all available keyboard shortcuts

### Requirement: Dark Mode
The system SHALL support a dark mode theme toggle.

#### Scenario: Toggle Dark Mode
- **WHEN** the user clicks the dark mode toggle in the header
- **THEN** the frontend SHALL switch between light and dark themes and persist the preference in localStorage

#### Scenario: System Preference Detection
- **WHEN** the user first visits the application
- **THEN** the frontend SHALL detect prefers-color-scheme media query and apply the matching theme

#### Scenario: Dark Mode Persistence
- **GIVEN** a user who selected dark mode
- **WHEN** the user returns to the application
- **THEN** the frontend SHALL restore the dark theme from localStorage

### Requirement: Role-based UI reflects server role
The system SHALL fetch and apply the user's workspace role so role-gated UI (export, member management) matches server enforcement.

#### Scenario: Analyst loads dashboard
- **GIVEN** a user with the `analyst` role
- **WHEN** the dashboard loads
- **THEN** the export control is enabled

#### Scenario: Viewer loads dashboard
- **GIVEN** a user with the `viewer` role
- **WHEN** the dashboard loads
- **THEN** the export control is disabled/hidden

### Requirement: Email change flow
The system SHALL let a user change their account email through `PATCH /user/me`, keeping the old email until the new one is verified.

#### Scenario: Email change requested
- **GIVEN** an authenticated user
- **WHEN** they submit a new email
- **THEN** a verification email is sent, the account stays on the old email, and the new email activates only after verification

### Requirement: Channel reauthorization
The system SHALL let a user re-link an expired or revoked YouTube token to an existing channel without creating a duplicate.

#### Scenario: Expired token re-linked
- **GIVEN** a channel whose OAuth token expired
- **WHEN** the user reauthorizes the channel
- **THEN** the new token is stored on the existing channel tracking row and the expiry banner clears

### Requirement: Export result surfaced
The system SHALL show the user the result of an async export (status, download link, expiry).

#### Scenario: Export completes
- **GIVEN** a requested data export
- **WHEN** the export completes
- **THEN** the UI shows a working download link and expiry, including for partial exports

### Requirement: Mobile navigation
The system SHALL provide navigation to all primary sections (feed, saved, channels, billing, account) and workspace switching on mobile viewports.

#### Scenario: Mobile user navigates
- **GIVEN** a viewport below the desktop breakpoint
- **WHEN** the user opens the navigation drawer
- **THEN** all primary sections and the workspace switcher are reachable and focusable

### Requirement: Keyboard and screen-reader access
The system SHALL make interactive components (menus, modals, tabs, insight controls, forms) operable by keyboard and announced correctly by screen readers.

#### Scenario: Keyboard-only user
- **GIVEN** a focusable menu or modal
- **WHEN** the user uses Tab, arrow keys, and Escape
- **THEN** items are reachable, Escape closes, and focus is managed (trapped in modal, restored on close)

#### Scenario: Screen-reader user submits a form
- **GIVEN** a form with a validation error
- **WHEN** the error appears
- **THEN** the field is marked invalid and the error is announced and linked to the field

### Requirement: Dark mode readability
The system SHALL render all views readably in dark mode with no dark-on-dark text or mismatched surfaces.

#### Scenario: Dark mode on settings pages
- **GIVEN** dark mode enabled
- **WHEN** the user opens settings, saved, or landing views
- **THEN** text and surfaces meet contrast requirements and are readable

### Requirement: Feed pagination correctness
The system SHALL keep feed pagination stable across optimistic star/unstar operations.

#### Scenario: Unstar in saved view
- **GIVEN** the saved-comments feed with loaded pages
- **WHEN** a comment is unstarred optimistically
- **THEN** subsequent pages do not skip or duplicate rows

### Requirement: Insight drill-down
The system SHALL let a user navigate from an actionable insight to the underlying comments.

#### Scenario: Insight clicked
- **GIVEN** an actionable insight card
- **WHEN** the user clicks it
- **THEN** the feed opens filtered by that insight type (and channel when applicable)

### Requirement: Password change from account settings
The system SHALL let a signed-in user change their password from the account settings.

#### Scenario: Correct current password
- **GIVEN** an authenticated user
- **WHEN** they change their password with the correct current password
- **THEN** the password updates, other sessions are invalidated, and the current session remains valid

#### Scenario: Incorrect current password
- **GIVEN** an authenticated user
- **WHEN** they submit an incorrect current password
- **THEN** the change is rejected and no session is invalidated

### Requirement: Token-expiry in-app banner
The system SHALL show an in-app banner when any channel's OAuth token is expiring within the warning window.

#### Scenario: Token near expiry
- **GIVEN** a channel whose token expires within the warning window
- **WHEN** the user loads the dashboard
- **THEN** a banner identifies the channel and offers a reauthorize action

### Requirement: Canonical feed filter state
The frontend SHALL represent feed filters, intent, date range, channel selection, search, and view state as a canonical serializable scope and SHALL keep that scope synchronized with the active route.

#### Scenario: URL hydration replaces stale state
- **GIVEN** the store contains filters from a previous dashboard visit
- **WHEN** the route changes to a URL with omitted filter parameters
- **THEN** omitted parameters SHALL be reset to their documented defaults
- **AND** stale intent and date-range state SHALL not remain active

#### Scenario: Feed and Saved routes are isolated
- **GIVEN** a user applies filters on Insights Feed
- **WHEN** the user opens Saved comments
- **THEN** Saved SHALL use its own default scope and SHALL not inherit hidden feed filters

#### Scenario: Empty category selection
- **GIVEN** the user turns off every category
- **WHEN** the feed request is serialized
- **THEN** the request SHALL preserve an explicit empty category selection
- **AND** the UI SHALL not silently restore default categories

### Requirement: Intent tabs
The frontend intent tabs SHALL use the API intent contract and SHALL update active state and URL together.

#### Scenario: Ideas tab
- **GIVEN** the user selects Ideas
- **WHEN** the feed refreshes
- **THEN** it SHALL request Ideas `OR` semantics and show Ideas as active

#### Scenario: Reset to All
- **GIVEN** a specific intent is active
- **WHEN** the user selects All or resets filters
- **THEN** the intent predicate, active intent indicator, and URL intent parameter SHALL all be cleared

### Requirement: Widget scope synchronization
Dashboard widgets SHALL use the same selected channels, date range, and relevant filters as the visible feed.

#### Scenario: Multiple selected channels
- **GIVEN** the user selects two or more channels
- **WHEN** widgets load
- **THEN** their values SHALL aggregate all selected channels rather than only the first channel

#### Scenario: Custom or all-time range
- **GIVEN** the user changes the feed to a custom or all-time range
- **WHEN** widgets reload
- **THEN** they SHALL use the new range and SHALL not use the previous range's values

### Requirement: Saved comments presentation
The Saved view SHALL display all starred comments by default and SHALL make any additional filtering explicit.

#### Scenario: Saved low-signal comment
- **GIVEN** a starred comment would be excluded by the ordinary feed's category or toxicity defaults
- **WHEN** the user views Saved without additional filters
- **THEN** the comment SHALL still be displayed

### Requirement: Cost display in billing views

The billing page SHALL show a month-to-date LLM cost figure in the plan card and a per-period cost column in the usage history table, using the fields from `GET /api/billing/status` and `GET /api/billing/usage`. When cost fields are absent (older backend), the UI SHALL render the existing view without the cost column.

#### Scenario: Usage table with cost column
- **WHEN** a user opens the billing page with cost fields present
- **THEN** the usage history table shows a cost column and the plan card shows month-to-date spend

#### Scenario: Backend without cost fields
- **WHEN** the usage response lacks cost fields
- **THEN** the table renders without the cost column and no error is shown
