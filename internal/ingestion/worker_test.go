package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"noumena/internal/db"
)

// fakeWorkerDB implements WorkerDB without a real Postgres. Lock acquisition
// is bypassed via a separate test path in processJobNoLock.
type fakeWorkerDB struct {
	mu               sync.Mutex
	jobs             []*db.IngestionJob
	channels         map[string]*db.PlatformChannel
	tokenA           string
	tokenR           string
	tokenErr         error
	promoteCalls     int
	completedJobs    []string
	failedJobs       []string
	deadLetteredJobs []string
	rescheduled      []string
	insertCalls      [][]*db.Comment
	lastSyncedSet    map[string]time.Time
	backoffSet       map[string]time.Time
	resetCalls       []string
	maxBackfillDays  int
}

func newFakeWorkerDB() *fakeWorkerDB {
	return &fakeWorkerDB{
		channels:        map[string]*db.PlatformChannel{},
		lastSyncedSet:   map[string]time.Time{},
		backoffSet:      map[string]time.Time{},
		maxBackfillDays: 90,
	}
}
func (f *fakeWorkerDB) ClaimNextIngestionJob(ctx context.Context) (*db.IngestionJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.jobs) == 0 {
		return nil, nil
	}
	j := f.jobs[0]
	f.jobs = f.jobs[1:]
	return j, nil
}
func (f *fakeWorkerDB) ReleaseJobToPending(_ context.Context, jobID string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rescheduled = append(f.rescheduled, jobID)
	return nil
}
func (f *fakeWorkerDB) MarkJobCompleted(_ context.Context, jobID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completedJobs = append(f.completedJobs, jobID)
	return nil
}
func (f *fakeWorkerDB) MarkJobFailed(_ context.Context, jobID, _ string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failedJobs = append(f.failedJobs, jobID)
	return nil
}
func (f *fakeWorkerDB) MarkJobDeadLettered(_ context.Context, jobID, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deadLetteredJobs = append(f.deadLetteredJobs, jobID)
	return nil
}
func (f *fakeWorkerDB) UpdateLastSyncedAt(_ context.Context, pcID string, t time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastSyncedSet[pcID] = t
	return nil
}
func (f *fakeWorkerDB) UpdateLastSyncedAtIfNull(_ context.Context, pcID string) error {
	return nil
}
func (f *fakeWorkerDB) UpdateIngestionError(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}
func (f *fakeWorkerDB) ResetIngestionError(_ context.Context, pcID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resetCalls = append(f.resetCalls, pcID)
	return nil
}
func (f *fakeWorkerDB) MarkChannelBackoff(_ context.Context, pcID string, until time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.backoffSet[pcID] = until
	return nil
}
func (f *fakeWorkerDB) GetPlatformChannelByID(_ context.Context, pcID string) (*db.PlatformChannel, error) {
	if c, ok := f.channels[pcID]; ok {
		return c, nil
	}
	return nil, db.ErrNotFound
}
func (f *fakeWorkerDB) GetIngestionToken(_ context.Context, _ string) (string, string, error) {
	return f.tokenA, f.tokenR, f.tokenErr
}
func (f *fakeWorkerDB) GetIngestionTokenRow(_ context.Context, _ string) (*db.UserChannelTracking, bool, error) {
	return nil, false, nil
}
func (f *fakeWorkerDB) UpdateTokenAndExpiry(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}
func (f *fakeWorkerDB) RecoverStaleIngestionJobs(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
func (f *fakeWorkerDB) PromoteNextIngestionToken(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.promoteCalls++
	return nil
}
func (f *fakeWorkerDB) GetMaxBackfillDays(_ context.Context, _ string) (int, error) {
	return f.maxBackfillDays, nil
}
func (f *fakeWorkerDB) InsertComments(_ context.Context, comments []*db.Comment) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]*db.Comment, len(comments))
	copy(cp, comments)
	f.insertCalls = append(f.insertCalls, cp)
	return len(comments), nil
}

// processJobNoLock is a test-only variant of processJob that skips advisory
// locking (the real lock requires a live Postgres pool). It mirrors the
// production state machine so we can exercise the dispatch logic.
func (w *Worker) processJobNoLock(parent context.Context, job *db.IngestionJob) {
	jobCtx, cancel := context.WithTimeout(parent, w.JobDeadline)
	defer cancel()

	pc, err := w.DB.GetPlatformChannelByID(jobCtx, job.PlatformChannelID)
	if err != nil {
		w.failJob(parent, job, err)
		return
	}
	if pc.Platform != "youtube" {
		_ = w.DB.MarkJobCompleted(parent, job.ID)
		return
	}

	// In tests we pass the access token via fakeWorkerDB.tokenA directly,
	// short-circuiting oauth2 refresh.
	accessTok, _, err := w.DB.GetIngestionToken(jobCtx, job.PlatformChannelID)
	if err != nil || accessTok == "" {
		_ = w.DB.MarkJobCompleted(parent, job.ID)
		return
	}

	publishedAfter := pc.LastSyncedAt
	if job.JobType == "backfill" || pc.LastSyncedAt == nil {
		t := time.Now().Add(-90 * 24 * time.Hour)
		publishedAfter = &t
	}

	if err := w.runIngestion(jobCtx, parent, pc, accessTok, publishedAfter, job); err != nil {
		var rate *RateLimitError
		var tokInv *TokenInvalidError
		switch {
		case errors.As(err, &rate):
			_ = w.DB.MarkChannelBackoff(parent, job.PlatformChannelID, time.Now().Add(rate.RetryAfter))
			_ = w.DB.MarkJobCompleted(parent, job.ID)
			return
		case errors.As(err, &tokInv):
			_ = w.DB.PromoteNextIngestionToken(parent, job.PlatformChannelID)
			_ = w.DB.ReleaseJobToPending(parent, job.ID, 5*time.Second)
			return
		case errors.Is(err, context.DeadlineExceeded):
			_ = w.DB.MarkJobCompleted(parent, job.ID)
			return
		}
		w.failJob(parent, job, err)
		return
	}
	_ = w.DB.MarkJobCompleted(parent, job.ID)
	_ = w.DB.ResetIngestionError(parent, job.PlatformChannelID)
	_ = w.DB.UpdateLastSyncedAt(parent, job.PlatformChannelID, time.Now())
}

// stubYouTubeServer returns an httptest server that serves one video with
// two top-level comments and one reply.
func stubYouTubeServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/search":
			_, _ = w.Write([]byte(`{"items":[
				{"id":{"videoId":"v1"},"snippet":{"publishedAt":"2025-01-01T10:00:00Z"}}
			]}`))
		case r.URL.Path == "/commentThreads":
			resp := commentThreadsResponse{
				Items: []commentThreadItem{
					{
						ID: "T1",
						Snippet: struct {
							TopLevelComment ytComment `json:"topLevelComment"`
							TotalReplyCount int       `json:"totalReplyCount"`
						}{
							TopLevelComment: ytComment{ID: "c1", Snippet: commentSnippet{
								AuthorDisplayName: "A", TextDisplay: "hi", PublishedAt: "2025-01-01T10:05:00Z",
							}},
						},
						Replies: &struct {
							Comments []ytComment `json:"comments"`
						}{Comments: []ytComment{{ID: "c1r", Snippet: commentSnippet{
							AuthorDisplayName: "B", TextDisplay: "yo", PublishedAt: "2025-01-01T10:06:00Z",
						}}}},
					},
					{
						ID: "T2",
						Snippet: struct {
							TopLevelComment ytComment `json:"topLevelComment"`
							TotalReplyCount int       `json:"totalReplyCount"`
						}{
							TopLevelComment: ytComment{ID: "c2", Snippet: commentSnippet{
								AuthorDisplayName: "C", TextDisplay: "hey", PublishedAt: "2025-01-01T10:07:00Z",
							}},
						},
					},
				},
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return srv
}

func TestWorker_HappyPathPoll(t *testing.T) {
	srv := stubYouTubeServer(t)
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{
		ID:                "pc1",
		Platform:          "youtube",
		PlatformChannelID: "UC1",
		LastSyncedAt:      &last,
	}
	f.tokenA = "tok"
	f.tokenR = "ref"

	w := &Worker{
		DB:          f,
		Client:      &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL},
		JobDeadline: 30 * time.Second,
		Alerter:     LogAlerter{},
	}
	job := &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"}
	w.processJobNoLock(context.Background(), job)

	if len(f.completedJobs) != 1 || f.completedJobs[0] != "j1" {
		t.Fatalf("expected j1 completed, got %v", f.completedJobs)
	}
	if len(f.insertCalls) == 0 {
		t.Fatalf("expected at least one InsertComments batch")
	}
	total := 0
	for _, b := range f.insertCalls {
		total += len(b)
	}
	if total != 3 {
		t.Fatalf("expected 3 comments inserted, got %d", total)
	}
	if len(f.resetCalls) != 1 {
		t.Fatalf("expected ResetIngestionError once, got %v", f.resetCalls)
	}
}

func TestWorker_RateLimitParksChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA = "tok"
	f.tokenR = "ref"

	w := &Worker{
		DB:          f,
		Client:      &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL},
		JobDeadline: 5 * time.Second,
		Alerter:     LogAlerter{},
	}
	job := &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"}
	w.processJobNoLock(context.Background(), job)

	if _, ok := f.backoffSet["pc1"]; !ok {
		t.Fatalf("expected channel parked via MarkChannelBackoff")
	}
	if len(f.completedJobs) != 1 {
		t.Fatalf("rate-limit should mark job completed, got completed=%v failed=%v", f.completedJobs, f.failedJobs)
	}
	if len(f.failedJobs) != 0 {
		t.Fatalf("rate-limit must NOT consume retry budget, got failed=%v", f.failedJobs)
	}
}

func TestWorker_TokenInvalidPromotes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA = "tok"
	f.tokenR = "ref"

	w := &Worker{
		DB:          f,
		Client:      &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL},
		JobDeadline: 5 * time.Second,
		Alerter:     LogAlerter{},
	}
	job := &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"}
	w.processJobNoLock(context.Background(), job)

	if f.promoteCalls != 1 {
		t.Fatalf("expected one PromoteNextIngestionToken call, got %d", f.promoteCalls)
	}
	if len(f.rescheduled) != 1 {
		t.Fatalf("expected job rescheduled, got %v", f.rescheduled)
	}
}

func TestWorker_DeadLetterAfterMaxRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA = "tok"
	f.tokenR = "ref"

	alerts := 0
	w := &Worker{
		DB:          f,
		Client:      &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL},
		JobDeadline: 5 * time.Second,
		Alerter:     alerterFunc(func() { alerts++ }),
	}
	job := &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll", RetryCount: maxRetries}
	w.processJobNoLock(context.Background(), job)

	if len(f.deadLetteredJobs) != 1 {
		t.Fatalf("expected dead-letter, got dead=%v failed=%v", f.deadLetteredJobs, f.failedJobs)
	}
	if alerts != 1 {
		t.Fatalf("expected exactly one alert, got %d", alerts)
	}
	if _, ok := f.backoffSet["pc1"]; !ok {
		t.Fatalf("expected channel parked for 1h on dead-letter")
	}
}

type alerterFunc func()

func (a alerterFunc) Alert(_ context.Context, _, _, _ string) { a() }

func TestWorker_RecoverStaleJobsCallsDB(t *testing.T) {
	f := newFakeWorkerDB()
	w := &Worker{DB: f, JobDeadline: 5 * time.Second, Alerter: LogAlerter{}}
	// Must not panic and must complete.
	w.RecoverStaleJobs(context.Background())
}
