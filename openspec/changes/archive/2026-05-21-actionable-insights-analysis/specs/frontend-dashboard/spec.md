## MODIFIED Requirements

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
- **WHEN** the frontend received a 401 from /auth/refresh
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

## REMOVED Requirements

### Requirement: Sentiment Trend Chart
**Reason**: Sentiment analysis is being replaced by actionable insights extraction.
**Migration**: The sentiment trend chart component is removed. Users should use the actionable insights panel and insight type distribution chart instead.
