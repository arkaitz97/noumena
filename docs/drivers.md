# Quality drivers and scenarios

Architecture drivers for Noumena, written as an ADD 3.0 driver sheet plus
six-part quality-attribute scenarios. Every measure below maps to a constant,
a test, or a CI job in this repository. Where no enforcement exists, the
scenario says `Gap`.

## 3.0 Driver sheet

| ID | Type | Driver | Measure |
|---|---|---|---|
| Q-1 | Quality: reliability | The insight pipeline survives an LLM provider outage. | Fallback provider takes over after 3 consecutive primary failures (`internal/llm/failover.go`, `LLMFailoverErrorThreshold: 3`). |
| Q-2 | Quality: security | A user sees only comments from channels they track or that a workspace they belong to has granted. | 0 rows and HTTP 403 for a non-member; enforced by one SQL predicate (`internal/db/comments_feed.go` `accessControlClause`). |
| Q-3 | Quality: correctness | A replayed Stripe webhook applies its side effects at most once. | Second delivery of the same `event.id` returns 200 with 0 subscription updates (`processed_stripe_events`, `internal/api/billing_handlers.go`). |
| Q-4 | Quality: privacy | An account deletion removes all personal data within the reversible window plus one worker tick. | Default window 720 h (`DELETION_REVERSIBLE_HOURS`, `internal/config/config.go`); the wipe touches 15 tables plus the user row (`internal/db/lifecycle.go`). |
| Q-5 | Quality: performance | The comment feed answers within its query budget. | Feed query context timeout 2 s, count query 1.5 s, max page 100 rows (`internal/config/config.go`). |
| Q-6 | Quality: modifiability | A new LLM provider is an adapter, not a rewrite. | One `LLMClient` interface; three implementations (`opencode`, `openai`, `anthropic`) selected by env. |
| C-1 | Constraint | Ingestion must not exceed the YouTube Data API daily unit budget. | Per-job ceilings: 100 videos, 25 thread pages, 100 reply fetches (`internal/config/config.go`). |
| C-2 | Constraint | One process owns the database. | SQLite via `modernc.org/sqlite`, single writer; horizontal scale-out is out of scope. |
| O-1 | Operational | Metered overage is invoiced without double-billing. | `comment_usage_logs` rows per user and period, cron at 08:10 UTC (`internal/billing/overage_invoicing.go`). |

## Quality scenarios

### S-1 Provider failover (Q-1)

| Part | Value |
|---|---|
| Source | LLM provider (primary) |
| Stimulus | Three consecutive classification requests fail |
| Environment | Normal operation, fallback provider configured |
| Artifact | `internal/llm/failover.go` wrapper around `Processor` |
| Response | Router flips permanently to the fallback; processing continues |
| Response measure | The 3rd error flips the route. Success resets the counter. Tests: `TestFailover_FlipsAfterConsecutiveThreshold`, `TestFailover_SuccessResetsCounter` (`internal/llm/failover_test.go`). |

### S-2 Cross-tenant read isolation (Q-2)

| Part | Value |
|---|---|
| Source | Authenticated user, member of no workspace holding the channel |
| Stimulus | `GET /api/comments/feed` or `GET /api/insights/sentiment?channel_id=<foreign>` |
| Environment | Two users, two tracked channels, no shared grant |
| Artifact | `BuildFeedQuery` access predicate and the sentiment handler's channel check |
| Response | Feed returns zero rows; sentiment returns HTTP 403 `not_tracked` |
| Response measure | Exactly 0 rows and status 403. Test: `TestSentiment_AccessControlIsolation` (`internal/api/insights_sentiment_test.go`). Grant paths: `TestGrantChannel_MemberSeesSharedFeed`, `TestGrantChannel_NonMemberDeniedFeed`, `TestGrantChannel_StarWithoutAccessDenied` (`internal/db/comments_feed_test.go`). CI job `fitness-tenant-isolation`. |

### S-3 Webhook replay (Q-3)

| Part | Value |
|---|---|
| Source | Stripe |
| Stimulus | Redelivery of an already-processed `event.id` |
| Environment | Webhook endpoint reachable, signature valid |
| Artifact | `BillingHandler.WebhookStripe` and `processed_stripe_events` |
| Response | Short-circuit before dispatch; no second subscription update |
| Response measure | HTTP 200 and 0 `ApplySubscriptionUpdate` calls. Test: `TestStripeWebhookDuplicateShortCircuits` (`internal/api/billing_test.go`). CI job `fitness-webhook-idempotency`. |

### S-4 Account erasure (Q-4)

| Part | Value |
|---|---|
| Source | Account owner |
| Stimulus | `DELETE /api/user/account` with matching email confirmation |
| Environment | Lifecycle workers enabled; export storage configured |
| Artifact | `AccountHandler.DeleteAccount`, `DeletionWorker.runFinalStage` |
| Response | Export bundle generated, channels detached or purged, user row hard-deleted, export directory removed |
| Response measure | Rows for the user in all 15 GDPR tables and in `users` equal 0; another user's stars and memberships survive. Test: `TestDeleteUserGDPRData_WipesAllTables_FKOn`, `TestPurgeChannelData_OrphanCleanup` (`internal/db/gdpr_lifecycle_test.go`). CI job `go` runs the whole package. |

### S-5 Feed latency budget (Q-5)

| Part | Value |
|---|---|
| Source | Dashboard user |
| Stimulus | `GET /api/comments/feed?limit=100` |
| Environment | 100k comments per channel |
| Artifact | `BuildFeedQuery` plus the FTS5 index and `idx_comments_channel_processed` |
| Response | Page returned or the request cancelled at the query deadline |
| Response measure | Query cancelled at 2000 ms; page limited to 100 rows (`FeedMaxLimit`). `Gap`: no load test asserts p95. The timeout and cap are enforced; latency is not measured in CI. |

### S-6 Ingestion replay and dedupe (Q-3, C-1)

| Part | Value |
|---|---|
| Source | Ingestion scheduler |
| Stimulus | A crash or retry after a partially consumed page |
| Environment | Backfill job running with a saved checkpoint |
| Artifact | `CommitCommentsAndCheckpoint`, `UNIQUE(platform_channel_id, platform_comment_id)` |
| Response | Comments upsert; the checkpoint advances in the same transaction |
| Response measure | Replaying the same batch leaves one row per `(platform_channel_id, platform_comment_id)` and one checkpoint row. `Gap`: no dedicated test asserts replay idempotency. The invariant is enforced by `UNIQUE(platform_channel_id, platform_comment_id)` and the `ON CONFLICT ... DO UPDATE` in `CommitCommentsAndCheckpoint` (`internal/db/comments.go`). |

## CI mapping

| CI job | Scenario enforced |
|---|---|
| `fitness-tenant-isolation` | S-2 cross-tenant read isolation |
| `fitness-webhook-idempotency` | S-3 webhook replay |
| `go` (`go test ./...`) | S-4 erasure, S-6 ingestion replay, S-1 failover |
| `frontend` | Dashboard rendering and unit contracts |

## Open gaps

- No load test proves S-5. Add one when the feed passes 10k rows per channel
  in a representative dataset.
- `Gap` in S-6: the replay invariant has no single named test. Extract one from
  the existing db suite when the ingestion checkpoint code changes next.
