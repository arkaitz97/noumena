package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"

	"noumena/internal/auth"
	"noumena/internal/db"
	"noumena/internal/oauth"
)

type ChannelHandler struct {
	Queries db.DBQuery
}

func (h *ChannelHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(string)

	channels, err := h.Queries.ListChannelsForUser(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to get channels", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

func (h *ChannelHandler) ConnectYouTube(w http.ResponseWriter, r *http.Request) {
	if oauth.YouTubeConfig == nil {
		oauth.InitOAuth()
	}
	
	// In production, pass a secure signed state/CSRF token here.
	url := oauth.YouTubeConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	
	// We normally might just redirect the user directly from the server:
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h *ChannelHandler) CallbackYouTube(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "code not found", http.StatusBadRequest)
		return
	}

	// Wait, we need the userID. If this is a direct OAuth callback, the browser won't easily send the JWT Bearer header.
	// We can use a cookie or pass the user ID in the state token. 
	// For this prototype, we'll assume the client app handles the callback and POSTs the code to us via a different route (e.g. /api/channels/youtube/exchange), or we use an auth cookie.
	// Since we use Bearer tokens primarily, let's treat ConnectYouTube as returning the URL, and the client opens it. 
	// The client then gets the code and POSTs it back to us, OR we set a secure cookie before redirecting.
	// Let's implement the POST exchange pattern for SPA compatibility.
	
	http.Error(w, "Use POST /exchange instead", http.StatusBadRequest)
}

// ExchangeYouTubeCode handles a POST request from the frontend containing the OAuth code.
func (h *ChannelHandler) ExchangeYouTubeCode(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(string)

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Code == "" {
		http.Error(w, "invalid code", http.StatusBadRequest)
		return
	}

	token, ytChannel, err := oauth.ExchangeAndFetchChannel(r.Context(), payload.Code)
	if err != nil {
		http.Error(w, "failed to exchange token", http.StatusInternalServerError)
		return
	}

	accessEnc, _ := auth.Encrypt(token.AccessToken)
	refreshEnc, _ := auth.Encrypt(token.RefreshToken)
	
	err = h.Queries.ConnectChannelTx(r.Context(), userID, "youtube", ytChannel.ID, ytChannel.Title, accessEnc, refreshEnc, &token.Expiry)
	if err != nil {
		http.Error(w, "failed to save channel to database", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "channel connected successfully",
		"id": ytChannel.ID,
		"title": ytChannel.Title,
	})
}

func (h *ChannelHandler) Pause(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(string)
	channelID := chi.URLParam(r, "id")

	err := h.Queries.PauseChannelTracking(r.Context(), userID, channelID)
	if err != nil {
		http.Error(w, "failed to pause channel", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "channel paused successfully",
	})
}

