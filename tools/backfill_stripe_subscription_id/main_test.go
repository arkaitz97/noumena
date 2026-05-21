package main

import (
	"database/sql"
	"testing"
)

// fakeResult implements sql.Result for testing.
type fakeResult struct {
	rowsAffected int64
}

func (r *fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r *fakeResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

// TestFakeResult verifies our fake sql.Result works correctly.
func TestFakeResult(t *testing.T) {
	r := &fakeResult{rowsAffected: 5}
	n, err := r.RowsAffected()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}
}

// TestSummaryFields verifies the Summary struct has the expected fields.
func TestSummaryFields(t *testing.T) {
	s := &Summary{
		Candidates:     10,
		Backfilled:     5,
		NoSubscription: 3,
	}
	if s.Candidates != 10 {
		t.Fatalf("expected 10 candidates, got %d", s.Candidates)
	}
	if s.Backfilled != 5 {
		t.Fatalf("expected 5 backfilled, got %d", s.Backfilled)
	}
	if s.NoSubscription != 3 {
		t.Fatalf("expected 3 no subscription, got %d", s.NoSubscription)
	}
}

// TestDBExecInterface verifies that *sql.DB implements dbExec.
func TestDBExecInterface(t *testing.T) {
	// This test just verifies at compile time that *sql.DB satisfies dbExec
	var _ dbExec = (*sql.DB)(nil)
}
