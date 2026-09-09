## MODIFIED Requirements

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
