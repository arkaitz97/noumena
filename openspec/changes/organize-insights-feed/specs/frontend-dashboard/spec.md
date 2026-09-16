## ADDED Requirements

### Requirement: Intent-Based Feed Tabs
The system SHALL provide top-level workflow tabs above the comment feed to enable rapid switching between common triage modes without opening the filter drawer.

#### Scenario: Switching to Actionable Insights Tab
- **GIVEN** an authenticated user on /dashboard
- **WHEN** the user selects the "Actionable" intent tab
- **THEN** the frontend SHALL filter the feed to comments containing actionable insights (`insight_type != null` or high-confidence insights) and synchronize the selection with URL query parameters

#### Scenario: Switching to Questions Tab
- **GIVEN** an authenticated user on /dashboard
- **WHEN** the user selects the "Questions" intent tab
- **THEN** the frontend SHALL filter the feed to comments categorized as `question` and update the active tab indicator

#### Scenario: Switching to Ideas Tab
- **GIVEN** an authenticated user on /dashboard
- **WHEN** the user selects the "Ideas" intent tab
- **THEN** the frontend SHALL filter the feed to comments categorized as `content_idea` or carrying `video_idea` insights

#### Scenario: Switching to All Comments Tab
- **GIVEN** an authenticated user with a specific intent tab active
- **WHEN** the user selects the "All" tab
- **THEN** the frontend SHALL clear the intent filter and restore the default feed stream

### Requirement: Group-By-Video Organization Mode
The system SHALL support organizing comments grouped by video, collapsing repetitive video information and aggregating signal counts per video.

#### Scenario: Toggling Video Grouping
- **GIVEN** an authenticated user viewing comments on /dashboard
- **WHEN** the user toggles the view mode from "Stream" to "Group by Video"
- **THEN** the frontend SHALL group comments under collapsible video headers showing video title, total comments in range, count of actionable insights, and link to YouTube

#### Scenario: Expanding and Collapsing Video Sections
- **GIVEN** a list of comments grouped by video
- **WHEN** the user clicks a video group header
- **THEN** the frontend SHALL toggle visibility of the comments belonging to that specific video

## MODIFIED Requirements

### Requirement: Comment Feed UI
The system SHALL display a real-time feed of high-signal comments with filtering, prominent actionable insight callouts, and clean metadata hierarchy.

#### Scenario: Feed Display
- **GIVEN** an authenticated user with processed comments
- **WHEN** the user views the dashboard
- **THEN** the frontend SHALL fetch GET /api/comments/feed, display comments with visual category accents, prominent actionable insight callouts, topic labels, and channel/video provenance, and support infinite scroll

#### Scenario: Actionable Insight Surface on Comment Cards
- **GIVEN** a comment in the feed with one or more actionable insights
- **WHEN** the comment card is rendered
- **THEN** the frontend SHALL display the primary actionable insight prominently in a top callout banner with its type, confidence percentage, and description, without requiring the user to expand an accordion

#### Scenario: Feed Filtering
- **GIVEN** a user viewing the feed
- **WHEN** the user applies filters (channel, category, date range, insight_type, search, or intent tab)
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
The system SHALL render actionable insights panels, toxicity metrics, and interactive topic and insight distributions that allow cross-filtering.

#### Scenario: Actionable Insights Panel
- **GIVEN** a user with >= 10 processed comments for a channel
- **WHEN** the user views the insights dashboard
- **THEN** the frontend SHALL fetch GET /api/insights/actionable-insights/top and render panels grouped by insight type (top video ideas, top editing tips, etc.) with confidence indicators

#### Scenario: Interactive Topic Cross-Filtering
- **GIVEN** an authenticated user viewing the trends widget with top topics
- **WHEN** the user clicks any topic chip
- **THEN** the frontend SHALL apply that topic as a search/filter on the comment feed and update the feed results

#### Scenario: Interactive Distribution Cross-Filtering
- **GIVEN** an authenticated user viewing the insight type distribution chart
- **WHEN** the user clicks an insight type bar or label
- **THEN** the frontend SHALL apply that insight type filter to the comment feed

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
