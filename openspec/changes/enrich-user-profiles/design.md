## Context

Noumena has established authentication, billing, comment ingestion, and LLM processing, but currently treats all users identically regardless of persona (growing creators, agencies, brand/sponsor teams). The existing user account state stores only operational details (email, verification, tier, GDPR flags, channels).

The `enrich-user-profiles` proposal introduces a progressive profile model. To maintain user trust and avoid friction, profile collection is lightweight: primary persona and goals are collected during onboarding (with skip support), deeper workflow details are editable in settings, and sensitive data (exact MRR, follower counts, sensitive personal demographics) are strictly omitted.

## Goals / Non-Goals

**Goals:**
- Store structured declared profile attributes per user in a dedicated `user_profiles` table via SQLite migration v15.
- Provide authenticated REST API endpoints: `GET /api/user/profile` and `PATCH /api/user/profile` with comprehensive input validation.
- Integrate profile creation into the frontend onboarding wizard ("About your work" step prior to channel linking) with skip and completion tracking.
- Provide a dedicated profile management section in Account Settings (`AccountView.vue`).
- Wire optional profile context into the weekly digest LLM prompt builder behind the `DIGEST_PROFILE_CONTEXT_ENABLED` feature flag.
- Ensure full compliance with data lifecycle operations: include profile data in GDPR ZIP exports and cascade-delete profiles during account erasure.

**Non-Goals:**
- Workspace-level multi-user profiles (profiles remain 1:1 with `users` for this iteration).
- Automatic profile inference or scraping from connected social channels.
- Real-time personalization of comment classification models (only weekly digest prompt ordering/context is influenced).
- Mandatory gatekeeping during onboarding: users MUST be able to skip without blocking channel connection or core features.

## Decisions

### 1. Dedicated `user_profiles` table vs. columns on `users` table
- **Decision:** Introduce a separate `user_profiles` table with a foreign key to `users(id) ON DELETE CASCADE`.
- **Rationale:** The `users` table has undergone rebuilds and strict column constraints. Profile attributes have distinct lifecycle update cadences and are optional for legacy users. Isolating them avoids bloating the authentication table and simplifies GDPR export assembly.
- **Alternatives Considered:** Adding nullable columns to `users`. Rejected because schema changes to `users` require complex table rebuild migrations in SQLite without foreign key enforcement.

### 2. JSON arrays for multi-value fields (`content_topics`, `insight_priorities`)
- **Decision:** Store multi-select fields as JSON-encoded text (`TEXT`) with application-level length and value validation.
- **Rationale:** The lists are small (max 10 topics, max 3 insight priorities) and always fetched and updated atomically with the profile. Normalized junction tables would add needless query complexity for key-value tags.
- **Alternatives Considered:** Normalized `user_profile_topics` table. Rejected due to over-engineering for simple tag storage.

### 3. Progressive onboarding with non-blocking skip
- **Decision:** Introduce the profile step as Step 1 of onboarding (before channel connection), allowing the user to either save choices or click "Skip for now".
- **Rationale:** Forcing completion causes drop-offs. Allowing skip preserves activation while still capturing high-intent declared context from users seeking tailored setups.
- **Alternatives Considered:** Mandatory questionnaire. Rejected due to activation churn risk.

### 4. Digest prompt enhancement gated by environment variable
- **Decision:** Pass sanitized profile attributes (role, goals, content topics) to `internal/llm/digest_llm.go` only when `DIGEST_PROFILE_CONTEXT_ENABLED=true`.
- **Rationale:** Allows controlled cohort testing and immediate kill-switch if token cost increases or prompt drift degrades output quality.
- **Alternatives Considered:** Unconditional prompt injection. Rejected to guard against prompt regression and token cost inflation.

### 5. Display name / user name in progressive profile
- **Decision:** Store an optional `display_name` (up to 100 characters) in `user_profiles`, auto-populated from Google SSO claims on first registration when available, and editable in onboarding and Account Settings.
- **Rationale:** Gives creators control over how they are addressed in emails, weekly digest summaries, and workspace team lists without burdening the core `users` authentication table with a complex migration.
- **Alternatives Considered:** Adding `first_name` and `last_name` columns to `users`. Rejected because single display name better accommodates creator handles/mononyms and avoids cultural naming assumptions.

## Risks / Trade-offs

- **[Friction in Onboarding]** Additional wizard step might increase drop-off.  
  → *Mitigation:* Make the step skippable with a single click, autofocus the continue action, and provide clear explanatory copy on why this data helps.
- **[LLM Prompt Token Overhead]** Injecting user profile context consumes tokens and may distract the LLM from comment trends.  
  → *Mitigation:* Sanitize and compact profile context to concise key-value hints; enforce character caps on topics; gate behind feature flag.
- **[Data Drift between Settings and Profile]** Incomplete updates or client cache staleness.  
  → *Mitigation:* Use atomic `PATCH /api/user/profile` with optimistic UI updates in Pinia and authoritative server response payloads.

## Migration Plan

1. **Migration v15:** Run `CREATE TABLE user_profiles (...)` in `internal/db/migrations.go` with foreign keys and indexes.
2. **Backend deployment:** Handlers, validation, export integration, and deletion hooks deployed. Existing users have no profile row until visited or updated (handled gracefully with default empty object or null).
3. **Frontend deployment:** Onboarding flow and Account Settings profile editor deployed.
4. **Digest flag activation:** Enable `DIGEST_PROFILE_CONTEXT_ENABLED=true` on staging, evaluate digest quality, then roll out to production.
5. **Rollback strategy:** If issues occur, feature flag `DIGEST_PROFILE_CONTEXT_ENABLED` can be set to false without downtime; `user_profiles` table can remain inert.

## Open Questions

- Should workspace invites allow invitees to fill out their own profile, or inherit the workspace creator's profile? (Deferred to multi-seat workspace profile expansion).
- What telemetry events should track completion vs. skip rates without violating user privacy? (Add `onboarding_profile_completed` and `onboarding_profile_skipped` client-side events without field values).
