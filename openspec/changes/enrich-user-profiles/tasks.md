## 1. Database & Storage

- [x] 1.1 Add SQLite migration v15 for `user_profiles` table with foreign key to `users(id)` and constraints in `internal/db/migrations.go`
- [x] 1.2 Implement DB methods `GetUserProfile` and `UpsertUserProfile` in `internal/db/`
- [x] 1.3 Database unit tests for profile migration and DB operations in `internal/db/`

## 2. API & Validation

- [x] 2.1 Implement `GET /api/user/profile` and `PATCH /api/user/profile` handlers in `internal/api/profile_handlers.go`
- [x] 2.2 Register `/api/user/profile` routes in `cmd/api/main.go`
- [x] 2.3 API handler tests for retrieval, updates, invalid enum values, and length limits

## 3. Data Lifecycle & GDPR

- [x] 3.1 Include profile data in GDPR export bundle as `profile/profile.json` in `internal/lifecycle/export_worker.go`
- [x] 3.2 Add `user_profiles` deletion to account erasure final wipe in `internal/db/lifecycle.go`
- [x] 3.3 Test GDPR export serialization and account deletion cascading for profile records

## 4. Weekly Digest Personalization

- [x] 4.1 Update digest LLM prompt builder in `internal/llm/digest_llm.go` to inject sanitized profile context when `DIGEST_PROFILE_CONTEXT_ENABLED=true`
- [x] 4.2 Wire profile retrieval in `internal/llm/digest_worker.go`
- [x] 4.3 Unit tests for digest prompt construction with and without profile context

## 5. Frontend Dashboard & UI

- [x] 5.1 Add profile types and API client functions in `frontend/src/services/`
- [x] 5.2 Add profile store state, actions, and getters in Pinia
- [x] 5.3 Implement "About your work" step in onboarding with skip option in `frontend/src/views/auth/OnboardingView.vue`
- [x] 5.4 Implement Profile Editor card in Account Settings in `frontend/src/views/settings/AccountView.vue`
- [x] 5.5 Frontend component tests for onboarding profile step and settings editor

## 6. Documentation & Configuration

- [x] 6.1 Update `docs/api/user.md` with `/api/user/profile` schema and examples
- [x] 6.2 Document `DIGEST_PROFILE_CONTEXT_ENABLED` flag in `.env.example`
