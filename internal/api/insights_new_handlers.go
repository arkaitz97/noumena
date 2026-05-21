package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"noumena/internal/db"
)

func (h *InsightsHandler) SearchComments(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(string)
	q := r.URL.Query().Get("q")
	if len(q) < 3 {
		writeJSONError(w, http.StatusBadRequest, "search query must be at least 3 characters")
		return
	}

	comments, err := h.Queries.SearchComments(r.Context(), userID, q)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "search_failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"comments": comments,
		"total":    len(comments),
	})
}

func (h *InsightsHandler) BulkStarComments(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(string)

	var req struct {
		CommentIDs []string `json:"comment_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	count, err := h.Queries.BulkStarComments(r.Context(), userID, req.CommentIDs)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "bulk_star_failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"starred": count})
}

func (h *InsightsHandler) BulkUnstarComments(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(string)

	var req struct {
		CommentIDs []string `json:"comment_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	count, err := h.Queries.BulkUnstarComments(r.Context(), userID, req.CommentIDs)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "bulk_unstar_failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"unstarred": count})
}

func (h *InsightsHandler) GetCommentThread(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(string)
	commentID := chi.URLParam(r, "id")

	thread, err := h.Queries.GetCommentThread(r.Context(), userID, commentID, 3)
	if err != nil {
		if err == db.ErrNotFound {
			writeJSONError(w, http.StatusNotFound, "comment_not_found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "thread_failed")
		return
	}

	writeJSON(w, http.StatusOK, thread)
}
