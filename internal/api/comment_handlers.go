package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"noumena/internal/db"
)

type CommentHandler struct {
	Queries db.DBQuery
}

func (h *CommentHandler) Feed(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	comments, err := h.Queries.GetCommentFeed(r.Context(), userID, limit, offset)
	if err != nil {
		http.Error(w, "failed to get comments feed", http.StatusInternalServerError)
		return
	}

	if comments == nil {
		comments = []*db.Comment{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

func (h *CommentHandler) Star(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	commentID := chi.URLParam(r, "id")
	if commentID == "" {
		http.Error(w, "comment ID is required", http.StatusBadRequest)
		return
	}

	err := h.Queries.StarComment(r.Context(), userID, commentID)
	if err != nil {
		http.Error(w, "failed to star comment", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "comment " + commentID + " starred",
	})
}
