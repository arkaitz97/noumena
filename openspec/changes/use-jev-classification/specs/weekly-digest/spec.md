## MODIFIED Requirements

### Requirement: Digest Generation
The system SHALL generate weekly digests via LLM summarization on a scheduled cron using a configurable generative provider (`DIGEST_LLM_PROVIDER`, default `opencode`), optionally incorporating user profile context.

#### Scenario: Successful Digest Generation
- **GIVEN** an active user with >= 10 processed comments in the last 7 days
- **WHEN** the digest cron runs (Monday 08:00 UTC)
- **THEN** the system SHALL fetch the user's comments (excluding toxic/spam), call the configured generative provider to generate top 5 content ideas, top 5 unanswered questions, top 5 actionable insights by type, and engagement health score, store the result in weekly_digests with status='generated', and optionally email if the user has opted in

#### Scenario: Insufficient Comment Data
- **GIVEN** an active user with fewer than 10 processed comments in the last 7 days
- **WHEN** the digest cron runs
- **THEN** the system SHALL NOT generate a digest and SHALL NOT attempt to send an email

#### Scenario: LLM Unavailable During Digest
- **GIVEN** the LLM API is unavailable during digest generation
- **WHEN** the digest job attempts classification
- **THEN** the system SHALL retry up to 3 times over 2 hours. If all fail, mark the digest as status='deferred'

#### Scenario: Digest uses the configured generative provider
- **GIVEN** `DIGEST_LLM_PROVIDER` is set to a generative provider (opencode, openai, or anthropic)
- **WHEN** the digest worker is started
- **THEN** the system SHALL generate digests through that provider and record digest token cost under that provider's name

## ADDED Requirements

### Requirement: Configurable generative digest provider
The digest worker SHALL select its generation provider from `DIGEST_LLM_PROVIDER` (default `opencode`) and `DIGEST_LLM_FALLBACK`, and SHALL reject non-generative providers. This applies regardless of `JEV_CLASSIFICATION_ENABLED`, because Jev cannot produce the free-text digest.

#### Scenario: Default provider preserves current behavior
- **GIVEN** `DIGEST_LLM_PROVIDER` is unset
- **WHEN** the digest worker starts with `DIGEST_WORKER_ENABLED=true`
- **THEN** the system SHALL use the OpenCode client with the existing `OPENCODE_BASE_URL`/`OPENCODE_MODEL` configuration

#### Scenario: Jev rejected as digest provider
- **GIVEN** `DIGEST_LLM_PROVIDER` is `typesafe` or `jev`
- **WHEN** the digest worker is started
- **THEN** the system SHALL refuse to start the worker and log that Jev cannot generate digest prose, leaving classification unaffected

#### Scenario: Digest provider requires a key
- **GIVEN** `DIGEST_LLM_PROVIDER` names a provider whose API key environment variable is empty
- **WHEN** the digest worker is started
- **THEN** the system SHALL refuse to start and log the missing key, rather than starting an unauthenticated worker
