## ADDED Requirements

### Requirement: Painted-door reply-draft click capture

The system SHALL expose `POST /api/events/reply-draft-click` accepting an authenticated user's `{comment_id}` (and the comment's channel/category derived server-side). The endpoint SHALL record the click and return 202. A user SHALL be able to register at most one click per comment; further clicks SHALL increment a counter on the same row. The endpoint SHALL be rate-limited like other authenticated endpoints.

#### Scenario: First click on a comment
- **WHEN** an authenticated user posts a click for a comment they can access
- **THEN** the system records the event and returns 202

#### Scenario: Repeat click on the same comment
- **WHEN** the same user posts a click for the same comment again
- **THEN** the system increments the existing counter and does not create a duplicate row

#### Scenario: Comment not accessible
- **WHEN** the comment belongs to a channel the user does not track or have a grant for
- **THEN** the system returns 403 and records nothing

#### Scenario: Feature flag off
- **WHEN** the painted-door flag is disabled
- **THEN** the endpoint returns 503 and the UI hides the button

### Requirement: Demand measurement metric

The system SHALL increment a counter `noumena_reply_draft_clicks_total` on every accepted click, labelled by comment category and channel, and SHALL expose weekly-active-user click-rate data for the Go/No-Go review.

#### Scenario: Click recorded in metrics
- **WHEN** a click event is accepted
- **THEN** the counter increments with category and channel labels