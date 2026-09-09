package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"noumena/internal/db"
)

// Tests for the overage-invoicing job (implement-missing-product-features
// 8.x): exactly-once per (user, period) charging, guard-table marking,
// config gating, and Stripe-failure retry behaviour.

type fakeOverageStore struct {
	groups []db.OverageGroup
	users  map[string]*db.User
	// marked records MarkOverageInvoiced calls as userID|periodStart.
	marked  map[string]string
	markErr error
	getErr  error
}

func (f *fakeOverageStore) ListOverageGroups(ctx context.Context) ([]db.OverageGroup, error) {
	return f.groups, nil
}

func (f *fakeOverageStore) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	u, ok := f.users[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (f *fakeOverageStore) MarkOverageInvoiced(ctx context.Context, userID, periodStart, invoiceID string, amountCents int) error {
	if f.markErr != nil {
		return f.markErr
	}
	if f.marked == nil {
		f.marked = map[string]string{}
	}
	f.marked[userID+"|"+periodStart] = invoiceID
	return nil
}

func overageTestUser(id, plan string) *db.User {
	cust := "cus_" + id
	return &db.User{ID: id, SubscriptionPlan: plan, StripeCustomerID: &cust}
}

func TestOverageAmountCents(t *testing.T) {
	cases := []struct {
		overage int
		want    int
	}{
		{0, 0},
		{-5, 0},
		{1, 50},    // ceil(1/1000) * 50
		{999, 50},  // one chunk
		{1000, 50}, // one chunk
		{1001, 100},
		{2500, 150},
		{3000, 150},
	}
	for _, c := range cases {
		if got := OverageAmountCents(c.overage); got != c.want {
			t.Errorf("OverageAmountCents(%d): want %d got %d", c.overage, c.want, got)
		}
	}
}

func TestInvoiceOverageDisabledIsNoop(t *testing.T) {
	store := &fakeOverageStore{
		groups: []db.OverageGroup{{UserID: "u1", Plan: PlanCreator, Overage: 500}},
		users:  map[string]*db.User{"u1": overageTestUser("u1", PlanCreator)},
	}
	client := NewMockClient()

	if err := InvoiceOverage(context.Background(), store, client, false); err != nil {
		t.Fatalf("InvoiceOverage(disabled): %v", err)
	}
	if len(client.BalanceCharges) != 0 {
		t.Errorf("disabled: expected no charges, got %+v", client.BalanceCharges)
	}
	if len(store.marked) != 0 {
		t.Errorf("disabled: expected no guard rows, got %+v", store.marked)
	}
}

func TestInvoiceOverageChargesOnceAndGuards(t *testing.T) {
	ctx := context.Background()
	ps := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	pe := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	store := &fakeOverageStore{
		groups: []db.OverageGroup{
			{UserID: "u1", Plan: PlanCreator, PeriodStart: ps, PeriodEnd: pe, Overage: 1500},
			{UserID: "u2", Plan: PlanStudio, PeriodStart: ps, PeriodEnd: pe, Overage: 2500},
		},
		users: map[string]*db.User{
			"u1": overageTestUser("u1", PlanCreator),
			"u2": overageTestUser("u2", PlanStudio),
		},
	}
	client := NewMockClient()

	if err := InvoiceOverage(ctx, store, client, true); err != nil {
		t.Fatalf("InvoiceOverage: %v", err)
	}

	// Both users charged, correct amounts (1500 -> 2 units -> 100c; 2500 -> 3 units -> 150c).
	if len(client.BalanceCharges["cus_u1"]) != 1 || client.BalanceCharges["cus_u1"][0].AmountCents != 100 {
		t.Errorf("u1 charge: %+v", client.BalanceCharges["cus_u1"])
	}
	if len(client.BalanceCharges["cus_u2"]) != 1 || client.BalanceCharges["cus_u2"][0].AmountCents != 150 {
		t.Errorf("u2 charge: %+v", client.BalanceCharges["cus_u2"])
	}
	// Idempotency key is per (user, period).
	wantKey1 := "overage:u1:" + ps.Format(time.RFC3339)
	if got := client.BalanceCharges["cus_u1"][0].IdempotencyKey; got != wantKey1 {
		t.Errorf("u1 idempotency key: want %q got %q", wantKey1, got)
	}
	// Metadata carries the period for cross-referencing.
	if got := client.BalanceCharges["cus_u1"][0].Metadata["period_start"]; got != ps.Format(time.RFC3339) {
		t.Errorf("u1 metadata period_start: got %q", got)
	}
	// Guard rows recorded for both.
	if len(store.marked) != 2 {
		t.Errorf("guard rows: want 2 got %+v", store.marked)
	}
	if store.marked["u1|"+ps.Format(time.RFC3339)] == "" {
		t.Error("u1 guard row missing")
	}

	// Second run: simulate the store excluding invoiced groups (as the real
	// SQL does) — nothing new is charged.
	store.groups = nil
	if err := InvoiceOverage(ctx, store, client, true); err != nil {
		t.Fatalf("InvoiceOverage #2: %v", err)
	}
	if len(client.BalanceCharges["cus_u1"]) != 1 || len(client.BalanceCharges["cus_u2"]) != 1 {
		t.Errorf("double-run: charges must not grow, got %+v", client.BalanceCharges)
	}
}

// Idempotency end-to-end: the store still returns the group (e.g. guard
// write raced) but the client is Stripe-backed; the local guard must win.
// Here we simulate the real flow: a second InvoiceOverage over the same
// uninvoiced groups would charge twice — so the store MUST exclude marked
// periods. This test pins the contract by asserting the job relies on
// MarkOverageInvoiced having been called for every charged group.
func TestInvoiceOverageMarksEveryChargedGroup(t *testing.T) {
	ctx := context.Background()
	ps := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeOverageStore{
		groups: []db.OverageGroup{
			{UserID: "u1", Plan: PlanCreator, PeriodStart: ps, Overage: 2000},
		},
		users: map[string]*db.User{"u1": overageTestUser("u1", PlanCreator)},
	}
	client := NewMockClient()

	if err := InvoiceOverage(ctx, store, client, true); err != nil {
		t.Fatalf("InvoiceOverage: %v", err)
	}
	if len(client.BalanceCharges["cus_u1"]) != 1 {
		t.Fatalf("expected 1 charge, got %+v", client.BalanceCharges["cus_u1"])
	}
	invoiceID := client.BalanceCharges["cus_u1"][0].ID
	if store.marked["u1|"+ps.Format(time.RFC3339)] != invoiceID {
		t.Errorf("guard must reference the returned invoice id %q, got %q",
			invoiceID, store.marked["u1|"+ps.Format(time.RFC3339)])
	}
}

func TestInvoiceOverageSkipsNoStripeCustomer(t *testing.T) {
	ctx := context.Background()
	ps := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	u := &db.User{ID: "u1", SubscriptionPlan: PlanCreator} // no StripeCustomerID
	store := &fakeOverageStore{
		groups: []db.OverageGroup{{UserID: "u1", Plan: PlanCreator, PeriodStart: ps, Overage: 900}},
		users:  map[string]*db.User{"u1": u},
	}
	client := NewMockClient()

	if err := InvoiceOverage(ctx, store, client, true); err != nil {
		t.Fatalf("InvoiceOverage: %v", err)
	}
	if len(client.BalanceCharges) != 0 {
		t.Errorf("no customer: expected no charges, got %+v", client.BalanceCharges)
	}
	if len(store.marked) != 0 {
		t.Errorf("no customer: expected no guard rows, got %+v", store.marked)
	}
}

func TestInvoiceOverageStripeErrorRetriesNextRun(t *testing.T) {
	ctx := context.Background()
	ps := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeOverageStore{
		groups: []db.OverageGroup{{UserID: "u1", Plan: PlanCreator, PeriodStart: ps, Overage: 500}},
		users:  map[string]*db.User{"u1": overageTestUser("u1", PlanCreator)},
	}
	client := NewMockClient()
	client.BalanceChargeErr = errors.New("stripe down")

	if err := InvoiceOverage(ctx, store, client, true); err != nil {
		t.Fatalf("InvoiceOverage: %v", err)
	}
	// Charge failed -> no guard row -> next run retries.
	if len(store.marked) != 0 {
		t.Errorf("failed charge must not mark invoiced, got %+v", store.marked)
	}

	client.BalanceChargeErr = nil
	if err := InvoiceOverage(ctx, store, client, true); err != nil {
		t.Fatalf("InvoiceOverage retry: %v", err)
	}
	if len(client.BalanceCharges["cus_u1"]) != 1 {
		t.Errorf("retry: want 1 charge got %+v", client.BalanceCharges["cus_u1"])
	}
	if len(store.marked) != 1 {
		t.Errorf("retry: want 1 guard row got %+v", store.marked)
	}
}

func TestInvoiceOverageMarkErrorContinues(t *testing.T) {
	ctx := context.Background()
	ps := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeOverageStore{
		groups: []db.OverageGroup{
			{UserID: "u1", Plan: PlanCreator, PeriodStart: ps, Overage: 500},
			{UserID: "u2", Plan: PlanCreator, PeriodStart: ps, Overage: 1500},
		},
		users: map[string]*db.User{
			"u1": overageTestUser("u1", PlanCreator),
			"u2": overageTestUser("u2", PlanCreator),
		},
		markErr: errors.New("guard write failed"),
	}
	client := NewMockClient()

	// Mark failure must not abort the run: both groups still get charged.
	if err := InvoiceOverage(ctx, store, client, true); err != nil {
		t.Fatalf("InvoiceOverage: %v", err)
	}
	if len(client.BalanceCharges["cus_u1"]) != 1 || len(client.BalanceCharges["cus_u2"]) != 1 {
		t.Errorf("mark error must not skip other groups, got %+v", client.BalanceCharges)
	}
}

func TestStartOverageInvoicingCronDisabledIsNoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &fakeOverageStore{}
	StartOverageInvoicingCron(ctx, store, NewMockClient(), OverageInvoicingConfig{Enabled: false})
}

func TestStartOverageInvoicingCronNilStoreIsNoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartOverageInvoicingCron(ctx, nil, NewMockClient(), OverageInvoicingConfig{Enabled: true})
}

func TestStartOverageInvoicingCronRuns(t *testing.T) {
	ps := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeOverageStore{
		groups: []db.OverageGroup{{UserID: "u1", Plan: PlanCreator, PeriodStart: ps, Overage: 1000}},
		users:  map[string]*db.User{"u1": overageTestUser("u1", PlanCreator)},
	}
	client := NewMockClient()
	ctx, cancel := context.WithCancel(context.Background())
	StartOverageInvoicingCron(ctx, store, client, OverageInvoicingConfig{
		Enabled:  true,
		Interval: time.Hour, // long enough not to tick during the test
	})
	time.Sleep(50 * time.Millisecond)
	cancel()

	if len(client.BalanceCharges["cus_u1"]) != 1 {
		t.Errorf("cron first run: want 1 charge got %+v", client.BalanceCharges)
	}
}
