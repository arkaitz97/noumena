## Why

The Noumena 14-day zero-budget guerrilla campaign needs to prove whether targeted creator outreach converts into real product value, measured by Activated Channel Connections (ACC): a creator who signs up, connects a YouTube channel, and views at least one AI-generated insight within 48 hours. Today the product has no way to attribute a signup to a campaign touchpoint, no durable record of the first insight view, and the only UI event sink (`POST /api/events`) emits OpenTelemetry counters without persisting anything. Without per-user attribution and event data, ACC, time-to-first-insight (TTFI), funnel drop-off, and campaign-angle comparisons cannot be computed.

## What Changes

- Capture and persist campaign attribution: UTM parameters (first-touch) and a mandatory one-click self-reported attribution (SRA) survey answer, so mobile-to-desktop journeys that break cookie/UTM tracking are still attributed.
- Add durable, per-user product event persistence for the small set of signals the campaign needs: `insight_viewed`, `high_intent_action`, and anonymous `landing_view` (with an `anon_id` linking a visit to a later signup).
- Export the campaign's GitHub-owned event surface: replace the single-event OpenTelemetry-only `/api/events` sink with a validated, per-event schema that persists a closed allowlist of events while retaining the existing counter.
- Capture UTM parameters client-side on first landing and sync them to the server on authentication.
- Insert a self-reported attribution step in the onboarding wizard before the OAuth channel-connect step.
- Instrument the insights feed with a view observer and record a small number of high-intent actions.
- Compute the ACC/TTFI funnel server-side through a read-only SQL view over existing and new tables; existing signup (`users.created_at`), OAuth (`user_channel_tracking.created_at`), star, export, and digest-opt-in data already live in SQLite and are reused.

## Capabilities

### New Capabilities
- `campaign-attribution`: Campaign attribution capture (UTM + self-reported attribution), durable per-user product event ingestion (`insight_viewed`, `high_intent_action`, anonymous `landing_view`), and the cohort view that feeds ACC/TTFI/funnel reporting.

### Modified Capabilities
- `storage-layer`: Adds the `user_attribution` and `product_events` tables, the `users.is_admin` column is NOT part of this change, and a `v_campaign_cohort` read-only view via a new versioned migration.
- `frontend-dashboard`: Adds the self-reported attribution onboarding step, first-touch UTM capture, insight-card view instrumentation, and high-intent action events.

## Impact

- **Database / migrations**: `internal/db/schema.sql`, `internal/db/migrations.go` — new tables `user_attribution` and `product_events`, indexes, and the `v_campaign_cohort` view. Both tables cascade on user deletion.
- **Backend**:
  - `internal/db/attribution.go`, `internal/db/product_events.go` — new stores embedded into the `DBQuery` interface (`internal/db/db.go`).
  - `internal/api/attribution_handlers.go` — `POST /api/attribution`, `GET /api/attribution/me`.
  - `internal/api/events_handlers.go` — per-event validation schema, persistence, and OTel counter retention.
  - `cmd/api/main.go` — new authenticated routes and an anonymous `POST /public/events` beacon route.
- **Frontend**:
  - `frontend/src/lib/attribution.ts`, `frontend/src/services/attribution.ts`, `frontend/src/stores/auth.ts` — UTM capture and sync.
  - `frontend/src/views/auth/OnboardingView.vue` — new SRA step.
  - `frontend/src/components/CommentCard.vue`, `frontend/src/views/dashboard/InsightsFeed.vue`, `frontend/src/views/settings/AccountView.vue`, `frontend/src/services/events.ts` — instrumentation.
- **Lifecycle / privacy**: anonymous `landing_view` rows are cleared by a TTL sweep; attribution and product-event data are included in GDPR export and hard-deleted with the user.
- **Metrics scope**: ACC is YouTube-only for this campaign; TikTok OAuth is not shipped.
