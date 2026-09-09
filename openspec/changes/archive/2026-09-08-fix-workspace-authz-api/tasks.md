## 1. Mount workspace role enforcement

- [x] 1.1 Resolve workspace_id (path/header/JWT default) in middleware
- [x] 1.2 Check membership + role; deny with 403 when absent
- [x] 1.3 Mount on workspace-scoped /api routes in main.go
- [x] 1.4 Action→role map: connect/remove/star/member-mgmt=billing admin; export/insights=analyst+; read=viewer+
- [x] 1.5 Tests: viewer denied write; analyst can export; admin can manage; non-member 403

## 2. Workspace scoping + shared channels

- [x] 2.1 Set `tracking.workspace_id` at connect (migration backfills existing)
- [x] 2.2 Add `channel_grants(workspace_id, channel_id)` table (migration, UNIQUE)
- [x] 2.3 Grant/revoke endpoints (admin/analyst gate)
- [x] 2.4 BuildFeedQuery accepts access predicate: own-tracking OR workspace grant
- [x] 2.5 Apply predicate to star access + export filtering
- [x] 2.6 Tests: shared channel visible to member; non-member denied; star without access denied

## 3. Fix single-workspace endpoint

- [x] 3.1 Implement `GET /workspaces/{id}` returning one workspace (membership-checked)
- [x] 3.2 Test: returns 404 for foreign workspace, correct row for own

## 4. Star access control + legacy cleanup

- [x] 4.1 Gate Star/BulkStar/Unstar with access predicate
- [x] 4.2 Remove duplicate `/comments/feed` and `/comments/{id}/star` legacy registrations + handlers
- [x] 4.3 Tests: star foreign comment denied

## 5. Input validation + atomic cap

- [x] 5.1 Clamp `days` to {7,30,90} in ActionableInsights/TopInsights/InsightDistribution
- [x] 5.2 Move channel-limit check inside tracking INSERT tx
- [x] 5.3 Tests: days=10000 clamped; parallel connects can't exceed cap

## 6. Backfill wiring

- [x] 6.1 Fix `UpsertPlatformChannel` inserted detection (`xmax=0` or select-before-insert)
- [x] 6.2 Assign real `BackfillEnqueuer` in main.go
- [x] 6.3 Test: new channel enqueues backfill job; reconnect does not double-enqueue

## 7. Verify

- [x] 7.1 `go vet`, `go build`, `go test ./internal/api/... ./internal/db/...` green
- [ ] 7.2 Manual: viewer blocked from remove channel; shared channel appears for member; backfill job appears on connect
