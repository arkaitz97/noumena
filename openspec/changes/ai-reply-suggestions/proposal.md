# Proposal: AI Reply Suggestions (painted door first)

## Why

PRD §8 defers "AI-generated comment reply suggestions" to v1.5, but replying to comments is the creator's highest-frequency work-loop after publishing — and the natural conversion point from "intelligence" to "action". Before committing LLM spend, validate demand with the cheapest test: a **painted door** (button that signals the feature, records intent, does nothing yet). Evidence-based gate: if engagement is below the kill criterion, we drop it without building.

## What Changes

- **Painted door (Explore phase only — no LLM build):**
  - "Draft reply" button on `CommentCard` (feed + saved views) next to star/export actions.
  - Click → tracked event (`POST /api/events/reply-draft-click` with comment id) + toast "Reply drafts are coming — vote below / tell us" (no fake generation).
  - OTel counter `noumena_reply_draft_clicks_total` per comment category/channel for cohort analysis.
  - Kill criterion defined up front: <10% of weekly active dashboard users click at least once within 4 weeks → abandon, no build.
- **If the gate passes (follow-up change):** backend drafts (LLM-generated, tone options), copy-to-clipboard, and (later) comment posting via the platform API — explicitly out of scope here.

## Capabilities

### New Capabilities
- `reply-draft-requests`: minimal event-capture endpoint for painted-door demand measurement (no generation capability yet).

### Modified Capabilities
- `frontend-dashboard`: "Draft reply" button on comment cards with toast + event emission.

## Impact

- `frontend/src/components/CommentCard.vue` — button + click handler.
- `internal/api/` — small `POST /api/events/reply-draft-click` handler (auth-scoped, rate-limited, append-only event log or OTel counter only; no schema change unless events need durability).
- `internal/telemetry/` — counter metric.
- No LLM client, no new external dependency, no billing impact.