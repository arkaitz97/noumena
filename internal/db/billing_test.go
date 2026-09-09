package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// Tests for the implement-missing-product-features billing additions:
// 3.1 per-period usage snapshots (RecordCurrentPeriodUsage /
// UpsertBillingUsage), 3.2 read path (GetBillingUsage), and 8.x overage
// invoicing primitives (ListOverageGroups / MarkOverageInvoiced /
// IsOverageInvoiced).

func setupBillingDB(t *testing.T) (context.Context, *db.SQLiteQuery) {
	t.Helper()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)
	return context.Background(), q
}

func seedUserWithPlan(t *testing.T, ctx context.Context, q *db.SQLiteQuery, email, plan string, used, quota int, resetAt time.Time) string {
	t.Helper()
	u, err := q.CreateUser(ctx, email, "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	planPtr := plan
	usedPtr := used
	quotaPtr := quota
	status := "active"
	reset := resetAt.UTC()
	if err := q.ApplySubscriptionUpdate(ctx, nil, db.SubscriptionUpdate{
		UserID:                 u.ID,
		SubscriptionPlan:       &planPtr,
		CommentsUsedThisPeriod: &usedPtr,
		CommentQuotaMonthly:    &quotaPtr,
		SubscriptionStatus:     &status,
		QuotaResetAt:           &reset,
	}); err != nil {
		t.Fatalf("ApplySubscriptionUpdate: %v", err)
	}
	return u.ID
}

func TestUpsertBillingUsageIdempotentPerPeriod(t *testing.T) {
	ctx, q := setupBillingDB(t)
	u, err := q.CreateUser(ctx, "upsert@example.com", "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	ps := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	pe := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	// First write creates the row; second write refreshes it.
	if err := q.UpsertBillingUsage(ctx, db.BillingUsage{UserID: u.ID, PeriodStart: ps, PeriodEnd: pe, CommentsUsed: 10, Quota: 1000}); err != nil {
		t.Fatalf("first UpsertBillingUsage: %v", err)
	}
	if err := q.UpsertBillingUsage(ctx, db.BillingUsage{UserID: u.ID, PeriodStart: ps, PeriodEnd: pe, CommentsUsed: 42, Quota: 1000}); err != nil {
		t.Fatalf("second UpsertBillingUsage: %v", err)
	}

	rows, err := q.GetBillingUsage(ctx, u.ID, 10)
	if err != nil {
		t.Fatalf("GetBillingUsage: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for the period, got %d", len(rows))
	}
	if rows[0].CommentsUsed != 42 {
		t.Errorf("CommentsUsed: want 42 got %d", rows[0].CommentsUsed)
	}
	if !rows[0].PeriodStart.Equal(ps) || !rows[0].PeriodEnd.Equal(pe) {
		t.Errorf("period mismatch: %v..%v", rows[0].PeriodStart, rows[0].PeriodEnd)
	}
}

func TestRecordCurrentPeriodUsageSnapshotsAllQuotaUsers(t *testing.T) {
	ctx, q := setupBillingDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.AddDate(0, 1, 0)

	// Two users with quota schedules, different plans + usage.
	creatorID := seedUserWithPlan(t, ctx, q, "creator@example.com", "creator", 12000, 50000, reset)
	freeID := seedUserWithPlan(t, ctx, q, "free@example.com", "free", 300, 1000, reset)

	// A user with NO quota_reset_at must be skipped.
	noReset, err := q.CreateUser(ctx, "noreset@example.com", "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := q.DB().ExecContext(ctx,
		`UPDATE users SET quota_reset_at = NULL WHERE id = ?`, noReset.ID); err != nil {
		t.Fatalf("clear quota_reset_at: %v", err)
	}

	n, err := q.RecordCurrentPeriodUsage(ctx)
	if err != nil {
		t.Fatalf("RecordCurrentPeriodUsage: %v", err)
	}
	if n != 2 {
		t.Errorf("rows written: want 2 got %d", n)
	}

	// Idempotent: a second run upserts, does not duplicate.
	if n2, err := q.RecordCurrentPeriodUsage(ctx); err != nil || n2 != 2 {
		t.Errorf("second run: n=%d err=%v, want 2 nil", n2, err)
	}

	creatorRows, err := q.GetBillingUsage(ctx, creatorID, 5)
	if err != nil {
		t.Fatalf("GetBillingUsage creator: %v", err)
	}
	if len(creatorRows) != 1 || creatorRows[0].CommentsUsed != 12000 || creatorRows[0].Quota != 50000 {
		t.Errorf("creator snapshot wrong: %+v", creatorRows)
	}
	if !creatorRows[0].PeriodEnd.Equal(reset) {
		t.Errorf("creator period_end: want %v got %v", reset, creatorRows[0].PeriodEnd)
	}
	wantStart := reset.AddDate(0, -1, 0)
	if !creatorRows[0].PeriodStart.Equal(wantStart) {
		t.Errorf("creator period_start: want %v got %v", wantStart, creatorRows[0].PeriodStart)
	}

	freeRows, err := q.GetBillingUsage(ctx, freeID, 5)
	if err != nil {
		t.Fatalf("GetBillingUsage free: %v", err)
	}
	if len(freeRows) != 1 || freeRows[0].CommentsUsed != 300 || freeRows[0].Quota != 1000 {
		t.Errorf("free snapshot wrong: %+v", freeRows)
	}
}

func TestGetBillingUsageNewestFirstLimit(t *testing.T) {
	ctx, q := setupBillingDB(t)
	u, err := q.CreateUser(ctx, "history@example.com", "h")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	// Insert 3 periods (older -> newer).
	for i := 0; i < 3; i++ {
		ps := time.Date(2026, time.Month(6+i), 1, 0, 0, 0, 0, time.UTC)
		pe := ps.AddDate(0, 1, 0)
		if err := q.UpsertBillingUsage(ctx, db.BillingUsage{UserID: u.ID, PeriodStart: ps, PeriodEnd: pe, CommentsUsed: i * 100, Quota: 1000}); err != nil {
			t.Fatalf("UpsertBillingUsage: %v", err)
		}
	}

	rows, err := q.GetBillingUsage(ctx, u.ID, 2)
	if err != nil {
		t.Fatalf("GetBillingUsage: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("limit: want 2 rows got %d", len(rows))
	}
	if !rows[0].PeriodStart.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("newest first: first period_start = %v", rows[0].PeriodStart)
	}
	if rows[0].CommentsUsed != 200 {
		t.Errorf("newest first: comments_used = %d, want 200", rows[0].CommentsUsed)
	}
}

func TestListOverageGroupsAndMarkInvoiced(t *testing.T) {
	ctx, q := setupBillingDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.AddDate(0, 1, 0)
	ps := now.AddDate(0, -1, 0)

	creatorID := seedUserWithPlan(t, ctx, q, "ov-creator@example.com", "creator", 0, 50000, reset)
	studioID := seedUserWithPlan(t, ctx, q, "ov-studio@example.com", "studio", 0, 250000, reset)
	freeID := seedUserWithPlan(t, ctx, q, "ov-free@example.com", "free", 0, 1000, reset)

	insertLog := func(userID string, count int, pStart, pEnd time.Time) {
		t.Helper()
		if err := q.InsertCommentUsageLog(ctx, nil, userID, count, pStart, pEnd); err != nil {
			t.Fatalf("InsertCommentUsageLog: %v", err)
		}
	}
	// Creator: 1500 overage in one period. Studio: 3000 in one period.
	// Free: 9000 overage (must be excluded — free does not track overage).
	insertLog(creatorID, 500, ps, reset)
	insertLog(creatorID, 1000, ps, reset)
	insertLog(studioID, 3000, ps, reset)
	insertLog(freeID, 9000, ps, reset)

	groups, err := q.ListOverageGroups(ctx)
	if err != nil {
		t.Fatalf("ListOverageGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("want 2 overage groups (creator+studio), got %d: %+v", len(groups), groups)
	}
	byUser := map[string]int{}
	for _, g := range groups {
		byUser[g.UserID] = g.Overage
	}
	if byUser[creatorID] != 1500 {
		t.Errorf("creator overage: want 1500 got %d", byUser[creatorID])
	}
	if byUser[studioID] != 3000 {
		t.Errorf("studio overage: want 3000 got %d", byUser[studioID])
	}
	if _, ok := byUser[freeID]; ok {
		t.Error("free plan must not appear in overage groups")
	}

	// Mark creator invoiced; next listing excludes it.
	if err := q.MarkOverageInvoiced(ctx, creatorID, ps.Format(time.RFC3339), "cbtxn_1", 100); err != nil {
		t.Fatalf("MarkOverageInvoiced: %v", err)
	}
	ok, err := q.IsOverageInvoiced(ctx, creatorID, ps.Format(time.RFC3339))
	if err != nil || !ok {
		t.Errorf("IsOverageInvoiced: %v %v", ok, err)
	}
	// Idempotent re-mark.
	if err := q.MarkOverageInvoiced(ctx, creatorID, ps.Format(time.RFC3339), "cbtxn_1", 100); err != nil {
		t.Fatalf("re-MarkOverageInvoiced: %v", err)
	}

	groups2, err := q.ListOverageGroups(ctx)
	if err != nil {
		t.Fatalf("ListOverageGroups #2: %v", err)
	}
	if len(groups2) != 1 || groups2[0].UserID != studioID {
		t.Errorf("after mark-invoiced: want only studio group, got %+v", groups2)
	}

	var n int
	if err := q.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM overage_invoices`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("overage_invoices rows: want 1 got %d", n)
	}
}

// Overage grouping works for users regardless of Stripe customer presence
// (the job skips users without a customer id, but listing must not error).
func TestListOverageGroupsUnknownPlanUser(t *testing.T) {
	ctx, q := setupBillingDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.AddDate(0, 1, 0)
	ps := now.AddDate(0, -1, 0)

	creatorID := seedUserWithPlan(t, ctx, q, fmt.Sprintf("plan-%d@example.com", time.Now().UnixNano()), "creator", 0, 50000, reset)
	if err := q.InsertCommentUsageLog(ctx, nil, creatorID, 2500, ps, reset); err != nil {
		t.Fatalf("InsertCommentUsageLog: %v", err)
	}
	groups, err := q.ListOverageGroups(ctx)
	if err != nil {
		t.Fatalf("ListOverageGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].Overage != 2500 {
		t.Errorf("groups: want single 2500-overage group, got %+v", groups)
	}
}
