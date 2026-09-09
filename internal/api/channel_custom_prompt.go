package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]+>`)
	scriptTagRe  = regexp.MustCompile(`(?i)<script`)
	maxPromptLen = 500
)

func ValidateCustomPrompt(prompt string) error {
	if len(prompt) > maxPromptLen {
		return &apiError{code: "prompt_too_long", message: "custom prompt must be at most 500 characters"}
	}
	if scriptTagRe.MatchString(prompt) {
		return &apiError{code: "prompt_invalid", message: "custom prompt must not contain script tags"}
	}
	if htmlTagRe.MatchString(prompt) {
		return &apiError{code: "prompt_invalid", message: "custom prompt must not contain HTML"}
	}
	return nil
}

type apiError struct {
	code    string
	message string
}

func (e *apiError) Error() string { return e.message }

func (h *ChannelHandler) UpdateCustomPrompt(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	channelID := chi.URLParam(r, "id")

	var req struct {
		CustomPrompt *string `json:"custom_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CustomPrompt != nil {
		prompt := strings.TrimSpace(*req.CustomPrompt)
		if prompt != "" {
			if err := ValidateCustomPrompt(prompt); err != nil {
				ae := err.(*apiError)
				writeJSON(w, http.StatusBadRequest, map[string]string{"code": ae.code, "error": ae.message})
				return
			}
		} else {
			req.CustomPrompt = nil
		}
	}

	if err := h.Queries.UpdateChannelCustomPrompt(r.Context(), userID, channelID, req.CustomPrompt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update custom prompt")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channel_id":    channelID,
		"custom_prompt": req.CustomPrompt,
	})
}
