## ADDED Requirements

### Requirement: Draft reply button on comment cards

The comment card SHALL render a "Draft reply" action (icon + tooltip "coming soon") in the feed and saved views when the painted-door flag is enabled. Clicking it SHALL POST a reply-draft-click event for the comment and SHALL show a toast stating the feature is coming and interest is being measured. The button SHALL be keyboard-reachable and have an aria-label.

#### Scenario: Click emits event and toast
- **WHEN** a user clicks "Draft reply" on a comment card
- **THEN** the app posts the click event and shows the "coming soon" toast without navigating

#### Scenario: Flag disabled
- **WHEN** the painted-door flag is disabled
- **THEN** the button is not rendered