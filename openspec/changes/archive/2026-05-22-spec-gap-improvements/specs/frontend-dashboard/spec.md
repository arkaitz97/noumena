## ADDED Requirements

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
