## Why

After reviewing all 10 specs against the PRD v1.3, roadmap, and deferred tasks, significant gaps were found that would cause production incidents, security vulnerabilities, and missing user-facing features. Key issues: API rate limiting (PRD NFR 4.3) has no spec; multi-seat workspaces (PRD AUTH-05) are undocumented; GDPR deletion window conflicts between spec (48h) and PRD (30 days); email pipeline is missing 6+ required templates; observability/metrics referenced in deferred tasks but has no spec.

## What Changes

- Add API rate limiting spec (100 req/min authenticated, 10 req/min unauthenticated per PRD NFR 4.3)
- Add multi-seat workspace spec (invite team members, roles: Admin/Analyst/Viewer per PRD AUTH-05)
- Add observability & metrics spec (OpenTelemetry instrumentation for all subsystems)
- Fix data-lifecycle: align deletion reversible window with GDPR 30-day requirement, add audit logging
- Fix email-pipeline: add missing templates (digest, trial_expiry, deletion_scheduled, deletion_completed, payment_grace_final, export_ready)
- Fix billing: document downgrade behavior (pause excess channels alphabetically), annual pricing details
- Fix insights-api: add global search across channels, bulk star operations, comment thread view support
- Fix ingestion: add comment thread structure handling (parent_comment_id, reply chains)
- Fix llm-processing: add custom prompt per channel support (PRD LLM-06, Could priority)
- Fix weekly-digest: add email delivery flow, per-channel digest exclusion details
- Fix channel-management: add TikTok/Instagram placeholder support (PRD CM-02 Should, CM-03 Could)
- Fix frontend-dashboard: add onboarding progress indicator, keyboard shortcuts, dark mode support

## Capabilities

### New Capabilities
- `api-rate-limiting`: Per-user rate limiting with sliding window, Redis-free PostgreSQL-backed implementation, 100 req/min authenticated / 10 req/min unauthenticated
- `multi-seat-workspaces`: Team member invites, role-based access control (Admin/Analyst/Viewer), workspace-scoped channel access, audit logging
- `observability-metrics`: OpenTelemetry instrumentation across all subsystems, structured logging, health check endpoints, metrics export

### Modified Capabilities
- `data-lifecycle`: Extend reversible window to 30 days (GDPR Art. 17), add audit trail for deletion events, add Stripe subscription cancellation at period-end for annual plans
- `email-pipeline`: Add 6 missing template types (digest_email, trial_expiry, deletion_scheduled, deletion_completed, payment_grace_final, export_ready) with HTML + text variants
- `billing`: Add customer.subscription.updated webhook handling details, document downgrade channel selection behavior, add annual plan pricing (20% discount)
- `insights-api`: Add global search endpoint, bulk star/unstar operations, comment thread view with parent_comment_id resolution
- `ingestion`: Add comment thread structure preservation (parent_comment_id, reply depth), add reply ingestion via YouTube API threads.list
- `llm-processing`: Add custom prompt per channel (max 500 chars, validated), add thread-aware classification prompt
- `weekly-digest`: Add email delivery flow (honor digest_email_opt_in), add per-channel digest exclusion details, add deferred digest retry email
- `channel-management`: Add TikTok OAuth flow placeholder, add Instagram OAuth flow placeholder, add platform validation
- `frontend-dashboard`: Add onboarding progress indicator, add keyboard shortcuts for power users, add dark mode toggle

## Impact

- **Backend**: New middleware for rate limiting, new tables for workspaces/invites/roles, OpenTelemetry SDK integration, email template additions, ingestion thread handling
- **Frontend**: Workspace switcher UI, role-based route guards, rate limit error handling, dark mode theme, onboarding progress, keyboard shortcut handlers
- **Database**: New tables (workspaces, workspace_members, workspace_invites, rate_limit_buckets), schema additions (custom_prompt on user_channel_tracking, audit_log table)
- **API**: New endpoints for rate limit status, workspace CRUD, invite management, global search, bulk operations, health checks, metrics
- **Ops**: OpenTelemetry collector configuration, metrics dashboard setup, alerting rules for rate limits and error rates
