## 1. Backend event capture

- [ ] 1.1 Migration: `reply_draft_clicks` table (id, user_id, comment_id, count, created_at; unique user+comment)
- [ ] 1.2 `POST /api/events/reply-draft-click` handler: auth + comment-access 403 check, upsert counter, 202
- [ ] 1.3 OTel counter `noumena_reply_draft_clicks_total` (labels: category, channel_id)
- [ ] 1.4 `REPLY_DRAFT_PAINTED_DOOR_ENABLED` config flag + `.env.example`; 503 when off
- [ ] 1.5 Handler tests: first click 202, repeat increments, foreign comment 403, flag-off 503, rate limit

## 2. Frontend painted door

- [ ] 2.1 "Draft reply" icon button on CommentCard (feed + saved) with aria-label + "coming soon" tooltip
- [ ] 2.2 Click → POST event + "coming soon, interest measured" toast; flag-gated render
- [ ] 2.3 Component tests: click emits event + toast; button absent when flag off

## 3. Measurement & gate

- [ ] 3.1 Go/No-Go query: WAU denominator + click rates (single + repeat-clicker cohorts)
- [ ] 3.2 Kill criterion documented in proposal: <10% of WAU click within 4 weeks → abandon
- [ ] 3.3 Review checkpoint scheduled 4 weeks after flag-on deploy; result recorded in change notes