package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"noumena/internal/db"
)

type digestMockQuery struct {
	db.DBQuery
	users       []*db.User
	comments    []*db.Comment
	insertCount int
}

func (m *digestMockQuery) GetUsersForDigest(ctx context.Context) ([]*db.User, error) {
	return m.users, nil
}

func (m *digestMockQuery) GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*db.Comment, error) {
	return m.comments, nil
}

func (m *digestMockQuery) InsertWeeklyDigest(ctx context.Context, digest *db.WeeklyDigest) error {
	m.insertCount++
	return nil
}

func TestGenerateDigests(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices": [{"message": {"content": "{\"health_score\":90}"}}]}`))
	}))
	defer mockServer.Close()

	client := NewClient("test")
	client.baseURL = mockServer.URL

	mq := &digestMockQuery{
		users:    []*db.User{{ID: "u1"}},
		comments: make([]*db.Comment, 10), // Need 10 to trigger
	}
	for i := 0; i < 10; i++ {
		mq.comments[i] = &db.Comment{ID: "c"}
	}

	generateDigests(context.Background(), mq, client, nil)

	if mq.insertCount != 1 {
		t.Errorf("expected 1 digest inserted, got %d", mq.insertCount)
	}
}

func TestStartDigestWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	StartDigestWorker(ctx, nil, "apikey", nil)
}

func TestStartDigestWorker_NoKey(t *testing.T) {
	StartDigestWorker(context.Background(), nil, "", nil)
}
