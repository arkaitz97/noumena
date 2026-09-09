## Context

PRD §8 explicitly defers AI reply drafting to v1.5; the product currently ends at "intelligence" (insights) with no action step. The cheapest valid demand test is a painted door: expose the button, measure intent clicks, build nothing. The repo already has an OTel metrics pattern (`noumena_*_total` counters in internal/telemetry + per-domain meters) to carry the measurement.

## Goals / Non-Goals

**Goals:**
- Measure real demand for reply drafting before any LLM spend: clicks per user/session, per category, per channel.
- Zero-risk to existing flows: button is additive, non-blocking, and rate-limited.
- Define the kill criterion now (proposal: <10% of weekly active dashboard users click within 4 weeks → abandon).

**Non-Goals:**
- Any reply generation, tone options, or posting — even a mock.
- Email/notification of votes.
- Persistent per-user event storage beyond an append-only log (metrics + raw log only).

## Decisions

1. **Event endpoint, not client-side-only tracking.** `POST /api/events/reply-draft-click` (authenticated, rate-limited via existing middleware) records `{comment_id, channel_id, category, insight_type}`; returns 202. Server-side capture keeps analytics honest across ad-blockers and allows cohort joins on user id.
2. **Storage: metrics counter + append-only log row.** Counter `noumena_reply_draft_clicks_total` (labels: category, channel_id) for dashboards; one row in a new `reply_draft_clicks` table (id, user_id, comment_id, created_at) for cohort analysis (weekly-active denominator comes from existing auth metrics). Table is small; no index beyond user+created_at.
3. **Button placement:** `CommentCard` actions row (feed + saved), tooltip "Draft reply — coming soon", click → POST + toast "Reply drafts are coming. We're measuring interest — thanks!" No fake generation UI.
4. **Session attribution:** one click per user per comment counts once (unique constraint user+comment); repeated clicks update a counter column — prevents button-mashing inflation.
5. **Kill criterion enforced at review, not in code:** 4-week window after launch; measurement report generated from the metrics; no auto-follow-up build.

## Risks / Trade-offs

- **Users may misread painted door as broken feature** → toast copy is explicit ("coming soon", no generation attempt); risk accepted as inherent to painted doors.
- **Click ≠ purchase intent** — mitigation: cohort of repeat-clickers (≥3 distinct comments) tracked separately; only repeat-clicker rate feeds the Go/No-Go.
- **No a11y regression:** button follows existing icon-button pattern with aria-label; keyboard reachable.
- **Feature-gated:** endpoint + button mount only when `REPLY_DRAFT_PAINTED_DOOR_ENABLED=true` (default off) so the door can be opened/closed without a deploy.