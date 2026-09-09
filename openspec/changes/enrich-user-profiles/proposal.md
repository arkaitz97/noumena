# Proposal: Enrich User Profiles

## Why

Noumena currently knows how to authenticate, bill, and ingest data for a user, but knows very little about the user’s role, goals, content context, or definition of success. The existing account profile is effectively auth + billing + lifecycle state: email, verification, subscription fields, OAuth/channel relationships, digest email opt-in, and GDPR state `(source: internal/db/schema.sql:7-37, docs/api/user.md:25-46, internal/api/account_handlers.go:392-424)`.

The product has three distinct personas with different jobs-to-be-done: growing creators, agencies, and brand/sponsor teams `(source: PRD.md:30-50)`. Today the onboarding flow only guides YouTube channel connection; it does not capture whether a user wants content ideas, audience understanding, sponsorship evidence, moderation relief, or multi-client reporting `(source: frontend/src/views/auth/OnboardingView.vue:16-20, 184-236)`.

This gap weakens:

1. **Segmentation:** we cannot reliably separate creators from agencies or brand users for funnel, retention, and plan analysis.
2. **Personalization:** weekly digests and insights are generated without declared audience context or user priorities `(source: internal/llm/digest_llm.go:25-92)`.
3. **Activation learning:** we cannot tell which user types or goals complete onboarding, connect channels, open digests, or churn.
4. **Product targeting:** pricing and packaging are based on generic plan limits, not observed business context.

**Assumption:** richer declared context will improve relevant digests, onboarding completion, retention, and plan fit `(hypothesis)`. We have no repo evidence, support data, interviews, or profile-completion metrics yet. This proposal therefore minimizes user burden and ties every field to a decision.

## What Changes

Add a progressive user-profile capability:

- New `user_profiles` table, one row per user.
- `GET /api/user/profile` and `PATCH /api/user/profile`.
- Onboarding adds an "About your work" step before channel connection.
- Account settings adds a profile editor for later updates.
- Weekly digest generation can optionally use approved profile context behind a feature flag.
- GDPR export includes profile data.
- Account deletion removes profile data.
- UI telemetry records profile completion/skip without storing raw values.

### Profile fields

P0 fields answer: "Who is this user, what are they trying to do, and what audience are they serving?"

| Field | Values / shape | Required in onboarding | Enables |
|---|---|---:|---|
| `user_type` | `creator`, `agency`, `brand`, `other` | yes | persona segmentation, workspace path |
| `primary_goal` | `understand_audience`, `generate_content_ideas`, `improve_retention`, `win_sponsorships`, `reduce_toxicity`, `report_to_clients`, `other` | yes | onboarding promises, digest priority |
| `primary_platform` | `youtube`, `tiktok`, `instagram`, `other` | no | platform roadmap, channel setup |
| `content_topics` | JSON string array, max 10 items, max 40 chars each | no | insight relevance, topic grouping |
| `audience_size_bucket` | `under_1k`, `1k_10k`, `10k_50k`, `50k_500k`, `500k_1m`, `over_1m`, `prefer_not_to_say` | no | plan fit, creator maturity |
| `weekly_comment_volume_bucket` | `under_100`, `100_1k`, `1k_10k`, `10k_100k`, `over_100k`, `prefer_not_to_say` | no | quota/overage friction |
| `success_metric` | `views`, `watch_time`, `subscribers`, `engagement`, `sentiment`, `sponsor_deals`, `product_sales`, `client_reports`, `other` | no | activation and value measurement |

P1 fields answer: "How does this user work?" These should be optional during onboarding and editable in settings.

| Field | Values / shape | Enables |
|---|---|---|
| `role` | `owner`, `strategist`, `analyst`, `moderator`, `viewer`, `other` | multi-seat workflow design |
| `team_size_bucket` | `solo`, `2_5`, `6_20`, `21_plus`, `prefer_not_to_say` | workspace/plan positioning |
| `publishing_frequency_bucket` | `daily`, `several_weekly`, `weekly`, `biweekly`, `monthly`, `irregular`, `prefer_not_to_say` | digest cadence expectations |
| `insight_priorities` | JSON array of `questions`, `content_ideas`, `criticism`, `toxicity`, `collaboration`, `sentiment`, max 3 | digest and dashboard ordering |
| `current_workflow` | `native_tools`, `spreadsheets`, `moderation_tool`, `agency_dashboard`, `none`, `other` | competitive gaps and migration barriers |
| `biggest_pain` | `too_many_comments`, `missed_feedback`, `toxicity`, `reporting_time`, `unclear_content_plan`, `team_visibility`, `other` | opportunity validation |

P2 fields are later-stage enrichment, not initial onboarding scope:

| Field | Shape | Why deferred |
|---|---|---|
| `business_stage` | `hobby`, `side_project`, `full_time`, `agency`, `brand` | overlaps with `user_type` and `team_size_bucket` |
| `monetization_models` | JSON array: `ads`, `sponsorships`, `affiliate`, `products`, `memberships`, `services`, `donations`, `none`, `prefer_not_to_say` | sensitive; collect only when billing or sponsorship features use it |
| `digest_detail_level` | `summary`, `balanced`, `deep` | useful after personalization is proven valuable |
| `primary_audience_language` | BCP 47 tag | only needed with localization work |

### Explicitly not collected initially

- Exact revenue or MRR.
- Exact follower or subscriber counts.
- Date of birth, gender, ethnicity, religion, politics, health, sexual orientation.
- Precise location.
- Free-form biography unless a later feature uses it.

Rationale: Noumena can make useful product decisions from buckets and goals. Sensitive identifiers would expand GDPR, security, retention, and user-trust cost without proven value.

### Product chain

```
Outcome:
  Improve activation, retention, and plan fit for creator, agency, and brand users.

Opportunity:
  Declare minimum user/work context and use it to personalize and measure the product.

Solution:
  Progressive user profile + settings editor + optional digest context + telemetry.

Experiment/story:
  Show the profile step to a cohort, compare profile completion, channel connection, digest engagement, insight starring, and week-1 retention against users who skip.
```

## Capabilities

### New Capabilities

- `creator-profiles`: per-user declared context, validation, API access, export/deletion behavior, and telemetry.

### Modified Capabilities

- `frontend-dashboard`: onboarding profile step and account-settings profile editor.
- `weekly-digest`: optional profile-context input, gated by feature flag.
- `data-lifecycle`: GDPR export includes profile rows; deletion wipes profile rows.

## Impact

- **Backend:** new migration for `user_profiles` (next version after current v14), new handler(s) for `GET/PATCH /api/user/profile`, DB model/query methods, validation tests, rate-limit/access-control coverage through existing `/api` auth middleware `(source: cmd/api/main.go:504-544)`.
- **Frontend:** profile types, service wrapper, Pinia store or account-store extension, onboarding step, settings editor, tests `(source: frontend/src/services/account.ts, frontend/src/stores/account.ts, frontend/src/views/auth/OnboardingView.vue, frontend/src/views/settings/AccountView.vue)`.
- **Digest pipeline:** prompt builder receives optional sanitized profile context when `DIGEST_PROFILE_CONTEXT_ENABLED` is true `(source: internal/llm/digest_llm.go:22-92, internal/llm/digest_worker.go:207-298)`.
- **Lifecycle/export:** add `profile/profile.json` to GDPR bundle and include `user_profiles` in final wipe list `(source: internal/lifecycle/export_worker.go:289-321, internal/db/lifecycle.go:458-469)`.
- **Ops/config:** add feature flag to `.env.example`; no new external dependency.

## Open Questions

1. Do we require `user_type` and `primary_goal`, or make all profile questions skippable to protect activation?
2. Does selected profile context actually improve digest perceived value, or is it only useful for analytics?
3. Should agency users get a workspace-level profile separate from individual member profiles?
4. Which P1 fields are worth asking during onboarding versus waiting until the user has seen first insights?

## Dominant Risk

**Value risk:** users may complete a profile without perceiving better insights. Cheapest validation is the first experiment: collect profile answers for a cohort, manually personalize digest ordering or copy where possible, and measure digest engagement, starred insights, and skip rate before building deeper recommendation logic.
