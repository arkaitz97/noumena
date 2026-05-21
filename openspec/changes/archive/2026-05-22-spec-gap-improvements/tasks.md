## 1. Database Schema Migrations

- [x] 1.1 Create `rate_limit_buckets` table (user_key, window_start, count, unique constraint on user_key+window_start)
- [x] 1.2 Create `workspaces` table (id, name, created_by, created_at)
- [x] 1.3 Create `workspace_members` table (workspace_id, user_id, role, created_at, unique on workspace_id+user_id)
- [x] 1.4 Create `workspace_invites` table (id, workspace_id, email, role, token_hash, expires_at, consumed_at)
- [x] 1.5 Create `audit_log` table (id, user_id, action, details JSONB, ip_address, created_at)
- [x] 1.6 Add `custom_prompt TEXT` column to `user_channel_tracking` table
- [x] 1.7 Add `reply_depth INT` column to `comments` table (default 0)
- [x] 1.8 Add index on `comments(parent_comment_id)` for thread queries
- [x] 1.9 Add index on `rate_limit_buckets(window_start)` for cleanup queries

## 2. API Rate Limiting

- [x] 2.1 Implement `rate_limit` middleware with PostgreSQL-backed sliding window
- [x] 2.2 Add rate limit bypass for /health, /ready, /webhooks/stripe
- [x] 2.3 Implement IP-based key extraction for unauthenticated requests (X-Forwarded-For fallback)
- [x] 2.4 Add background cleanup goroutine for expired rate_limit_buckets rows (5-minute tick)
- [x] 2.5 Implement GET /api/user/rate-limit-status endpoint
- [x] 2.6 Add 429 response handler with Retry-After header
- [x] 2.7 Unit tests for rate limit counter increment, window reset, and bypass

## 3. Multi-Seat Workspaces

- [x] 3.1 Implement default workspace creation on user registration
- [x] 3.2 Implement workspace CRUD endpoints (GET/POST/PATCH/DELETE /api/workspaces)
- [x] 3.3 Implement invite system: POST /api/workspaces/{id}/invites with email + role
- [x] 3.4 Implement invite email template and signed link generation
- [x] 3.5 Implement GET /invites/accept?token=... endpoint (existing user + new user flows)
- [x] 3.6 Implement workspace-scoped channel access middleware
- [x] 3.7 Implement role enforcement middleware (Admin/Analyst/Viewer permission checks)
- [x] 3.8 Implement GET /api/workspaces/mine (list user's workspaces)
- [x] 3.9 Implement PUT /api/workspaces/{id}/activate (switch active workspace)
- [x] 3.10 Implement DELETE /api/workspaces/{id}/members/{user_id} (remove member)
- [x] 3.11 Implement PATCH /api/workspaces/{id}/members/{user_id} (change role)
- [x] 3.12 Add last-admin self-removal prevention logic
- [x] 3.13 Unit tests for workspace creation, invite flow, role enforcement

## 4. Observability & Metrics

- [x] 4.1 Add OpenTelemetry SDK dependency and initialize with no-op default
- [x] 4.2 Implement OTel initialization when OTEL_ENABLED=true
- [x] 4.3 Add HTTP middleware for request metrics (noumena_http_requests_total, duration histogram)
- [x] 4.4 Add slow request logging (>1000ms threshold)
- [x] 4.5 Add ingestion metrics (comments_ingested_total, errors_total, lock_contention_total)
- [x] 4.6 Add LLM processing metrics (requests_total, duration, tokens_total, failover_total)
- [x] 4.7 Add billing metrics (quota_usage gauge, webhooks_total, webhook_duration)
- [x] 4.8 Add lifecycle metrics (deletion_stage, deletions_total, export_size_bytes, exports_total)
- [x] 4.9 Implement GET /health endpoint (uptime, status)
- [x] 4.10 Implement GET /ready endpoint (database connectivity check)
- [x] 4.11 Implement structured logging with slog and email redaction
- [x] 4.12 Implement GET /metrics endpoint (Prometheus format)
- [x] 4.13 Unit tests for metrics registration and no-op behavior when disabled

## 5. Data Lifecycle Improvements

- [x] 5.1 Update DELETION_REVERSIBLE_HOURS default from 48 to 720 (30 days)
- [x] 5.2 Add audit_log entries for deletion_requested, deletion_cancelled, deletion_completed
- [x] 5.3 Implement annual plan cancellation at period end (Stripe subscription.cancel(at_period_end=true))
- [x] 5.4 Update deletion_scheduled email template to show 30-day reversible window
- [x] 5.5 Update cancel-deletion endpoint to check 30-day window
- [x] 5.6 Integration tests for audit trail on deletion events

## 7. Billing Improvements

- [x] 7.1 Add annual plan pricing to plans.go (20% discount calculations)
- [x] 7.2 Implement customer.subscription.updated webhook handler with downgrade logic
- [x] 7.3 Implement downgrade channel pausing (alphabetical, beyond new plan limit)
- [x] 7.4 Add downgrade notification email
- [x] 7.5 Update checkout session to support annual interval
- [x] 7.6 Unit tests for annual pricing, downgrade behavior, webhook handling

## 8. Insights API Improvements

- [x] 8.1 Implement GET /api/comments/search?q= with cross-channel search
- [x] 8.2 Add search query validation (min 3 chars)
- [x] 8.3 Implement POST /api/comments/bulk-star endpoint
- [x] 8.4 Implement POST /api/comments/bulk-unstar endpoint
- [x] 8.5 Implement GET /api/comments/{id}/thread with 3-level depth
- [x] 8.6 Add thread access control (user must track the channel)
- [x] 8.7 Add composite index on comments(platform_channel_id, parent_comment_id) for thread queries
- [x] 8.8 Integration tests for search, bulk operations, and thread view

## 9. Ingestion Thread Structure

- [x] 9.1 Update YouTube client to use commentThreads.list API (not just comments.list)
- [x] 9.2 Implement parent_comment_id extraction from YouTube API response
- [x] 9.3 Implement reply_depth calculation (0 for top-level, 1 for reply, 2 for reply-to-reply)
- [x] 9.4 Update comment batch insert to include parent_comment_id and reply_depth
- [x] 9.5 Implement reply pagination via nextPageToken
- [x] 9.6 Integration tests for thread ingestion with parent/reply relationships

## 10. LLM Processing Improvements

- [x] 10.1 Add custom_prompt column read in LLM processor
- [x] 10.2 Implement custom prompt validation (max 500 chars, no HTML/scripts)
- [x] 10.3 Implement PATCH /api/channels/{id}/custom-prompt endpoint
- [x] 10.4 Update classification prompt to prepend custom_prompt when set
- [x] 10.5 Implement thread-aware classification (include parent comment context, truncated to 200 chars)
- [x] 10.6 Unit tests for custom prompt validation and thread-aware classification

## 11. Weekly Digest Email Delivery

- [x] 11.1 Wire digest_email template to digest generation flow
- [x] 11.2 Honor digest_email_opt_in preference before sending
- [x] 11.3 Mark emailed_at on weekly_digests row after successful send
- [x] 11.4 Implement digest_delayed email for deferred digests
- [x] 11.5 Update digest query to explicitly exclude paused channels (tracking_status='active')
- [x] 11.6 Integration tests for digest email delivery and opt-in behavior

## 12. Channel Management Platform Placeholders

- [x] 12.1 Implement GET /api/channels/connect/tiktok returning 501
- [x] 12.2 Implement POST /api/channels/exchange/tiktok returning 501
- [x] 12.3 Implement GET /api/channels/connect/instagram returning 501
- [x] 12.4 Implement POST /api/channels/exchange/instagram returning 501
- [x] 12.5 Add platform validation middleware (reject unsupported platforms with 400)
- [x] 12.6 Unit tests for platform validation and 501 responses

## 13. Frontend Dashboard Improvements

- [x] 13.1 Implement onboarding progress indicator with sync-status polling (10s interval)
- [x] 13.2 Add sync complete notification and redirect to insights feed
- [x] 13.3 Implement keyboard shortcuts: 's' star, 'j/k' navigate, '/' search, '?' help
- [x] 13.4 Add keyboard shortcut help modal
- [x] 13.5 Implement dark mode toggle with localStorage persistence
- [x] 13.6 Add system preference detection (prefers-color-scheme)
- [x] 13.7 Add rate limit error handling UI (429 banner with countdown)
- [x] 13.8 Add workspace switcher UI in header
- [x] 13.9 Add role-based UI element visibility (hide admin-only controls for Viewer/Analyst)
- [x] 13.10 Manual testing checklist for all new UI features
