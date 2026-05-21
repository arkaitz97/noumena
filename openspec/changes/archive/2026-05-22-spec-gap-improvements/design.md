## Context

Noumena has 10 well-structured specs covering auth, channels, ingestion, LLM processing, insights, billing, data lifecycle, email, weekly digest, and frontend dashboard. However, cross-cutting concerns (rate limiting, observability, multi-seat) were deferred during initial implementation, and several specs have gaps against the PRD requirements. The codebase is Go backend + Vue 3 frontend with PostgreSQL-backed job queues, chi router, and feature-flagged modules.

Current state: All 10 specs are implemented to varying degrees (~70% average completion per DEFERRED_TASKS.md). Build is green. The primary blockers are integration test harness and staging environment, not spec gaps.

## Goals / Non-Goals

**Goals:**
- Close all spec gaps identified between PRD v1.3 and current specs
- Add 3 new capability specs: rate limiting, multi-seat workspaces, observability
- Fix 9 existing specs with missing requirements
- Maintain backward compatibility — no breaking changes to existing APIs
- All new specs follow RFC 2119 + GIVEN/WHEN/THEN format

**Non-Goals:**
- Do not implement TikTok/Instagram full ingestion (placeholder only, deferred to v2)
- Do not implement AI reply suggestions (PRD out of scope for v1)
- Do not implement real-time WebSocket feed (PRD out of scope for v1)
- Do not change existing database schema for already-implemented features
- Do not address deferred integration tests (separate workstream)

## Decisions

### 1. Rate Limiting: PostgreSQL-backed sliding window (not Redis)
**Decision**: Use a `rate_limit_buckets` table with sliding window counters, cleaned up by a background goroutine.
**Rationale**: Matches existing architecture pattern (no Redis dependency). PostgreSQL `INSERT ... ON CONFLICT` with atomic increment provides sufficient performance for 100 req/min per user.
**Alternatives considered**: Redis (faster but adds infra dependency), in-memory map (not shared across replicas), token bucket algorithm (more complex, sliding window is simpler and sufficient).

### 2. Multi-Seat: Workspace-scoped, not global roles
**Decision**: Users belong to workspaces; roles are workspace-scoped. A user can be Admin in one workspace and Viewer in another. Default workspace created on registration.
**Rationale**: Matches PRD AUTH-05 requirements. Workspace-scoped roles are more flexible than global roles and align with SaaS best practices.
**Alternatives considered**: Global roles (simpler but doesn't support multi-agency users), organization-based (too heavy for MVP).

### 3. Observability: OpenTelemetry with no-op default
**Decision**: Use OpenTelemetry SDK with no-op providers when `OTEL_ENABLED=false`. Zero performance impact when disabled.
**Rationale**: Already referenced in AGENTS.md conventions. No-op default means no regression for existing deployments. Structured logging via `slog` with redaction for sensitive fields.
**Alternatives considered**: Custom metrics (reinventing the wheel), Prometheus-only (less portable), Datadog agent (vendor lock-in).

### 4. GDPR Deletion Window: 30 days (align with PRD)
**Decision**: Change `DELETION_REVERSIBLE_HOURS` default from 48h to 720h (30 days). Keep configurable via env var.
**Rationale**: PRD Flow 6 states "reversible within 48 hours if purge has not yet run" but GDPR Art. 17 requires deletion within 30 days. The 48h was for user reversal; 30 days is the outer bound for actual data purge. We'll keep 48h for user reversal but extend the worker execution window to 30 days.
**Alternatives considered**: Keep 48h (non-compliant with GDPR), make it purely configurable (confusing defaults).

### 5. Email Templates: Extend existing pipeline, not new system
**Decision**: Add missing templates to the existing `internal/email/templates.go` registry. Each template has HTML + text variants using Go's `html/template` and `text/template`.
**Rationale**: Consistent with existing email-pipeline spec. No new infrastructure needed.
**Alternatives considered**: Separate email service (overkill), third-party template engine (adds dependency).

### 6. Comment Thread Structure: Store parent_comment_id, resolve in API
**Decision**: Ingestion stores `parent_comment_id` from YouTube API. Insights API resolves thread depth on query. Frontend renders nested view up to 3 levels.
**Rationale**: PRD ING-02 requires thread structure. Storing parent_comment_id is minimal overhead. Thread resolution at query time avoids recursive writes.
**Alternatives considered**: Materialized path (complex for MVP), nested set model (write-heavy), flat with client-side assembly (loses server-side filtering).

### 7. Custom LLM Prompt: Per-channel, validated, max 500 chars
**Decision**: Add `custom_prompt TEXT` column to `user_channel_tracking`. Validated on write (max 500 chars, no HTML/script tags). Prepended to system prompt when processing that channel's comments.
**Rationale**: PRD LLM-06 (Could priority). Per-channel is the right granularity. 500 chars prevents prompt injection attacks.
**Alternatives considered**: Global custom prompt (too broad), per-user (doesn't match channel-specific needs), unlimited length (security risk).

## Risks / Trade-offs

- **[Rate limiting performance]** PostgreSQL-based rate limiting may become a bottleneck at 10K+ concurrent users → Mitigation: Add in-memory LRU cache with 1-second TTL as first check; DB is authoritative but cache handles hot paths
- **[Multi-seat complexity]** Workspace model adds significant schema and middleware changes → Mitigation: Phase rollout — implement workspace creation first, then invites, then role enforcement. Each phase is independently deployable.
- **[GDPR window extension]** 30-day reversible window means deletion jobs sit longer → Mitigation: Jobs are in 'pending' state with no side effects until worker claims them. No data is at risk during the waiting period.
- **[Custom prompt injection]** User-supplied prompts could manipulate LLM behavior → Mitigation: Strict validation (no HTML, no script tags, no system prompt override patterns), prepend-only (user text goes before system instructions), audit log all custom prompts
- **[Observability overhead]** OTel SDK adds binary size and initialization time → Mitigation: No-op when `OTEL_ENABLED=false`. SDK is only ~2MB. Initialization is <10ms with no-op provider.
- **[Thread depth limit]** 3-level nesting may not cover all YouTube threads → Mitigation: YouTube comment threads are typically 2 levels (comment + replies). 3 levels covers edge cases. "View more replies" link for deeper threads.
