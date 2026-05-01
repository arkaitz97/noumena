package oauth

import (
	"context"
	"errors"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

var YouTubeConfig *oauth2.Config

func InitOAuth() {
	YouTubeConfig = &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("FRONTEND_URL") + "/api/channels/callback/youtube", // Usually points to API directly in this design
		Scopes: []string{
			"https://www.googleapis.com/auth/youtube.readonly",
		},
		Endpoint: google.Endpoint,
	}
}

type YTChannelInfo struct {
	ID    string
	Title string
}

// ExchangeAndFetchChannel swaps the code for tokens and calls YouTube API to get the channel ID
func ExchangeAndFetchChannel(ctx context.Context, code string) (*oauth2.Token, *YTChannelInfo, error) {
	if YouTubeConfig == nil {
		InitOAuth()
	}

	token, err := YouTubeConfig.Exchange(ctx, code)
	if err != nil {
		return nil, nil, err
	}

	client := YouTubeConfig.Client(ctx, token)
	ytService, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, nil, err
	}

	call := ytService.Channels.List([]string{"id", "snippet"}).Mine(true)
	response, err := call.Do()
	if err != nil {
		return nil, nil, err
	}

	if len(response.Items) == 0 {
		return nil, nil, errors.New("no youtube channel found for this account")
	}

	channel := response.Items[0]
	return token, &YTChannelInfo{
		ID:    channel.Id,
		Title: channel.Snippet.Title,
	}, nil
}
