package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ChannelInfo is the minimal YouTube channel metadata returned by
// `channels?mine=true&part=snippet`. It is used at OAuth-exchange time to
// resolve the authorizing user's primary channel id and display name.
type ChannelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`

	// Title is a legacy alias for DisplayName retained for callers that
	// still reference the old scaffolding's field name.
	Title string `json:"-"`
}

// Comment is the shape produced by the YouTube ingestion poller. The full
// ingestion implementation (mapping snippets to comments, threading, etc.)
// lives in Spec 03; this type pins the contract used by callers in this
// change.
type Comment struct {
	PlatformCommentID string
	ParentCommentID   *string
	AuthorHandle      string
	Body              string
	PostedAt          time.Time
}

// FetchChannelInfo calls `GET /youtube/v3/channels?mine=true&part=snippet`
// using the supplied access token. It never logs or persists the token.
func FetchChannelInfo(ctx context.Context, accessToken string) (*ChannelInfo, error) {
	if accessToken == "" {
		return nil, errors.New("access token required")
	}
	endpoint := "https://www.googleapis.com/youtube/v3/channels?mine=true&part=snippet"

	doReq := func(ctx context.Context) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		client := &http.Client{Timeout: 15 * time.Second}
		return client.Do(req)
	}

	var body []byte
	err := withBackoff(ctx, func(ctx context.Context) (retry bool, retryAfter time.Duration, err error) {
		resp, err := doReq(ctx)
		if err != nil {
			// Network errors are treated as retryable.
			return true, 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
			return true, retryAfter, fmt.Errorf("youtube channels.list: HTTP %d", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			return false, 0, fmt.Errorf("youtube channels.list: HTTP %d", resp.StatusCode)
		}
		body, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return false, 0, err
	})
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode channels.list: %w", err)
	}
	if len(parsed.Items) == 0 {
		return nil, errors.New("no youtube channel found for this account")
	}
	log.Printf("youtube: fetched authorized channel info id=%s title=%q", parsed.Items[0].ID, parsed.Items[0].Snippet.Title)
	return &ChannelInfo{
		ID:          parsed.Items[0].ID,
		DisplayName: parsed.Items[0].Snippet.Title,
		Title:       parsed.Items[0].Snippet.Title,
	}, nil
}

// withBackoff runs op with exponential backoff: 1s, 2s, 4s, 8s, 16s. It
// honours an explicit retryAfter when present.
func withBackoff(ctx context.Context, op func(ctx context.Context) (retry bool, retryAfter time.Duration, err error)) error {
	delays := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	var lastErr error
	for attempt := 0; ; attempt++ {
		retry, retryAfter, err := op(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry || attempt >= len(delays) {
			return lastErr
		}
		wait := delays[attempt]
		if retryAfter > 0 {
			wait = retryAfter
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
