package oauth

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"noumena/internal/config"
	"noumena/internal/ingestion"
)

// youtubeReadonlyScope is the scope demanded by the YouTube channel-tracking
// flow. It is intentionally narrower than the SSO scope used by `google_sso.go`.
const youtubeReadonlyScope = "https://www.googleapis.com/auth/youtube.readonly"

var (
	youtubeOAuthMu     sync.Mutex
	youtubeOAuthConfig *oauth2.Config

	// YouTubeConfig is exported for tests/callers that want to inspect the
	// underlying oauth2.Config (e.g. ingestion's NewService(client)).
	// Callers should prefer GetYouTubeOAuthConfig() to ensure lazy init.
	YouTubeConfig *oauth2.Config
)

// GetYouTubeOAuthConfig returns the lazily-initialized YouTube OAuth client.
// It is safe for concurrent use.
func GetYouTubeOAuthConfig() *oauth2.Config {
	youtubeOAuthMu.Lock()
	defer youtubeOAuthMu.Unlock()
	if youtubeOAuthConfig == nil {
		youtubeOAuthConfig = &oauth2.Config{
			ClientID:     config.Default.YouTubeOAuthClientID,
			ClientSecret: config.Default.YouTubeOAuthClientSecret,
			RedirectURL:  config.Default.YouTubeOAuthRedirectURL,
			Scopes:       []string{youtubeReadonlyScope},
			Endpoint:     google.Endpoint,
		}
		YouTubeConfig = youtubeOAuthConfig
	}
	return youtubeOAuthConfig
}

// InitOAuth is retained for backwards compatibility with the original
// scaffolding. It re-initializes the YouTube OAuth config from the current
// process configuration.
func InitOAuth() {
	youtubeOAuthMu.Lock()
	youtubeOAuthConfig = nil
	YouTubeConfig = nil
	youtubeOAuthMu.Unlock()
	GetYouTubeOAuthConfig()
}

// BuildAuthURL returns the YouTube OAuth 2.0 authorization URL bound to the
// given CSRF state. It always sets `access_type=offline` and `prompt=consent`
// so Google issues a refresh token even on repeat consents.
func BuildAuthURL(state string) string {
	cfg := GetYouTubeOAuthConfig()
	return cfg.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
}

// ErrNoRefreshToken is returned by ExchangeCode when Google omits a refresh
// token from a successful response. Callers MUST surface this to the user
// rather than persisting an unrenewable tracker.
var ErrNoRefreshToken = errors.New("no refresh token returned by google")

// ExchangeCode swaps a YouTube authorization code for an OAuth token pair.
// It rejects responses that lack a refresh token.
func ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	cfg := GetYouTubeOAuthConfig()
	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	if token == nil || token.RefreshToken == "" {
		return nil, ErrNoRefreshToken
	}
	return token, nil
}

// YTChannelInfo is the legacy alias used by older callers; kept for backward
// compatibility with the original scaffolding.
//
// Deprecated: Use ingestion.ChannelInfo via ExchangeAndFetchChannel.
type YTChannelInfo = ingestion.ChannelInfo

// ExchangeAndFetchChannel exchanges the OAuth code and immediately fetches
// the authorizing user's primary YouTube channel metadata.
func ExchangeAndFetchChannel(ctx context.Context, code string) (*oauth2.Token, *ingestion.ChannelInfo, error) {
	token, err := ExchangeCode(ctx, code)
	if err != nil {
		return nil, nil, err
	}

	info, err := ingestion.FetchChannelInfo(ctx, token.AccessToken)
	if err != nil {
		return nil, nil, err
	}
	return token, info, nil
}
