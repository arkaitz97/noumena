package llm

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"noumena/internal/db"
	"noumena/internal/email"
)

func StartDigestWorker(ctx context.Context, q db.DBQuery, apiKey string, emailClient email.Client) {
	if apiKey == "" {
		log.Println("Digest worker disabled: no OpenAI API key")
		return
	}

	client := NewClient(apiKey)

	// Check every hour if it's time to generate digests (Monday 08:00 UTC)
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Digest worker stopping")
			return
		case <-ticker.C:
			now := time.Now().UTC()
			if now.Weekday() == time.Monday && now.Hour() == 8 {
				generateDigests(ctx, q, client, emailClient)
			}
		}
	}
}

func generateDigests(ctx context.Context, q db.DBQuery, client *Client, emailClient email.Client) {
	users, err := q.GetUsersForDigest(ctx)
	if err != nil {
		log.Printf("DigestWorker: Error getting users for digest: %v", err)
		return
	}

	oneWeekAgo := time.Now().UTC().AddDate(0, 0, -7)

	for _, u := range users {
		comments, err := q.GetRecentProcessedCommentsForUser(ctx, u.ID, oneWeekAgo)
		if err != nil {
			log.Printf("DigestWorker: Error getting comments for user %s: %v", u.ID, err)
			continue
		}

		if len(comments) < 10 {
			// Not enough data for insight generation this week
			continue
		}

		commentsJSONBytes, _ := json.Marshal(comments)
		commentsJSON := string(commentsJSONBytes)

		log.Printf("DigestWorker: Generating digest for user %s (%d comments)", u.ID, len(comments))

		result, err := client.GenerateDigest(ctx, commentsJSON)
		if err != nil {
			log.Printf("DigestWorker: Error generating digest for user %s: %v", u.ID, err)
			continue
		}

		contentBytes, _ := json.Marshal(result)

		digest := &db.WeeklyDigest{
			UserID:        u.ID,
			WeekStartDate: oneWeekAgo.Format("2006-01-02"),
			Status:        "generated",
			Content:       string(contentBytes),
		}

		if err := q.InsertWeeklyDigest(ctx, digest); err != nil {
			log.Printf("DigestWorker: Error inserting digest for user %s: %v", u.ID, err)
		} else {
			if emailClient != nil {
				// We need the user's email, which we can get from user struct
				_ = emailClient.SendDigestEmail(u.Email, digest.Content)
			}
		}
	}
}
