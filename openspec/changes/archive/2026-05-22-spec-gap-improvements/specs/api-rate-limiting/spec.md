# API Rate Limiting Specification

## Purpose
Enforces per-user API rate limits to protect backend resources from abuse and ensure fair usage. Implements PostgreSQL-backed sliding window counters with no external dependencies.

## Requirements

### Requirement: Authenticated Rate Limit
The system SHALL enforce a rate limit of 100 requests per minute for authenticated users.

#### Scenario: Within Rate Limit
- **WHEN** an authenticated user makes their 50th request within a 60-second window
- **THEN** the system SHALL allow the request and update the counter

#### Scenario: Rate Limit Exceeded
- **WHEN** an authenticated user makes their 101st request within a 60-second window
- **THEN** the system SHALL return 429 with `{"code": "rate_limit_exceeded", "retry_after": <seconds>}` and set `Retry-After` header

#### Scenario: Window Reset
- **WHEN** 60 seconds have elapsed since the start of the current window
- **THEN** the system SHALL reset the counter and allow new requests

### Requirement: Unauthenticated Rate Limit
The system SHALL enforce a rate limit of 10 requests per minute for unauthenticated requests.

#### Scenario: Unauthenticated Within Limit
- **WHEN** an unauthenticated client makes their 5th request within a 60-second window to a public endpoint
- **THEN** the system SHALL allow the request

#### Scenario: Unauthenticated Limit Exceeded
- **WHEN** an unauthenticated client makes their 11th request within a 60-second window
- **THEN** the system SHALL return 429 with `{"code": "rate_limit_exceeded", "retry_after": <seconds>}`

### Requirement: Rate Limit Storage
The system SHALL store rate limit counters in PostgreSQL using a sliding window approach.

#### Scenario: Counter Increment
- **WHEN** a request passes through the rate limit middleware
- **THEN** the system SHALL execute `INSERT INTO rate_limit_buckets (user_key, window_start, count) VALUES ($1, $2, 1) ON CONFLICT (user_key, window_start) DO UPDATE SET count = count + 1`

#### Scenario: Counter Cleanup
- **WHEN** the cleanup ticker fires (default every 5 minutes)
- **THEN** the system SHALL delete rows where `window_start < NOW() - INTERVAL '2 minutes'`

### Requirement: Rate Limit Bypass
The system SHALL allow specific endpoints to bypass rate limiting.

#### Scenario: Health Check Bypass
- **WHEN** a request hits GET /health or GET /ready
- **THEN** the system SHALL NOT apply rate limiting

#### Scenario: Webhook Bypass
- **WHEN** a request hits POST /webhooks/stripe
- **THEN** the system SHALL NOT apply rate limiting (Stripe retries must not be blocked)

### Requirement: Rate Limit Status Endpoint
The system SHALL provide an endpoint for users to check their current rate limit status.

#### Scenario: Get Rate Limit Status
- **WHEN** an authenticated user calls GET /api/user/rate-limit-status
- **THEN** the system SHALL return `{"limit": 100, "remaining": <count>, "reset_at": <timestamp>, "window_seconds": 60}`

### Requirement: IP-Based Fallback for Unauthenticated
The system SHALL use client IP as the rate limit key for unauthenticated requests.

#### Scenario: IP Key Extraction
- **WHEN** an unauthenticated request arrives
- **THEN** the system SHALL use X-Forwarded-For header (first IP) or RemoteAddr as the rate limit key

#### Scenario: Shared IP Fairness
- **WHEN** multiple unauthenticated users share an IP (NAT, corporate network)
- **THEN** the system SHALL share the 10 req/min limit among them (documented limitation)
