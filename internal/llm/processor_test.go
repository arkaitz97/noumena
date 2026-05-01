package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"noumena/internal/db"
)

type mockQuery struct {
	db.DBQuery
	unprocessed []*db.Comment
	updates     map[string]string
}

func (m *mockQuery) GetUnprocessedComments(ctx context.Context, limit int) ([]*db.Comment, error) {
	if len(m.unprocessed) > limit {
		return m.unprocessed[:limit], nil
	}
	return m.unprocessed, nil
}

func (m *mockQuery) UpdateCommentLLMData(ctx context.Context, id, category, topic string, sentiment, confidence float64) error {
	if m.updates == nil {
		m.updates = make(map[string]string)
	}
	m.updates[id] = category
	return nil
}

func TestProcessBatch(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"choices": [{
				"message": {
					"content": "{\"results\":[{\"category\":\"Praise\",\"topic\":\"\",\"sentiment_score\":1.0,\"confidence\":0.9}]}"
				}
			}]
		}`))
	}))
	defer mockServer.Close()

	client := NewClient("test")
	client.baseURL = mockServer.URL

	mq := &mockQuery{
		unprocessed: []*db.Comment{{ID: "c1", Body: "Great video!"}},
	}

	processBatch(context.Background(), mq, client)

	if mq.updates["c1"] != "Praise" {
		t.Errorf("expected Praise, got %v", mq.updates["c1"])
	}
}

func TestStartBatchProcessor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately to not block
	StartBatchProcessor(ctx, nil, "apikey")
}

func TestStartBatchProcessor_NoKey(t *testing.T) {
	StartBatchProcessor(context.Background(), nil, "")
}
