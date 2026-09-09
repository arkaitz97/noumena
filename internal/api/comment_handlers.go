package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"noumena/internal/db"
)

type CommentHandler struct {
	Queries db.DBQuery
}

func (h *CommentHandler) Feed(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	comments, err := h.Queries.GetCommentFeed(r.Context(), userID, 50, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "feed_failed")
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

func (h *CommentHandler) Star(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	commentID := chi.URLParam(r, "id")
	if err := h.Queries.StarComment(r.Context(), userID, commentID); err != nil {
		writeError(w, http.StatusInternalServerError, "star_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"comment_id": commentID, "starred": "true"})
}
