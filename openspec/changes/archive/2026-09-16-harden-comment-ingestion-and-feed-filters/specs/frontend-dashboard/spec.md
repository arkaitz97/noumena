## MODIFIED Requirements

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
