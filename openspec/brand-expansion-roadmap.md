# Noumena Brand-Expansion Roadmap — Audience Intelligence for Creator–Brand Partnerships

**Status:** DRAFT | **Owner:** product-owner analysis (2026-09-17)
**Scope:** Expand Noumena from a creator-facing comment-intelligence tool into a B2B offering for marketing agencies and brands.
**Companion changes:** `add-audience-demand-discovery`, `add-content-asset-tracking`, `add-sponsorship-campaign-intelligence`.
**Compliance posture:** creator-opt-in only (authorized-representative model). No ingestion of third-party data by public scraping or public API keys.

> **Naming note.** The existing changes `add-campaign-attribution-tracking` and `add-campaign-metrics-dashboard` refer to Noumena's **own** 14-day marketing campaign (ACC/TTFI funnel). They are unrelated to this roadmap. This roadmap uses "sponsorship campaign" and "demand discovery" to avoid collision.

---

## 1. Positioning

**"Audience intelligence for creator–brand partnerships."** Noumena does not compete with influencer management platforms (CreatorIQ, GRIN, Aspire, Traackr) on roster, deals, contracts, or payments. It competes on the **comment-level layer** those platforms do not have: what a sponsored audience actually asked, objected to, loved, and wanted next — plus which creators' audiences already demonstrate unmet demand for a brand's category.

Traceability chain for every phase: **outcome → opportunity → solution → experiment**, with a named dominant risk and cheapest de-risking test (`product-owner` verification gate).

---

## 2. Why now — research summary

### Market map

| Category | Representative tools | What they sell | Gap Noumena can own |
|---|---|---|---|
| Influencer management | CreatorIQ, GRIN, Aspire, Traackr, Upfluence | Creator DB, CRM, deals, payments, campaign workflow | Comment-level creative + audience demand; not roster ops |
| Campaign measurement | InfluenceKit, trackio, Sheer, Influencer Army | View/engagement/spend attribution, white-label reports | Sentiment, objections, questions on sponsored assets |
| Social listening | Brand24, Hootsuite/Lumen, Octolens, Brandwatch | Brand mentions, share of voice, web sentiment | Creator-audience signals, not web mentions |
| Comment-intent / demand (emerging) | CrowdCore, HeadComments, CommentSignal, Upriver | "Find buyers / creator fit from comments" | Sparse incumbents, no clear leader — contested niche |

### Platform reality (decisive constraint)

| Platform | Third-party public comment reads | Cost | Decision |
|---|---|---|---|
| YouTube | Yes, `commentThreads.list` by `videoId` with API key | ~1 quota unit / 100 comments | **Build here first** |
| TikTok | **No** — Research API bars commercial users ("creators, advertisers, commercial users not eligible") | n/a | Own-account OAuth only; re-plan `add-tiktok-ingestion` |
| Instagram | **No** — API reads comments only on media the authorizing business/creator account owns | n/a | Creator connects account; brand granted access |
| X | Paid per read (~$0.005/post), 2–3M reads/mo cap, full archive gated | High | Defer; opt-in and cost-gated |

### Legal constraint (YouTube Developer Policies)

Directly relevant clauses:
- Aggregated authorized API data may be viewable **only** to the content/channel owner or their authorized representatives.
- Do not create "new or derived data or metrics" from API Data; no "custom scores… [for] brand suitability."
- Derived-metrics/storage use cases require an audited quota-extension request.

**Consequence:** the "brand tracks any creator's public content" model is a viability risk, not merely a technical one. The compliant model is **creator connects (OAuth) → brand/agency becomes an authorized representative via workspace grant**. This is exactly what Noumena's `platform_channels` + `user_channel_tracking` + `channel_grants` + `workspaces` model already implements.

---

## 3. Opportunity scoring (ODI)

`Opportunity score = Importance + max(Importance − Satisfaction, 0)`, 1–10 scales.

| # | Desired outcome (job to be done) | Imp | Sat | Score | Verdict |
|---|---|---|---|---|---|
| 1 | Find creators whose audience already asks for my product category | 9 | 3 | **15** | Major underserved — lead bet |
| 2 | Prove a sponsored campaign drove real audience interest/sentiment | 8 | 4 | **12** | Significant |
| 3 | Vet a creator's audience quality/brand-safety before signing | 8 | 5 | 11 | Significant |
| 4 | Extract objections/questions from campaign comments to steer creative | 7 | 3 | 11 | Significant |
| 5 | Produce client-ready campaign reports without spreadsheets | 7 | 4 | 10 | Significant |
| 6 | Continuous brand-mention sentiment monitoring | 6 | 5 | 7 | Monitor |
| 7 | Manage deals/contracts/payments | 6 | 7 | **6** | **Do not build** — incumbent-dominated |

All inputs are `(hypothesis)` — no agency interviews or usage analytics yet. Phase 0 exists to replace assumption with evidence.

**Dominant risk across the roadmap = viability/compliance** (YouTube ToS on third-party aggregation and derived metrics) plus unproven agency willingness to pay. Buildability is comparatively low-risk because the classification, corpus, workspace, and grant primitives already exist.

---

## 4. Portfolio balance

Phases 1–3 are all **new features** on top of a growth-stage product. The existing RUF guidance for a growth-stage product (≈30% new features / 20% usability / 30% reliability / 20% growth) SHALL be preserved: this roadmap SHALL NOT consume the entire roadmap capacity. The B2C engine remains the primary business. `(hypothesis)`

Every new B2B table SHALL be included in GDPR export and hard-delete in the same change that introduces it, and SHALL be covered by a `storage-layer` migration test (`storage-layer` is the repo's schema capability).

---

## 5. Phases

### Phase 0 — Validate (no code)

**Goal:** replace the Phase-3 ODI `(hypothesis)` inputs with evidence before committing engineering.

- Run 5 structured interviews: 3 marketing agencies (2–20 clients) + 2 brand-side influencer/partnerships leads.
- Produce one **concierge demand report** by hand for a real product across a hand-picked channel set, using the existing dashboard + CSV export, and present it back.
- Legal read of the YouTube Developer Policies clauses in §2 against the creator-opt-in design; record the conclusion and any quota-extension need.
- Define kill criteria before starting (see §7).

**Exit gate:** ≥3 of 5 interviewees say a comment-level demand/campaign insight would change a decision they would pay for; legal confirms creator-opt-in aggregation is permitted or names the exact remediation.
**If the gate fails:** stop the roadmap; do not author further B2B changes.

---

### Phase 1 — Change A: `add-audience-demand-discovery`

**Outcome:** a brand or agency can ask "which creators' audiences want what we sell, and why?" and get a ranked, evidence-backed shortlist from the opted-in corpus.
**Opportunity:** #1 (score 15), #3 (score 11).
**Solution:** demand-watch definitions + corpus search + demand extraction + creator ranking + evidence export.
**Experiment/story:** the feature itself is the experiment; success metric = ≥1 shortlist exported and acted on by ≥1 pilot agency within 30 days.

**Compliance spine:** new `channel_discovery_consent` per tracked channel, default **off**. Only opted-in channels appear in discovery. The corpus is a network-effect asset: each creator who opts in makes discovery better.

**Depends on:** nothing (runs on the existing corpus + consent).
**Exit gate:** discovery ranks opted-in channels by demand with evidence excerpts; non-opted-in channels are excluded; consent respected in export/delete; `openspec validate --strict` passes.

---

### Phase 2 — Change B: `add-content-asset-tracking`

**Outcome:** the system has a first-class representation of a video/post and its engagement metrics, which campaign intelligence requires.
**Opportunity:** prerequisite for #2, #4, #5.
**Solution:** `content_assets` entity (per channel: platform, platform asset id, title, url, published-at, views/likes/comment-count), sponsored-content detection (`#ad`, `#sponsored`, paid-partnership, brand mention), metrics-refresh worker, asset APIs.
**Depends on:** `platform_channels` only.
**Exit gate:** assets populate for connected channels; `is_sponsored` correct on fixtures; metrics refresh idempotent.

---

### Phase 3 — Change C: `add-sponsorship-campaign-intelligence`

**Outcome:** an agency or brand can judge a sponsored campaign by audience response and compare creators, then export a client-ready report.
**Opportunity:** #2 (12), #4 (11), #5 (10).
**Solution:** `campaigns` + `campaign_assets`; campaign-scoped aggregates (sentiment trend, category mix, questions, objections, top topics, toxicity/brand-safety); cross-creator comparison; report export; brand/agency packaging on the existing Agency tier.
**Depends on:** Change B (assets), Change A (optional — demand context enriches creator comparison).
**Exit gate:** campaign dashboard renders; ≥2 creators comparable; export works; `openspec validate --strict` passes.

---

### Phase 4 — Deferred: client reporting surface

**Outcome:** external stakeholders consume results without a Noumena seat.
**Solution:** external read-only `client` role scoped to a single workspace; white-label report export (PDF); shareable, time-limited client link (reuse signed-URL pattern from data export).
**Depends on:** Phase 3. Not authored until Phases 1–3 are validated in production.

---

### Phase 5 — Deferred: multi-platform creator-opt-in

**Outcome:** campaigns span Instagram/TikTok/X via creator opt-in.
**Solution:** own-account OAuth ingestion for Instagram (Instagram Platform API, creator authorizes), TikTok (Display API, creator authorizes), X (opt-in, cost-gated). Re-plan `add-tiktok-ingestion`, whose Research API premise is invalid for commercial use.
**Depends on:** Phase 4 grants + platform abstraction work. Not authored until Phases 1–3 are validated.

---

## 6. Dependency matrix

| Phase | Change | Hard dependency | Can parallelize with |
|---|---|---|---|
| 0 | validation (no code) | — | — |
| 1 | `add-audience-demand-discovery` | — | Phase 0 result |
| 2 | `add-content-asset-tracking` | — | Change A |
| 3 | `add-sponsorship-campaign-intelligence` | Change B (assets); Change A optional | — |
| 4 | client reporting surface | Phase 3 | multi-platform |
| 5 | multi-platform opt-in | Phase 4 grants + platform abstraction | client reporting |

Critical path: **0 → 1 → 3** (Change B can run in parallel with Change A).

---

## 7. Kill criteria

| Bet | Kill criterion | Review point |
|---|---|---|
| Audience demand discovery | Fewer than 1 pilot agency exports and acts on a shortlist within 30 days of pilot start; or <3/5 Phase-0 interviews validate WTP | 30 days after Phase 1 pilot |
| Sponsorship campaign intelligence | Pilot brands do not open the campaign dashboard more than once per campaign; or comment-level signals do not change a creative decision | End of first pilot campaign |
| Whole B2B expansion | Phase-0 gate fails, or legal does not clear creator-opt-in aggregation | Phase 0 exit |

Sunk cost SHALL NOT decide continuation; each gate is evaluated on the named metric alone.

---

## 8. Pricing hypothesis `(hypothesis)`

| Plan | Target | Monthly | Rationale |
|---|---|---|---|
| Brand (new) | Brand-side manager, 1 campaign | ≤ $299 | Sits below GRIN Lite ($399) and entry influencer suites (~$2.3k–$2.5k) while above creator plans |
| Agency | Agency, multi-client | $499–$999 | Between GRIN Growth ($500–$1,149) and enterprise quote-only suites; leverages existing Agency tier (20 seats) |

Comment-processing quota remains the cost governor. Campaign and brand quotas SHALL be expressed in the billing spec before Phase 3 ships, and overage pricing SHALL remain comment-count based (consistent with the existing billing spec).

---

## 9. Cross-cutting requirements

- **GDPR/lifecycle:** every new table cascades on user deletion and is included in the data-export bundle (`data-lifecycle` spec).
- **Feature flags:** new surfaces gated by runtime flags (`DEMAND_DISCOVERY_API_ENABLED`, `CAMPAIGN_API_ENABLED`) following the existing env-flag pattern.
- **Observability:** new endpoints emit OTel counters with no user-supplied attribute values (cardinality firewall).
- **Testing:** each migration has a `storage-layer` test; each handler has happy-path, authz, and invalid-input tests.
- **Frontend:** new views follow the existing Vue 3 + Pinia + Tailwind patterns and pass the existing frontend tests/lint.

---

## 10. Compliance model (creator-opt-in)

1. A creator connects a platform channel via OAuth (existing flow) and explicitly opts the channel into discovery (`channel_discovery_consent`).
2. A brand or agency is granted access to a channel only through `channel_grants` to a workspace the creator authorized, or by the creator joining that workspace.
3. Aggregated campaign/discovery views are therefore always visible to an authorized representative of the channel owner — satisfying the YouTube "authorized representatives" clause.
4. No public API-key reads of third-party content, no scraping, no derived brand-suitability scores on non-opted-in channels.
5. Any derived metrics or long-term storage required by YouTube's derived-metrics policy is handled as a named task in the owning change, not silently.
