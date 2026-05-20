package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeRows implements pgx.Rows over a slice of (id, customerID).
type fakeRows struct {
	data [][2]string
	idx  int
}

func (f *fakeRows) Next() bool                                    { f.idx++; return f.idx <= len(f.data) }
func (f *fakeRows) Scan(dst ...any) error {
	row := f.data[f.idx-1]
	*dst[0].(*string) = row[0]
	*dst[1].(*string) = row[1]
	return nil
}
func (f *fakeRows) Close()                                        {}
func (f *fakeRows) Err() error                                    { return nil }
func (f *fakeRows) CommandTag() pgconn.CommandTag                 { return pgconn.CommandTag{} }
func (f *fakeRows) FieldDescriptions() []pgconn.FieldDescription  { return nil }
func (f *fakeRows) Values() ([]any, error)                        { return nil, nil }
func (f *fakeRows) RawValues() [][]byte                           { return nil }
func (f *fakeRows) Conn() *pgx.Conn                               { return nil }

// fakeDB implements dbExec; records every Exec call for assertions.
type fakeDB struct {
	rows      [][2]string
	queryErr  error
	execErr   error
	updates   []update
	rowsAff   int64 // RowsAffected to return on Exec
}

type update struct {
	subID  string
	userID string
}

func (f *fakeDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return &fakeRows{data: f.rows}, nil
}

func (f *fakeDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	f.updates = append(f.updates, update{
		subID:  args[0].(string),
		userID: args[1].(string),
	})
	// Simulate the requested RowsAffected by returning a synthetic tag.
	if f.rowsAff == 0 {
		f.rowsAff = 1
	}
	// pgconn.NewCommandTag is the only public constructor; build via UPDATE.
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

// 4.1: dry-run never calls Exec.
func TestRun_DryRunNeverWrites(t *testing.T) {
	db := &fakeDB{rows: [][2]string{{"u1", "cus_1"}, {"u2", "cus_2"}}}
	lookup := func(_ context.Context, cust string) (string, error) {
		return "sub_for_" + cust, nil
	}
	s, err := run(context.Background(), db, lookup, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Candidates != 2 {
		t.Errorf("Candidates: want 2 got %d", s.Candidates)
	}
	if s.Backfilled != 0 {
		t.Errorf("Backfilled in dry-run: want 0 got %d", s.Backfilled)
	}
	if len(db.updates) != 0 {
		t.Errorf("dry-run made %d Exec calls", len(db.updates))
	}
}

// 4.7: apply mode writes one UPDATE per candidate with a hit.
func TestRun_ApplyWritesOnePerHit(t *testing.T) {
	db := &fakeDB{rows: [][2]string{{"u1", "cus_1"}, {"u2", "cus_2"}}}
	lookup := func(_ context.Context, cust string) (string, error) {
		return "sub_for_" + cust, nil
	}
	s, err := run(context.Background(), db, lookup, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Backfilled != 2 {
		t.Errorf("Backfilled: want 2 got %d", s.Backfilled)
	}
	if len(db.updates) != 2 {
		t.Fatalf("expected 2 UPDATEs got %d", len(db.updates))
	}
	if db.updates[0].subID != "sub_for_cus_1" || db.updates[0].userID != "u1" {
		t.Errorf("first update wrong: %+v", db.updates[0])
	}
}

// 4.5/4.8: customers Stripe knows nothing about increment NoSubscription
// and don't trigger a write.
func TestRun_NoSubscriptionIncrementsCounter(t *testing.T) {
	db := &fakeDB{rows: [][2]string{{"u1", "cus_orphan"}}}
	lookup := func(_ context.Context, _ string) (string, error) { return "", nil }
	s, err := run(context.Background(), db, lookup, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.NoSubscription != 1 {
		t.Errorf("NoSubscription: want 1 got %d", s.NoSubscription)
	}
	if s.Backfilled != 0 {
		t.Errorf("Backfilled when no sub found: want 0 got %d", s.Backfilled)
	}
	if len(db.updates) != 0 {
		t.Errorf("expected no UPDATEs when no sub found")
	}
}

// 4.5: a Stripe-side error for one customer does not abort the whole run;
// other candidates still get processed.
func TestRun_StripeErrorIsPerCandidate(t *testing.T) {
	db := &fakeDB{rows: [][2]string{{"u_bad", "cus_bad"}, {"u_good", "cus_good"}}}
	lookup := func(_ context.Context, cust string) (string, error) {
		if cust == "cus_bad" {
			return "", errors.New("stripe rate-limited")
		}
		return "sub_good", nil
	}
	s, err := run(context.Background(), db, lookup, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Candidates != 2 {
		t.Errorf("Candidates: want 2 got %d", s.Candidates)
	}
	if s.Backfilled != 1 {
		t.Errorf("Backfilled: want 1 got %d", s.Backfilled)
	}
	if len(db.updates) != 1 || db.updates[0].userID != "u_good" {
		t.Errorf("expected one UPDATE for u_good, got %+v", db.updates)
	}
}

// 4.2: a database error during the SELECT bubbles up.
func TestRun_LoadCandidatesError(t *testing.T) {
	db := &fakeDB{queryErr: errors.New("connection refused")}
	_, err := run(context.Background(), db, nil, true)
	if err == nil {
		t.Fatal("expected error from Query")
	}
}
