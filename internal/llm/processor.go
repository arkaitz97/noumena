package llm

import (
	"context"
	"log"
	"time"

	"noumena/internal/db"
)

func StartBatchProcessor(ctx context.Context, q db.DBQuery, apiKey string) {
	if apiKey == "" {
		log.Println("Batch processor disabled: no OpenAI API key")
		return
	}

	client := NewClient(apiKey)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Batch processor stopping")
			return
		case <-ticker.C:
			processBatch(ctx, q, client)
		}
	}
}

func processBatch(ctx context.Context, q db.DBQuery, client *Client) {
	// Fetch up to 50 unprocessed comments
	comments, err := q.GetUnprocessedComments(ctx, 50)
	if err != nil {
		log.Printf("BatchProcessor: error fetching unprocessed comments: %v", err)
		return
	}

	if len(comments) == 0 {
		return
	}

	log.Printf("BatchProcessor: Processing batch of %d comments via LLM", len(comments))

	var texts []string
	for _, c := range comments {
		texts = append(texts, c.Body)
	}

	results, err := client.ClassifyBatch(ctx, texts)
	if err != nil {
		log.Printf("BatchProcessor: error from LLM classification: %v", err)
		return
	}

	if len(results) != len(comments) {
		log.Printf("BatchProcessor: mismatch in results count (%d) and comments count (%d)", len(results), len(comments))
		return
	}

	// Update DB
	for i, res := range results {
		c := comments[i]

		if res.Error != "" {
			log.Printf("BatchProcessor: LLM returned error for comment %s: %s", c.ID, res.Error)
			_ = q.UpdateCommentLLMData(ctx, c.ID, "Other", "", 0, 0)
			continue
		}

		err = q.UpdateCommentLLMData(ctx, c.ID, res.Category, res.Topic, res.SentimentScore, res.Confidence)
		if err != nil {
			log.Printf("BatchProcessor: error updating LLM data for comment %s: %v", c.ID, err)
		}
	}
}
