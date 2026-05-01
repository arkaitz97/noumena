package api

import (
	"encoding/json"
	"net/http"
	"time"

	"noumena/internal/auth"
	"noumena/internal/db"
	"noumena/internal/email"
)

type AuthHandler struct {
	Queries     db.DBQuery
	EmailClient email.Client
}

type AuthRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" {
		http.Error(w, "email and password are required", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "failed to process password", http.StatusInternalServerError)
		return
	}

	user, err := h.Queries.CreateUser(r.Context(), req.Email, hash)
	if err != nil {
		// Basic error handling for duplicate email
		http.Error(w, "email already registered or database error", http.StatusConflict)
		return
	}

	access, refresh, err := auth.GenerateTokens(user.ID)
	if err != nil {
		http.Error(w, "failed to generate tokens", http.StatusInternalServerError)
		return
	}

	setRefreshTokenCookie(w, refresh)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if h.EmailClient != nil {
		// Mock a 6 digit code for now, or just use a static one
		_ = h.EmailClient.SendVerificationEmail(req.Email, "123456")
	}

	json.NewEncoder(w).Encode(map[string]string{
		"access_token": access,
		"message":      "check inbox to verify account",
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.Queries.GetUserByEmail(r.Context(), req.Email)
	if err != nil || user == nil || user.PasswordHash == nil {
		http.Error(w, "invalid email or password", http.StatusUnauthorized)
		return
	}

	if !auth.CheckPasswordHash(req.Password, *user.PasswordHash) {
		http.Error(w, "invalid email or password", http.StatusUnauthorized)
		return
	}

	access, refresh, err := auth.GenerateTokens(user.ID)
	if err != nil {
		http.Error(w, "failed to generate tokens", http.StatusInternalServerError)
		return
	}

	setRefreshTokenCookie(w, refresh)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token": access,
	})
}

func setRefreshTokenCookie(w http.ResponseWriter, refresh string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refresh,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}
