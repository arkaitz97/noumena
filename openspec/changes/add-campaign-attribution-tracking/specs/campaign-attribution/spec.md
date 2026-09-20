## ADDED Requirements

### Requirement: Campaign attribution capture

The system SHALL persist first-touch UTM parameters and a self-reported attribution (SRA) source for each authenticated user, linked to the anonymous landing identifier when one is present. User-facing attribution fields SHALL be validated against closed enums before storage.

#### Scenario: UTM parameters are stored for a new user

- **WHEN** an authenticated user sends UTM parameters to the attribution endpoint
- **THEN** the system stores `utm_source`, `utm_medium`, `utm_campaign`, `utm_content`, and `utm_term` on the user's attribution row
- **AND** stores the client-provided `anon_id` when present
- **AND** the response is 200 with the persisted attribution

#### Scenario: Partial update preserves previously stored fields

- **WHEN** a user with an existing attribution row submits only the SRA answer
- **THEN** the system updates `sra_source` and leaves existing UTM fields unchanged

#### Scenario: Invalid UTM or SRA values are rejected

- **WHEN** a request supplies an `sra_source` outside the allowed enum
- **THEN** the system responds 400 and stores nothing

#### Scenario: Attribution requires authentication

- **WHEN** an unauthenticated request is sent to the attribution endpoint
- **THEN** the system responds 401 and stores nothing

### Requirement: Campaign attribution retrieval

The system SHALL expose the authenticated user's own attribution row so the client can determine whether the SRA question has already been answered.

#### Scenario: Existing attribution is returned

- **WHEN** an authenticated user with an attribution row requests their attribution
- **THEN** the system responds 200 with the stored attribution fields

#### Scenario: Missing attribution returns an empty record

- **WHEN** an authenticated user without an attribution row requests their attribution
- **THEN** the system responds 200 with a null/empty attribution payload rather than 404

### Requirement: Per-user product event ingestion

The system SHALL accept a closed allowlist of product events over the authenticated events endpoint and persist them as append-only rows with the user id, event name, validated JSON properties, and a server timestamp. Events outside the allowlist or with property values outside their enum SHALL be rejected without a stored row. The existing OpenTelemetry counter SHALL continue to be incremented for accepted events.

#### Scenario: First insight view is persisted

- **WHEN** an authenticated user posts an `insight_viewed` event with a comment id
- **THEN** the system stores a `product_events` row with event name `insight_viewed` and the comment id in its properties
- **AND** responds 204

#### Scenario: High-intent action is persisted

- **WHEN** an authenticated user posts a `high_intent_action` event whose action is one of `star`, `export`, or `digest_opt_in`
- **THEN** the system stores a `product_events` row with that action in its properties

#### Scenario: Unknown event is rejected

- **WHEN** an authenticated user posts an event name outside the allowlist
- **THEN** the system responds 400 and stores no row

#### Scenario: Unknown event property value is rejected

- **WHEN** an authenticated user posts a known event with a property value outside its enum
- **THEN** the system responds 400 and stores no row

### Requirement: Anonymous landing view beacon

The system SHALL record `landing_view` events from unauthenticated clients through a rate-limited public endpoint. These rows SHALL carry no user id and SHALL store only an opaque client-generated `anon_id` plus validated campaign query attributes.

#### Scenario: Anonymous landing view is recorded

- **WHEN** an unauthenticated client posts a `landing_view` event with an `anon_id`
- **THEN** the system stores a `product_events` row with a null user id and the provided `anon_id`

#### Scenario: Non-landing event is rejected on the public endpoint

- **WHEN** an unauthenticated client posts any event other than `landing_view` to the public endpoint
- **THEN** the system responds 400 and stores no row

### Requirement: Campaign cohort view

The system SHALL provide a read-only SQL view, `v_campaign_cohort`, exposing one row per attributed user with: signup time, first OAuth completion time, first insight view time, seconds from OAuth to first insight, a derived `is_acc` flag (OAuth completed and first insight within 48 hours of OAuth), a high-intent flag derived from existing star/export/digest-opt-in data plus `high_intent_action` events, and the attribution fields (UTM, SRA source, campaign angle).

#### Scenario: Activated channel connection is derived

- **WHEN** a user has an OAuth completion time and a first insight view within 48 hours of it
- **THEN** the view reports `is_acc` true for that user

#### Scenario: Late first insight is not an activation

- **WHEN** a user's first insight view occurs more than 48 hours after OAuth completion
- **THEN** the view reports `is_acc` false for that user

#### Scenario: OAuth without insight is not an activation

- **WHEN** a user has completed OAuth but has no insight view
- **THEN** the view reports `is_acc` false and a null first insight time

#### Scenario: High intent is derived from existing signals

- **WHEN** a user has starred a comment, exported comments, enabled the weekly digest, or emitted a `high_intent_action` event
- **THEN** the view reports the high-intent flag true for that user

### Requirement: Attribution lifecycle compliance

Attribution and product event rows SHALL be deleted when the owning user is deleted, and SHALL be included in the user's GDPR data export. Anonymous product event rows SHALL be removable by a retention sweep.

#### Scenario: User deletion removes campaign data

- **WHEN** a user account is hard-deleted
- **THEN** their `user_attribution` row and all `product_events` rows referencing their user id are deleted

#### Scenario: GDPR export includes campaign data

- **WHEN** a user requests a data export
- **THEN** the export includes their attribution row and their product event rows

#### Scenario: Anonymous events are swept

- **WHEN** an anonymous `landing_view` row is older than the configured retention window
- **THEN** the retention sweep deletes it
