package ingestion

import (
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"noumena/internal/auth"
	"noumena/internal/db"
	"noumena/internal/oauth"
)

// FetchAndStoreComments connects to YouTube, fetches recent comments for the channel, and stores them.
func FetchAndStoreComments(ctx context.Context, q db.DBQuery, pc *db.PlatformChannel) error {
	accessEnc, refreshEnc, err := q.GetIngestionToken(ctx, pc.ID)
	if err != nil {
		return fmt.Errorf("no active ingestion token available: %w", err)
	}

	access, err := auth.Decrypt(accessEnc)
	if err != nil {
		return err
	}
	refresh, err := auth.Decrypt(refreshEnc)
	if err != nil {
		return err
	}

	// We reconstruct the token. Expiry isn't strictly needed here as the Google client will try to refresh if it gets a 401
	// provided the refresh token is present. (In production we should save and use the expiry).
	token := &oauth2.Token{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
	}

	if oauth.YouTubeConfig == nil {
		oauth.InitOAuth()
	}

	client := oauth.YouTubeConfig.Client(ctx, token)
	ytService, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return err
	}

	call := ytService.CommentThreads.List([]string{"snippet"}).AllThreadsRelatedToChannelId(pc.PlatformChannelID).MaxResults(50)
	response, err := call.Do()
	if err != nil {
		// If authorization fails, the worker can promote the next token.
		return fmt.Errorf("youtube api call failed: %w", err)
	}

	for _, item := range response.Items {
		tlc := item.Snippet.TopLevelComment
		if tlc == nil {
			continue
		}

		postedAt := tlc.Snippet.PublishedAt // simplify parse error handling for this prototype

		t, err := parseTime(postedAt)
		if err != nil {
			continue
		}

		comment := &db.Comment{
			PlatformChannelID: pc.ID,
			PlatformCommentID: tlc.Id,
			AuthorHandle:      &tlc.Snippet.AuthorDisplayName,
			Body:              tlc.Snippet.TextOriginal,
			PostedAt:          t,
		}

		if dbErr := q.InsertComment(ctx, comment); dbErr != nil {
			log.Printf("Failed to insert comment %s: %v", tlc.Id, dbErr)
		}
	}

	return nil
}

// Simple time parser helper for ISO8601 subset that YT sends
func parseTime(dt string) (time.Time, error) {
	return time.Parse(time.RFC3339, dt)
}
