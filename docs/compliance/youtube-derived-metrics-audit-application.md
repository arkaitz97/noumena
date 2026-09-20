# YouTube API Compliance Audit — Application Pack

**Status:** DRAFT — ready to submit after legal sign-off
**Date:** 2026-09-17
**Companion:** [`platform-tos-compliance.md`](platform-tos-compliance.md)
**Form:** https://support.google.com/youtube/contact/yt_api_form

This document assembles everything needed to request a YouTube Data API **quota extension / API Compliance Audit** and accept the *Additional policies for derived metrics and data storage* amendment. Submitting it is the linchpin of the target operating model: it converts Noumena's derived-metric features from prohibited to permitted for an approved analytics use case.

> Do not submit until counsel has reviewed the answers below. Audit approval is discretionary and periodically re-audited.

---

## 1. What we are requesting

1. Acceptance of the derived-metrics amendment to the YouTube API Services Developer Policies.
2. A quota extension sufficient to sustain production comment ingestion.
3. Confirmation that Noumena's analytics use case qualifies (sentiment/NLP, categorization, scores, planning-stage suitability).

## 2. Form selections

| Field | Selection |
|---|---|
| Request type | Quota extension / compliance audit |
| Use case section | "Section 5: Use Cases, API Integration, and Feature Implementation" |
| Use case | "Analytics & Reporting" |
| Derived metrics | Apply for permission to create derived metrics and/or store statistical data |

## 3. Amendment acknowledgment (accept verbatim)

The form states the following amendment, which must be acknowledged:

> Sections III E.2.a and E.4.h are amended to allow you to create derived metrics for limited purposes on the condition that you clearly and prominently label these metrics as generated independently by you and not sourced directly from YouTube's API.
>
> Section III E.4.b, E.4.c, and E.4.d are amended by allowing you to store publicly available stats (e.g., views, likes, comment counts) for up to 36 calendar months. Derived metrics (e.g., custom scores using views, likes, etc.) that are based on retrieved data may also be stored for up to 36 calendar months. Other data (e.g., video titles, creator names, comment text) still must follow the 30-day refresh and deletion policy.

## 4. Application narrative (paste into the form)

Noumena is a comment-intelligence tool for YouTube creators. A creator connects their own channel via Google OAuth (`https://www.googleapis.com/auth/youtube.readonly`); Noumena ingests that channel's videos and comments and produces derived analytics for the creator's own use.

Data handling:
- Access is exclusively through creator-authorized OAuth. We do not read third-party channels, do not use public API keys, and do not scrape.
- Raw API Data (comment text, author display names, video titles/descriptions) is refreshed or deleted within 30 calendar days.
- Derived metrics (viewer sentiment, content categorization/tags, topic extraction, engagement scores) and statistical aggregates are retained up to 36 calendar months under the derived-metrics amendment.
- Every derived metric shown to a user is labelled: "Generated independently by Noumena; not sourced from the YouTube API."
- We do not infer protected attributes, audience demographics/composition, watch time, reach, or monetization status, and we do not make post-campaign brand-safety claims.

Requested metrics (per the acceptable-metrics list):
- Viewer Sentiment Analysis (NLP over comments).
- Content Categorization and Tagging.
- Custom Channel Scores and Ratios.
- Gamification and Leaderboards (creator's own channels).
- Brand Suitability & Safety Scoring — **Planning (Creator Insights)** only, for creator vetting/matchmaking by authorized brand/agency representatives.

Aggregation: any aggregation is presented only to the authorizing content owner or their expressly authorized representative; cross-creator views expose derived metrics only, never compiled raw API Data.

## 5. Evidence pack to attach / be ready to show

| Item | Location |
|---|---|
| Privacy policy (live URL) | _to be published — Phase 1_ |
| Terms of service (live URL) | _to be published — Phase 1_ |
| OAuth consent screen + scopes | Google Cloud Console |
| Screencast: OAuth grant + each scope's user-facing use | _to be recorded_ |
| Confirmation that competitor/public tracking is removed | `internal/api/channel_handlers.go`, `ChannelsView.vue` |
| 30-day refresh/delete job evidence | _to be built — Phase 2_ |
| Derived-metric labelling screenshots | _to be built — Phase 2_ |
| LLM subprocessor list + DPAs | OpenAI, Anthropic, OpenCode, TypeSafe |

## 6. Pre-submission checklist

- [ ] Legal sign-off on the narrative and open questions (see report §8).
- [ ] `connect/youtube/by-name` endpoint, resolver, competitor UI, and `YOUTUBE_API_KEY` ingestion path removed.
- [ ] Live Privacy Policy + ToS with YouTube-required links (YouTube ToS, Google Privacy Policy, Google security-settings revocation page, contact).
- [ ] `force-ssl` scope removed until a write feature ships.
- [ ] 30-day raw-data refresh/delete job implemented and tested.
- [ ] 30-day reauthorization reconfirmation + delete-on-revocation job implemented.
- [ ] Derived-metric labels present in every user-facing surface.
- [ ] LLM DPAs / no-train confirmations collected.
- [ ] Screencast recorded (consent flow + each scope's use).
- [ ] Derived-metric features remain flag-gated until approval.

## 7. Post-approval obligations

- Use the granted quota only for the approved analytics use case; re-submit the audit if the use case changes.
- Keep derived metrics labelled as independently generated.
- Maintain 30-day raw retention and 36-month derived retention.
- Expect periodic re-audits; keep the evidence pack current.
