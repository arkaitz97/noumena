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
	enqueueCounter   int

	// SyncStateDB state (optimize-youtube-api-quota).
	playlists   map[string]string
	pollSet     map[string]int
	videoStates map[string]*db.VideoSyncState
}

func newFakeWorkerDB() *fakeWorkerDB {
	return &fakeWorkerDB{
		channels:        map[string]*db.PlatformChannel{},
		lastSyncedSet:   map[string]time.Time{},
		backoffSet:      map[string]time.Time{},
		maxBackfillDays: 90,
		playlists:       map[string]string{},
		pollSet:         map[string]int{},
		videoStates:     map[string]*db.VideoSyncState{},
	}
}

func videoKey(pcID, videoID string) string { return pcID + "/" + videoID }

func (f *fakeWorkerDB) SetUploadsPlaylistID(_ context.Context, pcID, playlistID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playlists[pcID] = playlistID
	return nil
}
func (f *fakeWorkerDB) SetPollInterval(_ context.Context, pcID string, minutes int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pollSet[pcID] = minutes
	return nil
}
func (f *fakeWorkerDB) GetVideoSyncState(_ context.Context, pcID, videoID string) (*db.VideoSyncState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.videoStates[videoKey(pcID, videoID)]; ok {
		cp := *st
		return &cp, nil
	}
	return nil, db.ErrNotFound
}
func (f *fakeWorkerDB) UpsertVideoSyncState(_ context.Context, st *db.VideoSyncState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := videoKey(st.PlatformChannelID, st.VideoID)
	existing, ok := f.videoStates[key]
	if !ok {
		cp := *st
		if cp.Status == "" {
			cp.Status = "active"
		}
		f.videoStates[key] = &cp
		return nil
	}
	// Mirror SQLite semantics: watermark COALESCE(kept when nil), page
	// checkpoint and status replaced wholesale.
	if st.CommentWatermarkAt != nil {
		existing.CommentWatermarkAt = st.CommentWatermarkAt
	}
	existing.PageCheckpoint = st.PageCheckpoint
	if st.Status != "" {
		existing.Status = st.Status
	}
	return nil
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

// GetChannelsNeedingSync / EnqueueIngestionJob make fakeWorkerDB usable as a
// SchedulerDB so tests can assert how the scheduler classifies a channel from
// the exact watermark state the worker produced.
func (f *fakeWorkerDB) GetChannelsNeedingSync(_ context.Context, _ int) ([]*db.PlatformChannel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*db.PlatformChannel
	for id, ch := range f.channels {
		if _, synced := f.lastSyncedSet[id]; !synced {
			out = append(out, ch)
		}
	}
	return out, nil
}

func (f *fakeWorkerDB) EnqueueIngestionJob(_ context.Context, platformChannelID, jobType string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enqueueCounter++
	return "job-" + platformChannelID + "-" + jobType, nil
}

// processJobNoLock is a test-only variant of processJob that skips advisory
// locking (the real lock requires a live Postgres pool). It mirrors the
// production state machine so we can exercise the dispatch logic.
func (w *Worker) processJobNoLock(parent context.Context, job *db.IngestionJob) {
	jobCtx, cancel := context.WithTimeout(parent, w.JobDeadline)
	defer cancel()
	jobCtx = WithJobRequestBudget(jobCtx, w.MaxRequestsPerJob)

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

	stats, err := w.runIngestion(jobCtx, parent, pc, accessTok, publishedAfter, job)
	if err != nil {
		var rate *RateLimitError
		var tokInv *TokenInvalidError
		var quotaEx *QuotaExhaustedError
		var ceiling *JobBudgetExceededError
		switch {
		case errors.As(err, &quotaEx):
			until := quotaEx.RetryAt
			if until.IsZero() {
				until = time.Now().Add(time.Hour)
			}
			_ = w.DB.MarkChannelBackoff(parent, job.PlatformChannelID, until)
			_ = w.DB.MarkJobCompleted(parent, job.ID)
			return
		case errors.As(err, &ceiling):
			_ = w.DB.MarkJobCompleted(parent, job.ID)
			return
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
	w.adaptPollInterval(parent, pc, stats)
}

// stubYouTubeServer returns an httptest server that serves one video with
// two top-level comments and one reply. The playlist route mirrors the
// uploads-playlist discovery path; timestamps are relative to now so poll
// jobs do not stop at their own watermark.
func stubYouTubeServer(t *testing.T) *httptest.Server {
	freshVideo := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/channels":
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU1"}}}]}`))
		case r.URL.Path == "/playlistItems":
			_, _ = w.Write([]byte(`{"items":[
				{"contentDetails":{"videoId":"v1"},"snippet":{"publishedAt":"` + freshVideo + `","title":"Video One"}}
			]}`))
		case r.URL.Path == "/search":
			_, _ = w.Write([]byte(`{"items":[
				{"id":{"videoId":"v1"},"snippet":{"publishedAt":"` + freshVideo + `","title":"Video One"}}
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
	if _, ok := f.lastSyncedSet["pc1"]; !ok {
		t.Fatalf("normal poll must advance last_synced_at")
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

// ============================================================================
// Quota-optimization behavior (optimize-youtube-api-quota).
// ============================================================================

func newTestWorker(f *fakeWorkerDB, srv *httptest.Server) *Worker {
	return &Worker{
		DB:                        f,
		Client:                    &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL},
		JobDeadline:               30 * time.Second,
		Alerter:                   LogAlerter{},
		PollIntervalMinMinutes:    30,
		PollIntervalMaxMultiplier: 8,
	}
}

func TestWorker_UploadsPlaylistDiscoveryAndCaching(t *testing.T) {
	srv := stubYouTubeServer(t)
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA, f.tokenR = "tok", "ref"

	w := newTestWorker(f, srv)
	job := &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"}
	w.processJobNoLock(context.Background(), job)

	if got := f.playlists["pc1"]; got != "UU1" {
		t.Fatalf("uploads playlist not cached: %q", got)
	}
	total := 0
	for _, b := range f.insertCalls {
		total += len(b)
	}
	if total != 3 {
		t.Fatalf("expected 3 comments via playlist discovery, got %d", total)
	}
	// Thread structure unchanged (4.6): depth and parent mapping survive.
	var byID = map[string]*db.Comment{}
	for _, b := range f.insertCalls {
		for _, c := range b {
			byID[c.PlatformCommentID] = c
		}
	}
	if c := byID["c1"]; c == nil || c.ReplyDepth != 0 || c.ParentCommentID != nil {
		t.Fatalf("top-level c1 mapping wrong: %+v", c)
	}
	if c := byID["c1r"]; c == nil || c.ReplyDepth != 1 || c.ParentCommentID == nil || *c.ParentCommentID != "c1" {
		t.Fatalf("embedded reply c1r mapping wrong: %+v", c)
	}
	if c := byID["c1"]; c.VideoTitle == nil || *c.VideoTitle != "Video One" {
		t.Fatalf("video title not carried: %+v", c)
	}
}

func TestWorker_ReplyGatingSkipsCommentsList(t *testing.T) {
	var commentsCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/comments":
			commentsCalls++
			_, _ = w.Write([]byte(`{"items":[]}`))
		case "/channels":
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU1"}}}]}`))
		case "/playlistItems":
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"videoId":"v1"},"snippet":{"publishedAt":"` +
				time.Now().Add(-30*time.Minute).UTC().Format(time.RFC3339) + `"}}]}`))
		case "/commentThreads":
			// T1: 3 replies, 3 embedded inline → complete → no comments.list.
			// T2: 0 replies → no comments.list.
			_, _ = w.Write([]byte(`{"items":[
				{"snippet":{"topLevelComment":{"id":"t1","snippet":{"authorDisplayName":"A","textDisplay":"x","publishedAt":"2025-01-01T10:00:00Z"}},"totalReplyCount":3},
				 "replies":{"comments":[
					{"id":"r1","snippet":{"textDisplay":"a","publishedAt":"2025-01-01T11:00:00Z","parentId":"t1"}},
					{"id":"r2","snippet":{"textDisplay":"b","publishedAt":"2025-01-01T11:01:00Z","parentId":"t1"}},
					{"id":"r3","snippet":{"textDisplay":"c","publishedAt":"2025-01-01T11:02:00Z","parentId":"t1"}}]}},
				{"snippet":{"topLevelComment":{"id":"t2","snippet":{"authorDisplayName":"B","textDisplay":"y","publishedAt":"2025-01-01T10:05:00Z"}},"totalReplyCount":0}}
			]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA, f.tokenR = "tok", "ref"

	w := newTestWorker(f, srv)
	w.processJobNoLock(context.Background(), &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"})

	if commentsCalls != 0 {
		t.Fatalf("comments.list called %d times; gating must skip inline-complete and zero-reply threads", commentsCalls)
	}
	total := 0
	for _, b := range f.insertCalls {
		total += len(b)
	}
	if total != 5 {
		t.Fatalf("expected 5 comments (2 top + 3 inline), got %d", total)
	}
}

func TestWorker_SearchFallbackWhenPlaylistUnresolvable(t *testing.T) {
	var searchCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/channels":
			w.WriteHeader(http.StatusNotFound)
		case "/search":
			searchCalls++
			_, _ = w.Write([]byte(`{"items":[{"id":{"videoId":"v1"},"snippet":{"publishedAt":"2025-01-01T10:00:00Z"}}]}`))
		case "/commentThreads":
			_, _ = w.Write([]byte(`{"items":[{"snippet":{"topLevelComment":{"id":"c9","snippet":{"authorDisplayName":"A","textDisplay":"hi","publishedAt":"2025-01-01T10:00:00Z"}},"totalReplyCount":0}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA, f.tokenR = "tok", "ref"

	w := newTestWorker(f, srv)
	w.processJobNoLock(context.Background(), &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"})

	if searchCalls != 1 {
		t.Fatalf("expected exactly one bounded search.list fallback, got %d", searchCalls)
	}
	if len(f.completedJobs) != 1 {
		t.Fatalf("fallback ingestion must still complete job, got completed=%v failed=%v", f.completedJobs, f.failedJobs)
	}
}

func TestWorker_CheckpointResumeAndWatermark(t *testing.T) {
	var sawPageToken string
	wm := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/channels":
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU1"}}}]}`))
		case "/playlistItems":
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"videoId":"v1"},"snippet":{"publishedAt":"` +
				time.Now().Add(-30*time.Minute).UTC().Format(time.RFC3339) + `"}}]}`))
		case "/commentThreads":
			sawPageToken = r.URL.Query().Get("pageToken")
			// Resumed page: one stale (below watermark) + one new comment.
			_, _ = w.Write([]byte(`{"items":[
				{"snippet":{"topLevelComment":{"id":"old","snippet":{"authorDisplayName":"A","textDisplay":"stale","publishedAt":"2025-01-01T10:00:00Z"}},"totalReplyCount":0}},
				{"snippet":{"topLevelComment":{"id":"new","snippet":{"authorDisplayName":"B","textDisplay":"fresh","publishedAt":"2025-01-01T11:00:00Z"}},"totalReplyCount":0}}
			]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA, f.tokenR = "tok", "ref"
	// Durable mid-video checkpoint from a previous interrupted run.
	cp := "P2"
	f.videoStates[videoKey("pc1", "v1")] = &db.VideoSyncState{
		PlatformChannelID: "pc1", VideoID: "v1",
		CommentWatermarkAt: &wm, PageCheckpoint: &cp, Status: "active",
	}

	w := newTestWorker(f, srv)
	w.processJobNoLock(context.Background(), &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"})

	if sawPageToken != "P2" {
		t.Fatalf("resume must start at page checkpoint P2, got %q", sawPageToken)
	}
	var ids []string
	for _, b := range f.insertCalls {
		for _, c := range b {
			ids = append(ids, c.PlatformCommentID)
		}
	}
	if len(ids) != 1 || ids[0] != "new" {
		t.Fatalf("watermark must filter already-persisted comments, got %v", ids)
	}
	// Completed walk: checkpoint cleared, watermark advanced.
	st := f.videoStates[videoKey("pc1", "v1")]
	if st.PageCheckpoint != nil {
		t.Fatalf("checkpoint must clear after complete walk, got %q", *st.PageCheckpoint)
	}
	if st.CommentWatermarkAt == nil || !st.CommentWatermarkAt.Equal(time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("watermark not advanced: %+v", st.CommentWatermarkAt)
	}
}

func TestWorker_SkippedVideoStaysSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/channels":
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU1"}}}]}`))
		case "/playlistItems":
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"videoId":"v1"},"snippet":{"publishedAt":"` +
				time.Now().Add(-30*time.Minute).UTC().Format(time.RFC3339) + `"}}]}`))
		case "/commentThreads":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"errors":[{"reason":"commentsDisabled"}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA, f.tokenR = "tok", "ref"

	w := newTestWorker(f, srv)
	job := &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"}
	w.processJobNoLock(context.Background(), job)

	st := f.videoStates[videoKey("pc1", "v1")]
	if st == nil || st.Status != "skipped" {
		t.Fatalf("commentsDisabled video must persist as skipped, got %+v", st)
	}
	if len(f.failedJobs) != 0 || len(f.completedJobs) != 1 {
		t.Fatalf("comments-disabled is permanent, not a failure: completed=%v failed=%v", f.completedJobs, f.failedJobs)
	}
}

func TestWorker_AdaptivePolling(t *testing.T) {
	// Empty channel: interval doubles toward the multiplier cap.
	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	cur := 60
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last, PollIntervalMinutes: &cur}
	f.tokenA, f.tokenR = "tok", "ref"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/channels":
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU1"}}}]}`))
		case "/playlistItems":
			_, _ = w.Write([]byte(`{"items":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	w := newTestWorker(f, srv)
	w.processJobNoLock(context.Background(), &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"})
	if got := f.pollSet["pc1"]; got != 120 {
		t.Fatalf("inactive channel interval should double 60→120, got %d", got)
	}

	// Active channel: halves back toward the configured floor.
	f2 := newFakeWorkerDB()
	f2.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last, PollIntervalMinutes: &cur}
	f2.tokenA, f2.tokenR = "tok", "ref"
	w2 := newTestWorker(f2, stubYouTubeServer(t))
	w2.processJobNoLock(context.Background(), &db.IngestionJob{ID: "j2", PlatformChannelID: "pc1", JobType: "poll"})
	if got := f2.pollSet["pc1"]; got != 30 {
		t.Fatalf("active channel interval should halve 60→30, got %d", got)
	}
}

func TestWorker_QuotaExhaustedDefersWithoutRetryBurn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA, f.tokenR = "tok", "ref"

	w := newTestWorker(f, srv)
	// Budget of 1: channels.list consumes it, the playlist page is
	// suppressed locally (never leaves the process).
	w.Client.Quota = NewQuotaAccounting(1)
	w.processJobNoLock(context.Background(), &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"})

	if len(f.failedJobs) != 0 {
		t.Fatalf("quota exhaustion must not consume retry budget, failed=%v", f.failedJobs)
	}
	if len(f.completedJobs) != 1 {
		t.Fatalf("expected job completed+deferred, got completed=%v", f.completedJobs)
	}
	if _, ok := f.backoffSet["pc1"]; !ok {
		t.Fatalf("expected channel deferred via backoff")
	}
}

func TestWorker_JobRequestCeilingDefers(t *testing.T) {
	srv := stubYouTubeServer(t)
	defer srv.Close()

	f := newFakeWorkerDB()
	last := time.Now().Add(-1 * time.Hour)
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1", LastSyncedAt: &last}
	f.tokenA, f.tokenR = "tok", "ref"

	w := newTestWorker(f, srv)
	w.MaxRequestsPerJob = 1 // /channels consumes it; discovery is cut off
	w.processJobNoLock(context.Background(), &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "poll"})

	if len(f.failedJobs) != 0 {
		t.Fatalf("ceiling is a clean stop, not a failure: failed=%v", f.failedJobs)
	}
	if len(f.completedJobs) != 1 {
		t.Fatalf("expected completed, got %v", f.completedJobs)
	}
}

// TestWorker_PartialBackfillDoesNotAdvanceWatermark proves the ingestion-loss
// invariant: a backfill that stops at its video ceiling must leave
// last_synced_at nil so the scheduler keeps classifying the channel as a
// backfill, and a later run still discovers the older, un-scanned video.
func TestWorker_PartialBackfillDoesNotAdvanceWatermark(t *testing.T) {
	v1pub := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	v2pub := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/channels":
			_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU1"}}}]}`))
		case "/playlistItems":
			// Newest-first: v1 then the older v2.
			_, _ = w.Write([]byte(`{"items":[
				{"contentDetails":{"videoId":"v1"},"snippet":{"publishedAt":"` + v1pub + `","title":"Video One"}},
				{"contentDetails":{"videoId":"v2"},"snippet":{"publishedAt":"` + v2pub + `","title":"Video Two"}}
			]}`))
		case "/commentThreads":
			vid := r.URL.Query().Get("videoId")
			_, _ = w.Write([]byte(`{"items":[{"snippet":{"topLevelComment":{"id":"c-` + vid + `","snippet":{"authorDisplayName":"A","textDisplay":"hi","publishedAt":"` + v1pub + `"}},"totalReplyCount":0}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	f := newFakeWorkerDB()
	f.channels["pc1"] = &db.PlatformChannel{ID: "pc1", Platform: "youtube", PlatformChannelID: "UC1"} // LastSyncedAt nil
	f.tokenA, f.tokenR = "tok", "ref"

	// First backfill: ceiling of 1 stops after v1, leaving v2 un-scanned.
	w := newTestWorker(f, srv)
	w.MaxVideosPerJob = 1
	w.processJobNoLock(context.Background(), &db.IngestionJob{ID: "j1", PlatformChannelID: "pc1", JobType: "backfill"})

	if len(f.completedJobs) != 1 {
		t.Fatalf("ceiling stop should complete the job, got completed=%v failed=%v", f.completedJobs, f.failedJobs)
	}
	if ts, ok := f.lastSyncedSet["pc1"]; ok {
		t.Fatalf("partial backfill must NOT advance last_synced_at, got %v", ts)
	}

	// The scheduler must still classify the channel as a backfill.
	res, err := NewScheduler(f, 30).Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Backfills != 1 || res.Polls != 0 {
		t.Fatalf("incomplete backfill must re-enqueue as backfill, got %+v", res)
	}

	// A later backfill with room for both videos discovers the older v2
	// rather than skipping it.
	w2 := newTestWorker(f, srv)
	w2.MaxVideosPerJob = 2
	w2.processJobNoLock(context.Background(), &db.IngestionJob{ID: "j2", PlatformChannelID: "pc1", JobType: "backfill"})

	var ids []string
	for _, b := range f.insertCalls {
		for _, c := range b {
			ids = append(ids, c.PlatformCommentID)
		}
	}
	foundV2 := false
	for _, id := range ids {
		if id == "c-v2" {
			foundV2 = true
		}
	}
	if !foundV2 {
		t.Fatalf("older video v2 must be discovered on the resumed backfill, got %v", ids)
	}
	// Now the full window is walked, so the channel is promoted to polling.
	if _, ok := f.lastSyncedSet["pc1"]; !ok {
		t.Fatalf("completed backfill must advance last_synced_at")
	}
}

// Compile-time proof the fake covers the optional interface.
var _ SyncStateDB = (*fakeWorkerDB)(nil)
