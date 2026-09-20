# Platform TOS Compliance Review

**Status:** DRAFT — pending legal review
**Date:** 2026-09-17
**Owner:** engineering (compliance workstream)
**Scope:** Third-party platforms Noumena integrates with — shipped, planned, and deferred — assessed against their current terms and developer policies.
**Target operating model:** full product roadmap, conditional on YouTube API Compliance Audit approval. Fallback = B2C-only if the audit is denied.

> This document is an engineering compliance analysis, not legal advice. Every clause marked `(unverified)` must be confirmed by counsel before it is relied upon. Sources were retrieved 2026-09-17; platform policies change and must be re-checked before launch and at each annual re-audit.

---

## 1. Scope and method

### 1.1 Integrations in scope

| Platform | Status | Entry point |
|---|---|---|
| YouTube Data API v3 | **Shipped** | OAuth (`youtube.readonly`) + API-key public reads; ingestion |
| Google SSO | Shipped | separate OAuth client, `openid email profile` |
| OpenAI | Shipped (LLM primary/fallback) | comment classification, sentiment, digests |
| Anthropic | Shipped (LLM fallback) | comment classification, sentiment, digests |
| OpenCode gateway | Shipped (default LLM provider) | classification, digests |
| TypeSafe Jev | Planned (`use-jev-classification`) | classification |
| Stripe | Shipped | billing |
| SMTP / Resend | Shipped | transactional email |
| TikTok Research API | Planned (`add-tiktok-ingestion`) | ingestion |
| Instagram Graph API | Deferred (brand-expansion Phase 5) | own-account OAuth |
| X API | Deferred (brand-expansion Phase 5) | paid, cost-gated |
| B2B surfaces | Planned | `add-audience-demand-discovery`, `add-content-asset-tracking`, `add-sponsorship-campaign-intelligence` |

### 1.2 Source documents

| Ref | Document | URL | Retrieved |
|---|---|---|---|
| S1 | YouTube API Services — Developer Policies | https://developers.google.com/youtube/terms/developer-policies | 2026-09-17 |
| S2 | YouTube API Services Terms of Service | https://developers.google.com/youtube/terms/api-services-terms-of-service | 2026-09-17 |
| S3 | Additional policies for derived metrics and data storage | https://developers.google.com/youtube/terms/derived-metrics-policy | 2026-09-17 |
| S4 | Complying with YouTube's Developer Policies (guide) | https://developers.google.com/youtube/terms/developer-policies-guide | 2026-09-17 |
| S5 | YouTube API compliance / quota-extension form (amendment acknowledgment) | https://support.google.com/youtube/contact/yt_api_form | 2026-09-17 |
| S6 | Google API Services User Data Policy | https://developers.google.com/terms/api-services-user-data-policy | 2026-09-17 |
| S7 | Google restricted/sensitive scope verification | https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification | 2026-09-17 |
| S8 | TikTok Research Tools access & eligibility | https://developers.tiktok.com/products/research-api/ | 2026-09-17 |
| S9 | TikTok Research Tools Terms of Service | https://www.tiktok.com/legal/page/global/terms-of-service-research-api/en | 2026-09-17 |
| S10 | Instagram Platform — Overview | https://developers.facebook.com/docs/instagram-platform/overview | 2026-09-17 |
| S11 | Instagram Platform — App Review | https://developers.facebook.com/docs/instagram-platform/app-review/ | 2026-09-17 |
| S12 | X Developer Agreement / Developer Policy | https://developer.x.com (developer agreement & policy pages) | 2026-09-17 |

### 1.3 Confidence labels

- **verified** — clause text read verbatim from the source document during this review.
- **inferred** — conclusion drawn from verified clause text plus product behavior; needs counsel confirmation.
- **(unverified)** — could not be confirmed verbatim; treat as an open question.

---

## 2. Executive summary

1. **The product survives ToS scrutiny, conditionally.** YouTube's *Additional policies for derived metrics and data storage* (S3) explicitly permits the calculations Noumena performs — NLP sentiment analysis on comments, content categorization/tagging, custom channel scores, cross-channel leaderboards, and planning-stage brand-suitability scoring. The mechanism is an **API Compliance Audit** in which the developer accepts a published amendment to the Developer Policies (S5). Until that approval exists, creating derived metrics violates the base policy.
2. **Two unconditional losses.** (a) OAuth-free **competitor/public channel tracking** via `YOUTUBE_API_KEY` reads Non-Authorized Data, on which aggregation and derived metrics are prohibited; it must be removed. (b) **Raw comment text may not be retained beyond 30 days** on any channel, authorized or not; saved comments must become annotations + deep links rather than persisted comment bodies.
3. **Aggregation is amended for approved use cases.** The base "aggregate only within the same content owner" rule (S1 Data Aggregation) is among the clauses amended by the audit acknowledgment (S5). Cross-creator **derived** metrics for planning/vetting are expressly contemplated. Cross-creator **raw** comment compilation for a third party remains restricted and must be creator-authorized.
4. **Non-YouTube platforms each have a narrow legal lane.** TikTok Research API is academic-only and cannot be used commercially (S8, S9); Instagram requires creator OAuth + Advanced Access + Business Verification (S10, S11); X requires a commercial agreement and forbids redistributing hydrated content beyond tight limits (S12).
5. **Immediate hygiene gaps.** No published privacy policy / terms of service (`frontend/src/views/public/LandingView.vue:427-428` links are `href="#"`), violating YouTube III.A; no 30-day reauthorization reconfirmation or delete-on-external-revocation lifecycle.
6. **The hinge is discretionary approval.** The audit is granted at YouTube's discretion and periodically re-audited. Engineering must keep derived metrics feature-flagged until approval lands, and must remain able to fall back to B2C-only.

---

## 3. Target operating model — "full roadmap under audit"

| Data class | Retention | Legal basis |
|---|---|---|
| Authorization tokens | As long as needed, per consent | S1 III.E.4.a |
| OAuth-authorized raw API Data (comment text, author, titles, descriptions) | **≤30 days**, then refresh or delete | S1 III.E.4.b–d |
| Statistical data (views, likes, comment counts) | **≤36 months** after amendment approval | S3, S5 |
| Derived metrics (sentiment, category, topic, confidence, scores) | **≤36 months** after amendment approval | S3, S5 |
| Non-Authorized Data (public API-key reads) | **≤30 days**, no aggregation, no derived metrics | S1 III.E.4.d |

Every derived metric displayed to a user must be labelled as generated independently by Noumena and not sourced from YouTube (S5 amendment text).

---

## 4. Per-platform findings

### 4.1 YouTube (primary exposure)

#### F-YT-1 — Derived metrics require the audit amendment
Noumena computes sentiment, comment category, topic, calibrated confidence, toxicity/spam flags, engagement-health score, and actionable insights. S1 III.E.4.h prohibits creating "new or derived data or metrics" from API Data. S3 then carves out an approved path: NLP-based viewer sentiment analysis, content categorization/tagging, custom channel scores, gamification/leaderboards, and brand suitability/safety **planning** scoring are permitted for developers accepted through the API Compliance Audit, provided metrics are labelled independent (S5).

**Verdict:** blocking until approved; fully supported after. This is the linchpin of the whole compliance posture. See `youtube-derived-metrics-audit-application.md`.

#### F-YT-2 — Aggregation
S1 Data Aggregation: "Do not aggregate API Data except ... channels that are under the same content owner ... Such aggregated API Data must only be viewable by that content owner." The S4 guide repeats it: compile/aggregate authorized API data only viewable to the content owner or an authorized representative. S5's acknowledgment amends III.E.2.a for approved analytics use cases, and S3 permits cross-channel leaderboards and planning-stage suitability scores.

**Verdict:** cross-creator *derived* metrics permitted under the amendment; cross-creator *raw* comment compilation for a third-party brand remains restricted. `openspec/brand-expansion-roadmap.md:193-199` currently asserts creator opt-in alone satisfies the clause — that assertion is incomplete and must be corrected to cite the amendment approval.

#### F-YT-3 — Non-authorized public reads (competitor tracking)
`POST /api/channels/connect/youtube/by-name` (`internal/api/channel_handlers.go:430-472`) creates a tokenless channel and ingests via the server API key (`internal/ingestion/worker.go:298-303`, `internal/ingestion/youtube_client.go:274-278,957-966`), surfaced in the UI as "Competitor channel" / "Tracks the channel's public comments. No YouTube sign-in needed." (`frontend/src/views/settings/ChannelsView.vue:216,234`), routed at `cmd/api/main.go:483`. This is Non-Authorized Data: no derived metrics, no aggregation, ≤30-day storage.

**Verdict:** **unconditional violation.** Remove the endpoint, the resolver, the UI, and `YOUTUBE_API_KEY` handling from ingestion.

#### F-YT-4 — 30-day raw retention
`comments` stores `body` and `author_handle` indefinitely (`internal/db/schema.sql:215-222`); `user_comment_stars` persists references (`:250`); exports persist (`:313`). S1 III.E.4.b–d and S3 require comment text, creator names, titles, and descriptions to be refreshed/deleted within 30 days.

**Verdict:** requires the retention split (§6.1). Accepted as workable by product.

#### F-YT-5 — Privacy policy and required disclosures
S1 III.A requires every API Client to link YouTube's Terms of Service, notify users that it uses YouTube API Services, link the Google Privacy Policy, explain what API Data is accessed/stored/shared, link the Google security-settings revocation page, and provide a contact for complaints. The frontend has no live policy: `LandingView.vue:427-428` links are placeholders.

**Verdict:** **violation until fixed.** Blocker for OAuth verification.

#### F-YT-6 — Revocation and 30-day reconfirmation
S1 requires programmatic token revocation when a user revokes via the app or Google security settings, and deletion of associated API Data within 7 days (user-directed) / 30 days (revocation via Google page). It also requires reconfirming authorization at least every 30 days and deleting data whose token cannot be refreshed. Noumena revokes on account deletion (`internal/lifecycle/`) but has no periodic reconfirmation job.

**Verdict:** add a reconfirmation + delete job.

#### F-YT-7 — Independent value (RMF)
S1 III.C / S4 require independent value beyond re-displaying YouTube data, and prohibit charging for functions free on YouTube. An analytics-only product is defensible (NLP insight is independent value), but the write/action loop (reply drafting/posting, moderation) materially strengthens the position.

**Verdict:** monitor; prioritize the write path when feasible. `ai-reply-suggestions` painted door is the natural vehicle.

#### F-YT-8 — Scope minimization and API projects
S1 III.D: request only scopes currently used; one API project per API Client; do not shard quotas. `docs/ops/runbooks/youtube-oauth-setup.md` currently lists `youtube.force-ssl` as a configured scope — remove it until a write feature ships, and keep Google SSO on its separate client.

**Verdict:** hygiene fixes.

### 4.2 Google SSO / Google API Services User Data Policy

Sending comment text to OpenAI/Anthropic/OpenCode/TypeSafe is a **transfer** of Google user data to third parties. S6 Limited Use permits transfers only to provide/improve prominent user-facing features, with user consent, and prohibits allowing humans to read the data. Disclosure and consent for subprocessors are therefore required. `youtube.readonly` and `youtube.force-ssl` are **sensitive scopes** requiring OAuth verification before public launch (S7). Confirm zero-retention / no-training terms with each LLM provider.

**Verdict:** compliant with disclosure + consent + DPAs; verification is a launch gate. `(inferred)`

### 4.3 TikTok — planned change is not viable as written

S8: eligible applicants are academic institutions or not-for-profit research bodies, must be "independent of commercial interests," and the FAQ states creators/advertisers/commercial users are **not** eligible. S9 forbids any commercial purpose, sublicensing, redistribution, and requires 30-day refresh/delete. `openspec/changes/add-tiktok-ingestion/design.md` decision 1 chooses the Research API as the only source.

**Verdict:** **violation if built as proposed.** Re-plan to commercial APIs (Login Kit / Display API with creator OAuth; TikTok Marketing/partner program). Comment availability under the commercial APIs must be confirmed with TikTok `(unverified)`.

### 4.4 Instagram / Meta — deferred; legal lane exists

S10: comments are readable only on media the authorizing professional account owns; serving accounts you do not manage requires Advanced Access. S11: Advanced Access requires App Review and **Business Verification**. Reading third-party public comments is not offered. The roadmap's own-account OAuth model (Phase 5) is compatible.

**Verdict:** compliant with creator authorization + Advanced Access + Business Verification. Long approval lead time — start early.

### 4.5 X — deferred; heavy restrictions

S12: commercial use requires a commercial designation/license; redistribution of hydrated content is limited to ~1.5M Post IDs per entity per 30 days and ~500 hydrated objects per user per day via non-automated means; using X content to train foundation models is prohibited; creating a substitute product is prohibited. Noumena's CSV export of comment text would be redistribution.

**Verdict:** keep deferred; if built, licensed commercial tier + export limited to IDs or per-user non-automated retrieval. `(unverified)` exact current limits.

### 4.6 LLM providers (OpenAI / Anthropic / OpenCode / TypeSafe Jev)

No platform-specific ToS conflict with the classification use case, subject to: a Data Processing Agreement, a "not used to train" guarantee, disclosure to users, and a lawful basis under GDPR. TypeSafe Jev is early-access; its terms are `(unverified)` and must be reviewed before enabling `JEV_CLASSIFICATION_ENABLED`.

### 4.7 Stripe and email

Stripe: standard services agreement; no restricted-business conflict. Email (SMTP/Resend): consent-based, honours `digest_email_opt_in`; ensure CAN-SPAM/GDPR unsubscribe is present in every digest. No material issues found. `(inferred)`

---

## 5. Citation matrix

| # | Platform | Clause (ref + short text) | Source | Our behavior | Evidence | Risk | Remediation |
|---|---|---|---|---|---|---|---|
| YT-01 | YouTube | III.E.4.h — must not create new or derived data or metrics from API Data | S1, S3, S5 | sentiment, category, topic, confidence, toxicity, scores | `internal/llm/`; `openspec/changes/use-jev-classification/` | **High** | Accept amendment via compliance audit; label metrics independently generated |
| YT-02 | YouTube | Data Aggregation (III.E.2.a) — aggregate only same content owner, viewable only by owner | S1, S4, S5 | cross-channel trends; planned cross-creator comparison | `openspec/brand-expansion-roadmap.md:193-199`; `add-sponsorship-campaign-intelligence/proposal.md` | **High** | Rely on III.E.2.a amendment; raw cross-creator compilation only via creator grant |
| YT-03 | YouTube | III.E.4.b–d — comment text/titles/names not in indefinite list → ≤30-day refresh/delete | S1, S3 | comments stored indefinitely | `internal/db/schema.sql:215-222` | **High** | Retention split (§6.1) |
| YT-04 | YouTube | III.E.4.d — Non-Authorized Data ≤30d, no aggregation, no derived metrics | S1 | competitor/public tracking via API key | `internal/api/channel_handlers.go:430-472`; `internal/ingestion/worker.go:298-303`; `youtube_client.go:957-966`; `ChannelsView.vue:216,234` | **Critical** | Remove feature + `YOUTUBE_API_KEY` ingestion path |
| YT-05 | YouTube | III.A — privacy policy must link YouTube ToS, Google Privacy Policy, revocation page; disclose data use | S1 | no live policy; `href="#"` | `LandingView.vue:427-428` | **High** | Publish policy + ToS; require acceptance |
| YT-06 | YouTube | III.D — revoke on request; delete within 7/30 days; reconfirm authorization every 30 days | S1 | revoke on account deletion only; no reconfirm job | `internal/lifecycle/` | **Medium** | 30-day reconfirm + delete job |
| YT-07 | YouTube | III.C / RMF — independent value; no charging for free YouTube functions | S1, S4 | analytics-only product | `ai-reply-suggestions/proposal.md` | **Low** | Ship write/action loop; keep value independent |
| YT-08 | YouTube | III.D — minimal scopes; one API project; no sharding | S1 | `youtube.force-ssl` listed | `docs/ops/runbooks/youtube-oauth-setup.md` | **Low** | Drop force-ssl until write feature ships |
| GOO-01 | Google | Limited Use — third-party transfers only for prominent features with consent; no human read | S6 | comment text sent to LLM providers | `internal/llm/`; `.env.example` | **Medium** | DPA + no-train + user disclosure/consent |
| GOO-02 | Google | Sensitive scopes require OAuth verification | S7 | `youtube.readonly` used | `internal/oauth/youtube.go:21` | **Medium** | Apply for verification as a launch gate |
| TT-01 | TikTok | Research Tools are academic/non-profit only; no commercial use | S8 | planned Research API ingestion | `openspec/changes/add-tiktok-ingestion/design.md` | **Critical** | Re-plan to commercial OAuth APIs or drop |
| TT-02 | TikTok | Research ToS — no sublicense/redistribution, 30-day refresh/delete | S9 | planned commercial product use | same | **Critical** | same |
| IG-01 | Meta | Comments readable only on owned media; Advanced Access + Business Verification for third-party accounts | S10, S11 | deferred own-account OAuth | `brand-expansion-roadmap.md` Phase 5 | **Medium** | Creator OAuth + App Review + Business Verification |
| X-01 | X | Commercial use requires commercial designation; no substitute product | S12 | deferred paid ingestion | `brand-expansion-roadmap.md:38` | **Medium** | Licensed commercial tier |
| X-02 | X | Redistribution limited to Post IDs; ≤1.5M IDs/30d; ≤500 objects/user/day non-automated | S12 | CSV export of text (future) | `docs/help/csv-export-schema.md` (future X) | **Medium** | Limit X exports; no bulk text redistribution |
| LLM-01 | LLM providers | Commercial terms + DPA; no training on inputs | provider terms | comment text processed | `internal/llm/openai.go`, `anthropic.go`, `opencode.go` | **Low** | Confirm DPA/no-train for every provider incl. Jev |
| STR-01 | Stripe | Services agreement; restricted businesses | Stripe terms | subscriptions | `internal/billing/` | **Low** | None identified |
| EML-01 | Email | Consent + anti-spam (CAN-SPAM/GDPR) | provider terms | digests | `internal/email/` | **Low** | Keep opt-in + unsubscribe in every digest |

---

## 6. Solution architecture

### 6.1 Retention split

Separate raw API Data from derived metrics so the 30-day rule and the 36-month allowance apply to the correct classes.

- **Raw layer** (`comments.body`, `author_handle`, asset `title`/`description`): TTL ≤30 days. A refresh/delete job re-fetches where the source still exists and deletes otherwise. Exports expire ≤30 days and are not archived.
- **Derived layer** (category, sentiment, confidence, topic, toxicity/spam, scores, aggregates): persisted up to 36 months, labelled independent of YouTube.
- **Saved comments**: store the user's annotation + a deep link to the YouTube comment; do not persist the comment body beyond the raw TTL. Re-render from the raw layer while it exists.
- **Consent/grant records**: retained per consent lifecycle, not as API Data.

### 6.2 Access model

- Creator OAuth only. No public API-key reads.
- Brand/agency access only via a creator-issued `channel_grants` to a workspace the creator authorized, or the creator joining that workspace.
- Cross-creator surfaces expose **derived metrics** only; raw comment evidence is shown per creator to that creator's authorized representative.

### 6.3 Labelling and disclaimers

- Every derived metric carries: "Generated independently by Noumena; not sourced from the YouTube API."
- Financial projections (if any) carry a "third-party estimate, not approved by Google" disclaimer.
- No protected-attribute, demographic, audience-composition, watch-time, reach, or monetization-status inference.
- "Brand safety/suitability" is available for **planning/vetting** only; no post-campaign safety claims or impression-level safety reports.

### 6.4 Audit

Apply through S5 selecting *Section 5: Use Cases, API Integration, and Feature Implementation → Analytics & Reporting* and accept the derived-metrics amendment. Derived-metrics features stay behind flags (`JEV_CLASSIFICATION_ENABLED`, `DEMAND_DISCOVERY_API_ENABLED`, `CAMPAIGN_API_ENABLED`) until approval. Details in `youtube-derived-metrics-audit-application.md`.

### 6.5 Brand-facing third-party intelligence — the only lawful routes

Unconsented third-party tracking is out of scope and cannot be reinstated by any approval. YouTube public comments read with an API key are Non-Authorized Data (S1 III.E.4.d): ≤30-day storage, no aggregation, no derived metrics. The derived-metrics amendment applies only to **authorized** data, so it does not legalize derived intelligence over third-party channels. Instagram and TikTok do not expose third-party public comments at all. X is the sole platform where third-party reads are permitted, on a paid commercial tier within redistribution limits.

Lawful ways to give brands third-party creator intelligence:

| Route | Mechanism | Platform | Notes |
|---|---|---|---|
| Creator-authorized opt-in | Creator connects via OAuth and grants the brand/agency workspace access as an authorized representative | YouTube, Instagram, X | Core model. Cross-creator **derived** views permitted under the amendment; raw evidence only per authorizing creator |
| Content-owner / MCN | All channels under one content owner; aggregation expressly allowed | YouTube | No per-creator consent needed; sell to networks/media companies |
| Licensed data provider | Buy data with redistribution/derived rights + contractual warranties | All | Verify the vendor is not scraping or reselling API Data against ToS |
| Paid X commercial tier | Third-party reads + analytics under license | X | Redistribution limited to Post IDs / capped hydrated objects |

Reframe: the brand-facing product is a **consent-based discovery surface over opted-in creators**, not unconsented tracking. Cross-creator derived matchmaking/suitability is permitted under the amendment; unconsented per-channel comment intelligence is not buildable on any platform. YouTube BrandConnect is an additional partnership/distribution channel.

---

## 7. Remediation plan

| Phase | Work | Gate |
|---|---|---|
| 0 | This report + audit application; external legal read; start OAuth sensitive-scope verification | Legal confirms amendment scope; verification submitted |
| 1 (stop-bleeding) | Remove `connect/youtube/by-name`, resolver, competitor UI, `YOUTUBE_API_KEY` ingestion path; publish Privacy Policy + ToS with YouTube-required links; audit unregistered derived-metric paths | No non-authorized reads remain; policies live |
| 2 (architecture) | Retention split + raw TTL job; saved-comments change; 30-day reconfirm + delete-on-revocation job; derived-metric labelling; drop unused `force-ssl` | 30-day lifecycle test passes; labels present |
| 3 (proposals) | Rewrite `add-tiktok-ingestion` to commercial OAuth APIs; annotate brand-expansion changes with planning/measurement guardrails; correct `brand-expansion-roadmap.md:193-199` | `openspec validate --strict` passes |
| 4 (launch) | Enable derived metrics post-approval; run B2B pilot (Planning-only) | Audit approved; pilot reviewed |

Fallback: if the audit is denied, keep the B2C-only model (no cross-creator surfaces) and ship only S4 "acceptable metrics" (simple arithmetic on raw API Data).

---

## 8. Open questions for counsel

1. Does the III.E.2.a amendment permit a third-party agency to view **cross-creator derived** metrics, or only the aggregate ranking/list?
2. Does "Planning (Creator Insights)" cover a live shortlist product, or only one-off vetting?
3. Does processing by a third-party LLM count as a **transfer** requiring per-user affirmative consent under the Google User Data Policy Limited Use rules?
4. Exact X rates/limits and whether comment text in a paid customer report is "redistribution."
5. TikTok: do the commercial Display/Login APIs expose comments on an authorized creator's own videos?
6. Whether refreshing raw data is acceptable in lieu of deleting it when the source has changed or been removed.
7. Re-audit cadence and what evidence YouTube expects for the derived-metrics labels.
8. TikTok: does any commercial API (Business/Marketing, not Display) expose organic comment reply/moderation for an authorized creator's own videos?
9. Meta/Instagram: exact retention/caching rules for comments and media metadata held by a multi-tenant tool (see Annex A `❓` cells).

---

## 9. Appendix — verbatim excerpts

**S3, acceptable metrics (abridged):** "Custom Channel Scores and Ratios"; "Content Categorization and Tagging"; "Viewer Sentiment Analysis — You may estimate viewer satisfaction or dissatisfaction (sentiment) using aggregate data, such as like/dislike ratios or comment analysis"; "Gamification and Leaderboards"; "Brand Suitability & Safety Scoring ... Planning (Creator Insights): You may use API Data metadata to assess channels and videos for both 'Brand Suitability' ... and 'Brand Safety' for the purpose of influencer marketing, vetting, and matchmaking."

**S3, data storage:** "you may store some statistical data for a duration beyond 30 days ... metrics ... for up to 36 calendar months. Derived metrics (such as sentiment analysis) ... may also be stored for up to 36 calendar months. Other data (such as video titles, creator names, descriptions, and comment text) must still follow the 30-day refresh and deletion policy."

**S5, amendment:** "Sections III E.2.a and E.4.h are amended to allow you to create derived metrics for limited purposes on the condition that you clearly and prominently label these metrics as generated independently by you and not sourced directly from YouTube's API."

**S1, aggregation:** "Do not aggregate API Data except that you may only aggregate API Data relating to YouTube channels that are under the same content owner ... Such aggregated API Data must only be viewable by that content owner."

**S8, eligibility:** "Be independent of commercial interests and able to conduct research on a not-for-profit or non-commercial basis." FAQ: "I am a creator, advertiser, or commercial user. Am I eligible ...? No."

---

## Annex A — Feature × platform matrix

### A.1 Legend

| Symbol | Meaning |
|---|---|
| ✅ | Permitted now, standard conditions (creator OAuth, consent, retention) |
| 🟡 | Permitted **only after** platform approval/amendment |
| 🟠 | Permitted but restricted (retention / redistribution / cost) |
| ⛔ | Prohibited — no approval path |
| 🚫 | Not available — platform does not expose the data / use case ineligible |
| ❓ | Unverified — needs confirmation (counsel or platform) |

"Own" = the creator/business account that authorized the app. "Third-party" = any account the app has not been authorized for.

### A.2 Feature eligibility

| # | Feature | YouTube | Instagram | TikTok | X |
|---|---|---|---|---|---|
| 1 | Read own comments | ✅ OAuth, raw ≤30d | ✅ `..._manage_comments` + Advanced Access + Business Verification (multi-tenant) | 🚫 Display API exposes profile+videos only, not comments ❓ | ✅ paid API |
| 2 | Read third-party public comments | ⛔ non-authorized data: no derived metrics/aggregation | ⛔ only media the account owns | ⛔ Research API commercial-ineligible | 🟠 paid, but redistribution limits |
| 3 | Raw comment display/feed | ✅ authorized, ≤30d | ✅ authorized | 🚫 | 🟠 hydrated content limits (≤500/user/day, non-automated) |
| 4 | Saved/starred raw comments | ⛔ body >30d → annotations + link | 🟠 retention rules ❓ | 🚫 | 🟠 |
| 5 | Asset (video/post) tracking + metrics | ✅ stats; 36mo after amendment, else ≤30d | ✅ Graph media insights | ✅ `video.list` (own videos) | ✅ paid |
| 6 | Raw export (CSV/GDPR) | 🟠 ≤30d, no durable archive | 🟠 retention ❓ | 🟠 | 🟠 IDs-only redistribution by default |
| 7 | Category / topic classification | 🟡 amendment (Content Categorization) | ✅ own | 🚫 no data | ✅ commercial license |
| 8 | Sentiment analysis | 🟡 amendment (Viewer Sentiment Analysis) | ✅ own | 🚫 | ✅ license |
| 9 | Toxicity / spam flag | 🟡 amendment | ✅ own | 🚫 | ✅ |
| 10 | Actionable insights (questions/ideas) | 🟡 amendment | ✅ own | 🚫 | ✅ |
| 11 | Weekly digest (LLM summary) | 🟡 amendment | ✅ own | 🚫 | ✅ |
| 12 | Sponsored-content detection | 🟡 amendment (categorization) | ✅ branded-content tags / metadata scan ❓ | 🚫 | ✅ metadata |
| 13 | Custom scores / leaderboards | 🟡 amendment (Custom Scores, Gamification) | ✅ own ❓ | 🚫 | ✅ license |
| 14 | Cross-creator comparison | 🟡 amendment (Leaderboards) | 🟡 only across authorized creators | 🚫 | 🟠 license limits |
| 15 | Audience demand discovery | 🟡 amendment; raw evidence only per authorized creator | 🟡 authorized creators only | 🚫 | 🟠 |
| 16 | Sponsorship campaign intelligence | 🟡 planning allowed; ⛔ post-campaign "brand safety" measurement claims | 🟡 authorized | 🚫 | 🟠 |
| 17 | Brand suitability / safety (pre-campaign planning) | 🟡 amendment — Planning explicitly allowed | 🟡 authorized | 🚫 | 🟠 |
| 18 | AI reply drafting | ✅ draft is local; posting needs `force-ssl` | ✅ draft / post ❓ | 🚫 organic comment API ❓ | ✅ paid |
| 19 | Reply posting / comment moderation | ✅ `force-ssl` + RMF | ✅ `..._manage_comments` (reply/hide/delete) | ❓ not in Display API | ✅ paid write |
| 20 | Publish content | ✅ `force-ssl` | ✅ content publish | ✅ Content Posting API | ✅ paid write |

### A.3 Permission mechanisms

| Platform | What unlocks the 🟡/🟠 cells | Difficulty | Can we get it? |
|---|---|---|---|
| YouTube | Accept derived-metrics amendment via API Compliance Audit (Analytics & Reporting use case) | Medium (discretionary, slow: weeks–months) | **Likely yes** — the use case is named in the allowed metrics; remediate violations first |
| YouTube (OAuth) | Sensitive-scope verification for `youtube.readonly` | Medium (slow: 6–8+ wks real-world) | Yes — standard; requires policy + demo video |
| Instagram | Advanced Access + App Review + Meta Business Verification + Tech Provider verification | Medium-Hard | Yes, with preparation; Verification must precede App Review |
| TikTok | Commercial comment access | **No path** | **No** — Display API lacks comments; Research API commercial-ineligible. Licensed third-party providers only |
| X | Commercial license | Easy (costly) | Yes — pay + honour redistribution limits |

### A.4 One-page summary

| Platform | Verdict | What survives | What is lost | Gate |
|---|---|---|---|---|
| YouTube | Conditional | All features for authorized channels (1, 3–20) | Third-party public reads; raw retention >30d | Derived-metrics amendment + scope verification |
| Instagram | Conditional | All features for authorized accounts | Third-party public reads | Advanced Access + App Review + Business/Tech Provider verification |
| TikTok | Majority lost | Video/metadata (5, 20) | All comment features (1–19) | None — no commercial comment API |
| X | Cost-gated | All features technically | Unrestricted redistribution | Paid commercial license + redistribution limits |

Bottom line: **YouTube = one discretionary amendment away; Instagram = verification away; TikTok comments = unavailable; X = a budget decision.**
