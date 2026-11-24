# 0001. Share one comment store and scope access with tracking rows and workspace grants

## Status

Accepted. 2026-11-24.

## Context and Problem Statement

Several users can connect the same YouTube channel. The product also adds
workspaces, where an owner shares a channel with analysts and viewers. The
system must answer "may this user read this comment?" on every read path:
feed, saved comments, thread, search, aggregates, and export.

Two data shapes were possible: give every user or workspace its own copy of
comments, or store comments once per platform channel and scope reads. The
choice also decides where deletion and LLM cost land.

## Decision Drivers

- Storage and LLM cost must be linear in distinct channels, not in users.
- A user who pauses or loses access must still not see the comments again.
- One predicate must protect every read path; a forgotten predicate is a data
  leak.
- Account deletion must remove one user's access without destroying another
  user's data.

## Considered Options

1. **Shared comment store with per-user tracking and workspace grants.**
2. **Database per tenant.** Strong isolation, one schema per customer.
3. **Per-user comment copies.** Each tracking row owns its own comment rows.

## Decision Outcome

Chosen option 1.

`platform_channels` is the shared identity. `comments` references it and is
unique on `(platform_channel_id, platform_comment_id)`
(`internal/db/schema.sql`). `user_channel_tracking` holds one row per user and
channel, with the encrypted OAuth token and the backfill window.
`channel_grants` shares a channel with a workspace; `workspace_members` holds
the roles.

Read visibility is one SQL constant in `internal/db/comments_feed.go`:

```sql
EXISTS (tracking row for the user with status active or paused)
OR EXISTS (grant row joined to a workspace membership for the user)
```

`BuildFeedQuery` emits it unconditionally for `ModeFeed`, `ModeSaved`,
`ModeCount`, and `ModeStream`. `WorkspaceMiddleware` and
`RequireWorkspaceRole` guard write routes and reject non-members with 403
(`internal/api/workspace_middleware.go`).

Billing and quota stay on `users`. A workspace is a sharing overlay; seat
limits come from the owner's plan.

### Consequences

#### Good

- Comments and LLM results are stored and paid for once per distinct channel.
- One predicate is auditable and testable. `TestGrantChannel_NonMemberDeniedFeed`
  and `TestSentiment_AccessControlIsolation` fail if it regresses.
- Deletion is a detach operation: remove the tracking row and the user's stars;
  keep shared rows while another tracker exists.

#### Bad

- A bug in the predicate is a cross-tenant read, not a cosmetic error. The
  entire isolation model rests on one SQL string.
- A shared comment row has no single owner, so GDPR erasure of a third-party
  author is not expressible. Only the tracking user's data is erasable.
- Workspace roles and user-level billing are different subjects. An analyst
  who needs more seats cannot pay for them; only the workspace owner can.

#### Neutral

- The feed query joins three tables on every read and relies on indexes
  (`idx_comments_channel_processed`, `idx_user_channel_tracking_workspace`).
- Paused channels remain readable. This is intentional: the user keeps their
  data after pausing.

## Confirmation

- `go test ./internal/db/ -run 'TestGrantChannel_|TestBulkStar_'`
- `go test ./internal/api/ -run TestSentiment_AccessControlIsolation`
- CI job `fitness-tenant-isolation` runs both.

## More Information

- `internal/db/comments_feed.go`
- `internal/api/workspace_middleware.go`
- `internal/api/workspace_handlers.go` (`GrantChannel`)
- `docs/design.md` section 3.2
