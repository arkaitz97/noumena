## ADDED Requirements

### Requirement: Self-reported attribution onboarding step

The onboarding wizard SHALL present a single-question self-reported attribution step after the profile step and before the channel-connect step. The step SHALL offer the options `dm_audit`, `public_teardown`, `community`, `friend`, and `other`, SHALL require a selection to advance, and SHALL submit the selection to the attribution endpoint. When the user selects `other`, the step SHALL allow an optional length-capped free-text note.

#### Scenario: Survey appears before channel connect

- **WHEN** a newly verified user with no connected channels enters onboarding
- **THEN** the self-reported attribution step is shown after the profile step and before the channel-connect step

#### Scenario: Selection is submitted

- **WHEN** the user selects a source and continues
- **THEN** the client submits the selected source to the attribution endpoint
- **AND** the wizard advances to the channel-connect step

#### Scenario: Step cannot be skipped without a selection

- **WHEN** the user attempts to continue without selecting a source
- **THEN** the wizard does not advance

#### Scenario: Other allows a note

- **WHEN** the user selects `other`
- **THEN** an optional free-text input is shown and its value is submitted with the attribution

### Requirement: First-touch campaign parameter capture

The frontend SHALL capture campaign query parameters and the landing referrer on first arrival, persist them locally on a first-touch-wins basis, and generate and persist an opaque anonymous identifier. Captured parameters SHALL be sent to the attribution endpoint after authentication without blocking the authentication flow, at most once per authenticated user.

#### Scenario: Parameters are captured on landing

- **WHEN** a user first lands on the application with UTM query parameters
- **THEN** the client persists the parameters and the referrer on the device

#### Scenario: First touch is preserved

- **WHEN** the client subsequently lands with different UTM parameters and a stored first-touch value exists
- **THEN** the stored first-touch value is not overwritten

#### Scenario: Parameters sync after authentication

- **WHEN** an authenticated session is established and unsent attribution exists for that user
- **THEN** the client posts the attribution to the server once

#### Scenario: Failure to sync does not block the user

- **WHEN** the attribution post fails
- **THEN** the authentication flow and subsequent navigation proceed normally

### Requirement: Insight view instrumentation

The insights feed SHALL report an `insight_viewed` product event for an insight card when that card becomes visible in the viewport, at most once per comment per browser session.

#### Scenario: Visible insight card is reported

- **WHEN** an insight card scrolls into the viewport
- **THEN** the client sends one `insight_viewed` event for that comment

#### Scenario: Duplicate views are suppressed

- **WHEN** the same insight card is viewed again in the same session
- **THEN** no additional `insight_viewed` event is sent for that comment

### Requirement: High-intent action instrumentation

The frontend SHALL report a `high_intent_action` event when the user stars a comment, requests a comment export, or enables the weekly digest, in addition to the persisted high-intent records the backend already writes.

#### Scenario: Starring reports high intent

- **WHEN** the user stars a comment
- **THEN** the client sends a `high_intent_action` event with action `star`

#### Scenario: Exporting reports high intent

- **WHEN** the user submits a comment export
- **THEN** the client sends a `high_intent_action` event with action `export`

#### Scenario: Digest opt-in reports high intent

- **WHEN** the user enables the weekly digest
- **THEN** the client sends a `high_intent_action` event with action `digest_opt_in`

### Requirement: Events client allowlist

The frontend events service SHALL accept only the event names and property keys permitted by the server's per-event schema, so that client misuse is caught at build time.

#### Scenario: Unsupported event name is a type error

- **WHEN** a developer calls the events service with an event name not in the allowlist
- **THEN** the TypeScript compiler rejects the call
