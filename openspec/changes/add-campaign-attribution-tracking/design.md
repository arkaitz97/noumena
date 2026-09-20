## Context

Noumena is a Go 1.25 + chi backend over embedded SQLite, with a Vue 3 + Vite frontend. The campaign measurement framework defines Activated Channel Connections (ACC) as its north-star metric: a creator who signs up, completes YouTube OAuth, and views at least one AI-generated insight within 48 hours. Supporting diagnostics are outreach-to-OAuth completion, time-to-first-insight (TTFI) and consumption depth, and a high-intent downstream action rate.

Three facts constrain the design:

1. Existing tables already carry most funnel timestamps. `users.created_at` is the signup time; `user_channel_tracking.created_at` is the OAuth completion time; `user_comment_stars`, `comment_exports`, and `users.digest_email_opt_in` are already persisted high-intent proxies. No new signal is needed for those.
2. There is no persistence for campaign attribution or insight views. The only UI telemetry path (`POST /api/events`, `internal/api/events_handlers.go`) validates a single event against a closed allowlist and increments an OpenTelemetry counter; it stores nothing.
3. OpenTelemetry metrics cannot express ACC. Counters are aggregate, and the repository's cardinality firewall forbids user-supplied attribute values (`docs/observability.md`, `internal/telemetry/attrs.go`). Per-user correlation with a 48-hour window requires SQL.

The campaign is zero-budget, solo-operated, and runs for 14 days with an expected N of 50-100 activated channels. Attribution must survive mobile-view → desktop-auth journeys, which break cookie/UTM tracking.

## Goals / Non-Goals

**Goals:**
- Persist first-touch UTM attribution and a self-reported attribution (SRA) answer per user.
- Persist a minimal, closed set of per-user product events sufficient to compute ACC, TTFI, consumption depth, and high-intent actions.
- Reuse existing tables for signup, OAuth, star, export, and digest-opt-in signals.
- Provide a read-only SQL view (`v_campaign_cohort`) that the reporting change consumes.
- Preserve the existing cardinality firewall for OTel metrics.

**Non-Goals:**
- No third-party analytics vendor (PostHog, Segment, GA).
- No Grafana/Prometheus campaign time series. Operational metrics stay in OTel; campaign cohort analytics stay in SQLite.
- No TikTok attribution (product ships YouTube-only; ACC is scoped to YouTube for this campaign).
- No new high-intent product features ("copy collab lead", "save idea" do not exist and are not built here).
- No global admin role or dashboard surface (delivered by the companion change `add-campaign-metrics-dashboard`).

## Decisions

### 1. Persist campaign data in SQLite, not OTel
**Decision:** New `user_attribution` and `product_events` tables plus a `v_campaign_cohort` view in `noumena.db`.

Rationale: ACC needs a per-user join with a 48-hour window and breakout by UTM/angle; OTel counters cannot express this and the repo already forbids high-cardinality attributes. Alternatives considered: (a) extend OTel counters only — cannot answer ACC; (b) daily CSV export — no first-insight signal without the same new instrumentation and far less queryable. SQLite is the existing source of truth and already holds signup, OAuth, star, and export data.

### 2. Separate attribution and event tables (not columns on `users`)
**Decision:** `user_attribution` is a 1:1 side table keyed by `user_id`; `product_events` is append-only.

Rationale: keeps the already-wide `users` row lean; keeps append-only event volume out of the hot user row; both tables use `ON DELETE CASCADE` so GDPR hard-delete and the existing lifecycle worker need no special casing. Alternative: columns on `users` — rejected because landing views (anonymous) and multiple insight views do not fit a 1:1 row.

### 3. Anonymous landing beacon with `anon_id` linking
**Decision:** `POST /public/events` accepts only `landing_view`, is protected by the always-on anonymous rate limiter, and writes a `product_events` row with `user_id = NULL` and a client-generated `anon_id`. The same `anon_id` is stored on `user_attribution` at signup.

Rationale: visit→signup conversion is a required funnel stage and cannot be derived server-side. `anon_id` is an opaque, client-generated identifier with no PII. Trade-off: anonymous rows accumulate; a TTL sweep limits retention.

### 4. Closed per-event schema, cardinality firewall preserved
**Decision:** Replace the single-prop `eventSections` map with a per-event schema describing allowed prop keys and their enum values. Unknown event or prop value returns 400 with no write. The OTel counter is retained.

Rationale: the repo's telemetry policy is that no user-supplied value reaches a metrics backend. Validation happens at the HTTP boundary; only validated enums reach the database (as JSON props) and the counter.

### 5. Client-side first-touch UTM capture, server sync on auth
**Decision:** `frontend/src/lib/attribution.ts` reads `window.location.search` on boot, persists UTM + `document.referrer` + `anon_id` under a localStorage key (first-touch wins), and the auth store POSTs it to `POST /api/attribution` once per authenticated user.

Rationale: UTM is only present on the landing request. Server-side capture would require threading it through every auth flow. Sync-on-auth avoids touching registration/login handlers. Known limitation: localStorage is per-device, which is precisely why SRA exists.

### 6. SRA as an onboarding wizard step
**Decision:** Insert the SRA question as a required step between the profile step and the channel-connect step in `OnboardingView.vue`.

Rationale: the framework mandates the survey appear post-registration and before OAuth. The onboarding wizard already runs in exactly that window and only for users with no channels. Alternative: a dedicated route — rejected because the `requiresOnboarded` guard keys on channel presence and would not gate it cleanly; also more friction.

### 7. First insight view via IntersectionObserver, deduped per session
**Decision:** `CommentCard.vue` observes visibility and fires `insight_viewed` with the comment id once per session per comment (module-level `Set`).

Rationale: ACC is satisfied by the first view, but consumption depth (`≥ 3 insights`) needs distinct comment ids. Rendering alone over-counts off-screen cards; an observer counts cards actually seen. Server-side dedup is not required because reporting uses `MIN(created_at)`.

### 8. Reporting is a separate change
**Decision:** This change only produces and persists the data plus the `v_campaign_cohort` view. The dashboard/report surface, global admin role, and CSV/endpoint delivery live in `add-campaign-metrics-dashboard`.

Rationale: the write path touches auth and user data and carries different risk than a read-only surface; they can ship and iterate independently.

## Risks / Trade-offs

- **Cross-device attribution loss** → Mitigated by the mandatory SRA step, which is the framework's designed fallback for exactly this case. Residual loss is expected and documented as a known ceiling on UTM-only numbers.
- **Anonymous landing rows accumulate** → Add a TTL sweep for `product_events` rows with `user_id IS NULL` older than the campaign retention window (reuse the lifecycle cleanup pattern).
- **Survey step adds friction before OAuth and can reduce conversion** → The step is a single click with five options; the funnel report explicitly measures OAuth drop-off so the cost is visible. If drop-off is unacceptable, the step can be demoted without a schema change.
- **Returning users never see the SRA step** (onboarding only runs with zero channels) → Acceptable: the campaign cohort is new signups. Existing users are out of scope.
- **`/api/events` documented as non-persisting** → The handler comment and `observability-metrics` contract must be updated; otherwise the "events are NOT persisted" statement becomes false.
- **PII in event props** → The schema only permits enum values and opaque ids (`comment_id`, `anon_id`); no free-form text is stored except the optional SRA "other" free-text field, which is length-capped.
- **SQLite write volume** → Insight views are one row per comment per user per session and capped by the session `Set`; volume is negligible at campaign scale.

## Migration Plan

1. Add migration v22: create `user_attribution`, `product_events`, their indexes, and the `v_campaign_cohort` view. Mirror the DDL in `schema.sql` for fresh installs. All DDL is `CREATE ... IF NOT EXISTS` and the view is recreated idempotently.
2. Deploy backend first (tables exist, no callers yet); frontend instrumentation is inert until the backend routes exist.
3. Rollback: the change is additive. Reverting the binary is safe; the tables and view can remain without affecting existing behavior. The view and tables can be dropped manually if a full rollback is required.
4. No backfill: attribution and event rows only exist for traffic after deploy; the campaign starts after deploy.

## Open Questions

- Should the SRA "other" free-text answer be stored verbatim or normalized to `other`? Current design stores a length-capped verbatim string for early-campaign learning.
- Should the anonymous landing beacon be retained if the campaign records visits manually? The design includes it; it can be dropped without affecting ACC.
