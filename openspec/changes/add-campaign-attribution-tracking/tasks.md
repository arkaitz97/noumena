## 1. Schema and migration

- [ ] 1.1 Add `user_attribution` and `product_events` DDL plus indexes to `internal/db/schema.sql`
- [ ] 1.2 Add migration v22 in `internal/db/migrations.go` creating both tables, indexes, and the `v_campaign_cohort` view (idempotent)
- [ ] 1.3 Add migration test coverage in `internal/db/migrations_test.go` for fresh install and upgrade paths
- [ ] 1.4 Verify cascade behavior is covered by a DB test (user delete removes attribution and events)

## 2. Database stores

- [ ] 2.1 Implement `internal/db/attribution.go` (upsert with partial update, get by user)
- [ ] 2.2 Implement `internal/db/product_events.go` (insert, first per user/event, distinct comment list)
- [ ] 2.3 Define `AttributionStore` and `ProductEventStore` interfaces and embed them in `DBQuery` in `internal/db/db.go`
- [ ] 2.4 Add store unit tests covering partial upsert, enum persistence, and null-user anonymous rows

## 3. Attribution API

- [ ] 3.1 Implement `internal/api/attribution_handlers.go` with `POST /api/attribution` and `GET /api/attribution/me`
- [ ] 3.2 Add enum/length validation for UTM fields, `sra_source`, and `sra_other_text`
- [ ] 3.3 Mount the authenticated attribution routes in `cmd/api/main.go`
- [ ] 3.4 Add handler tests: happy path, partial update, invalid enum, unauthenticated request

## 4. Product event ingestion

- [ ] 4.1 Replace the single-prop `eventSections` map in `internal/api/events_handlers.go` with a per-event schema (allowed keys and enum values)
- [ ] 4.2 Persist accepted `insight_viewed` and `high_intent_action` events to `product_events` while retaining the OTel counter
- [ ] 4.3 Add the rate-limited anonymous `POST /public/events` route accepting only `landing_view`
- [ ] 4.4 Update the handler doc comment and `docs/observability.md` to reflect that allowlisted events are now persisted
- [ ] 4.5 Add handler tests: persisted view, high-intent enum, unknown event/prop rejected, public endpoint rejection path

## 5. Frontend attribution capture

- [ ] 5.1 Create `frontend/src/lib/attribution.ts` (first-touch UTM + referrer + `anon_id` in localStorage)
- [ ] 5.2 Create `frontend/src/services/attribution.ts` (`postAttribution`, `getAttribution`)
- [ ] 5.3 Sync stored attribution once after authentication in `frontend/src/stores/auth.ts`, non-blocking on failure
- [ ] 5.4 Add Vitest tests for first-touch-wins behavior and sync-once behavior

## 6. Self-reported attribution step

- [ ] 6.1 Extend the onboarding step machine in `frontend/src/views/auth/OnboardingView.vue` with the SRA step between profile and connect
- [ ] 6.2 Add the five-option question with optional `other` text and require a selection to advance
- [ ] 6.3 Submit the selection to the attribution endpoint before proceeding to connect
- [ ] 6.4 Add component tests for required selection, submission, and `other` text handling

## 7. Frontend instrumentation

- [ ] 7.1 Observe insight card visibility in `frontend/src/components/CommentCard.vue` and emit `insight_viewed` once per comment per session
- [ ] 7.2 Emit `high_intent_action` on star (`CommentCard.vue`), export (`InsightsFeed.vue`), and digest opt-in (`AccountView.vue`)
- [ ] 7.3 Widen the allowlist types in `frontend/src/services/events.ts` to cover the new events and props
- [ ] 7.4 Add Vitest tests for view dedup and each high-intent action

## 8. Lifecycle and privacy

- [ ] 8.1 Include `user_attribution` and `product_events` in the GDPR export bundle
- [ ] 8.2 Add a retention sweep deleting anonymous `landing_view` rows past the retention window
- [ ] 8.3 Add tests for export inclusion and anonymous sweep

## 9. Verification

- [ ] 9.1 Run `make test` and `gofmt`/lint clean
- [ ] 9.2 Run frontend unit tests and lint
- [ ] 9.3 Manually verify the cohort view returns expected ACC/TTFI values for seeded data
- [ ] 9.4 Run `openspec validate add-campaign-attribution-tracking --strict`
