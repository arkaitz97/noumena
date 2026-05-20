package ingestion

import (
	"context"
	"testing"
	"time"

	"noumena/internal/db"
)

type fakeSchedDB struct {
	channels  []*db.PlatformChannel
	enqueued  map[string]string // platformChannelID|jobType -> jobID
	idCounter int
}

func (f *fakeSchedDB) GetChannelsNeedingSync(ctx context.Context, intervalMinutes int) ([]*db.PlatformChannel, error) {
	return f.channels, nil
}

func (f *fakeSchedDB) EnqueueIngestionJob(ctx context.Context, platformChannelID, jobType string) (string, error) {
	if f.enqueued == nil {
		f.enqueued = map[string]string{}
	}
	key := platformChannelID + "|" + jobType
	if _, exists := f.enqueued[key]; exists {
		return "", nil // idempotent: already pending
	}
	f.idCounter++
	id := "job-"
	for n := f.idCounter; n > 0; n /= 10 {
		id += string(rune('0' + n%10))
	}
	f.enqueued[key] = id
	return id, nil
}

func TestScheduler_EnqueuesBackfillForUnsynced(t *testing.T) {
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	f := &fakeSchedDB{
		channels: []*db.PlatformChannel{
			{ID: "c1", LastSyncedAt: nil},  // backfill
			{ID: "c2", LastSyncedAt: &old}, // poll
		},
	}
	s := NewScheduler(f, 30)
	res, err := s.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Backfills != 1 || res.Polls != 1 || res.Skipped != 0 {
		t.Fatalf("unexpected tick result: %+v", res)
	}
}

func TestScheduler_IdempotentAcrossTicks(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	f := &fakeSchedDB{
		channels: []*db.PlatformChannel{
			{ID: "c1", LastSyncedAt: nil},
			{ID: "c2", LastSyncedAt: &old},
		},
	}
	s := NewScheduler(f, 30)
	if _, err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := s.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Backfills != 0 || res.Polls != 0 || res.Skipped != 2 {
		t.Fatalf("expected all skipped on second tick, got %+v", res)
	}
}

func TestScheduler_DefaultsForBadInterval(t *testing.T) {
	s := NewScheduler(&fakeSchedDB{}, 0)
	if s.PollIntervalMin != 30 {
		t.Fatalf("expected default 30, got %d", s.PollIntervalMin)
	}
}
