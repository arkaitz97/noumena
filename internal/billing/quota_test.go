package billing

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Tests for the daily quota-reset cron (billing change, task 10.2).
//
// We validate the cron's plumbing — that it calls both DB primitives, that
// errors from one do not abort the other, and that the daily-time helper
// computes the correct delay until the next 08:00 UTC anchor.

type fakeQuotaStore struct {
	recordCalls  int
	resetCalls   int
	expireCalls  int
	resetErr     error
	expireErr    error
	recordErr    error
	resetRows    int64
	expireRows   int64
	recordRows   int64
	lastGraceHrs int
}

func (f *fakeQuotaStore) RecordCurrentPeriodUsage(ctx context.Context) (int64, error) {
	f.recordCalls++
	return f.recordRows, f.recordErr
}

func (f *fakeQuotaStore) ResetFreeTierQuotas(ctx context.Context) (int64, error) {
	f.resetCalls++
	return f.resetRows, f.resetErr
}

func (f *fakeQuotaStore) ExpirePastDueGrace(ctx context.Context, graceHours int) (int64, error) {
	f.expireCalls++
	f.lastGraceHrs = graceHours
	return f.expireRows, f.expireErr
}

func TestRunQuotaCronOnceCallsBothPrimitives(t *testing.T) {
	store := &fakeQuotaStore{recordRows: 7, resetRows: 5, expireRows: 2}
	runQuotaCronOnce(context.Background(), store, 72, nil)
	if store.recordCalls != 1 {
		t.Errorf("RecordCurrentPeriodUsage calls: want 1 got %d", store.recordCalls)
	}
	if store.resetCalls != 1 {
		t.Errorf("ResetFreeTierQuotas calls: want 1 got %d", store.resetCalls)
	}
	if store.expireCalls != 1 {
		t.Errorf("ExpirePastDueGrace calls: want 1 got %d", store.expireCalls)
	}
	if store.lastGraceHrs != 72 {
		t.Errorf("graceHours: want 72 got %d", store.lastGraceHrs)
	}
}

func TestRunQuotaCronOnceContinuesAfterResetError(t *testing.T) {
	store := &fakeQuotaStore{resetErr: errors.New("boom")}
	runQuotaCronOnce(context.Background(), store, 72, nil)
	if store.recordCalls != 1 {
		t.Errorf("RecordCurrentPeriodUsage must run before reset; calls=%d", store.recordCalls)
	}
	if store.expireCalls != 1 {
		t.Errorf("ExpirePastDueGrace must run even after reset error; calls=%d", store.expireCalls)
	}
}

func TestRunQuotaCronOnceContinuesAfterRecordError(t *testing.T) {
	store := &fakeQuotaStore{recordErr: errors.New("snapshot boom")}
	runQuotaCronOnce(context.Background(), store, 72, nil)
	if store.resetCalls != 1 {
		t.Errorf("ResetFreeTierQuotas must run even after record error; calls=%d", store.resetCalls)
	}
	if store.expireCalls != 1 {
		t.Errorf("ExpirePastDueGrace must run even after record error; calls=%d", store.expireCalls)
	}
}

func TestStartQuotaCronExitsOnContextCancel(t *testing.T) {
	store := &fakeQuotaStore{}
	ctx, cancel := context.WithCancel(context.Background())
	StartQuotaCron(ctx, store, QuotaCronConfig{
		Interval:          1 * time.Hour, // long enough not to tick during test
		PastDueGraceHours: 72,
	})
	// First run is immediate; give it a tiny grace then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()
	// We don't assert on no-extra-runs strictly (timing) but we DO assert
	// that at least one run happened.
	if store.resetCalls < 1 {
		t.Errorf("expected at least one cron run, got %d", store.resetCalls)
	}
}

func TestUntilNextDailyUTC(t *testing.T) {
	// Now = 2025-01-01 07:00 UTC; next 08:00 is in 1h.
	now := time.Date(2025, 1, 1, 7, 0, 0, 0, time.UTC)
	got := untilNextDailyUTC(now, 8, 0)
	if got != time.Hour {
		t.Errorf("want 1h, got %v", got)
	}

	// Now = 2025-01-01 08:00 UTC exactly; next is +24h (must be strictly after).
	now = time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	got = untilNextDailyUTC(now, 8, 0)
	if got != 24*time.Hour {
		t.Errorf("want 24h, got %v", got)
	}

	// Now = 2025-01-01 09:00 UTC; next 08:00 is in 23h.
	now = time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	got = untilNextDailyUTC(now, 8, 0)
	if got != 23*time.Hour {
		t.Errorf("want 23h, got %v", got)
	}
}

func TestStartQuotaCronNilStoreIsNoop(t *testing.T) {
	// Should not panic.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartQuotaCron(ctx, nil, QuotaCronConfig{})
}
