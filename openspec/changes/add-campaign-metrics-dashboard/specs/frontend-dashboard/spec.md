## ADDED Requirements

### Requirement: Admin route guard

The frontend router SHALL gate the admin dashboard route behind authentication and administrator status, and SHALL redirect non-administrators away from the route.

#### Scenario: Administrator reaches the dashboard

- **WHEN** an authenticated administrator navigates to the admin campaign route
- **THEN** the dashboard view is rendered

#### Scenario: Non-administrator is redirected

- **WHEN** an authenticated non-administrator navigates to the admin campaign route
- **THEN** the user is redirected away without seeing campaign data

### Requirement: Conditional admin navigation

The application navigation SHALL show the campaign dashboard entry only when the authenticated user is an administrator.

#### Scenario: Admin sees the entry

- **WHEN** an administrator is signed in
- **THEN** the campaign dashboard navigation entry is visible

#### Scenario: Non-admin does not see the entry

- **WHEN** a non-administrator is signed in
- **THEN** the campaign dashboard navigation entry is not rendered

### Requirement: Campaign dashboard presentation

The admin campaign view SHALL render the funnel summary, ACC against the framework target, median time-to-first-insight, high-intent rate, the decision bucket, and the attribution breakouts, and SHALL offer a CSV download. The view SHALL present small-sample caveats consistent with the measurement framework.

#### Scenario: Dashboard renders the decision bucket

- **WHEN** the dashboard loads summary data
- **THEN** the GREEN, YELLOW, or RED bucket returned by the server is displayed with its diagnostic signals

#### Scenario: CSV download is available

- **WHEN** an administrator activates the export control
- **THEN** the cohort CSV download is requested

#### Scenario: Caveats are shown for small samples

- **WHEN** the dashboard displays rates for a cohort below the framework's small-sample guidance
- **THEN** a caveat noting that small differences are not statistically meaningful is shown
