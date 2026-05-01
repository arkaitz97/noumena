package auth_test

import (
	"testing"
	"time"

	"noumena/internal/auth"
)

func TestPasswordHashing(t *testing.T) {
	password := "SecureP@ssw0rd123"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	if hash == password {
		t.Errorf("Hash should not equal the raw password")
	}

	if !auth.CheckPasswordHash(password, hash) {
		t.Errorf("Failed to verify correct password")
	}

	if auth.CheckPasswordHash("WrongPassword!", hash) {
		t.Errorf("Verified incorrect password")
	}
}

func TestJWTGenerationAndValidation(t *testing.T) {
	userID := "user-123"

	access, refresh, err := auth.GenerateTokens(userID)
	if err != nil {
		t.Fatalf("Failed to generate tokens: %v", err)
	}

	if access == "" || refresh == "" {
		t.Fatalf("Tokens should not be empty")
	}

	// Validate Access Token
	claims, err := auth.ValidateToken(access)
	if err != nil {
		t.Fatalf("Failed to validate access token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("Expected UserID %s, got %s", userID, claims.UserID)
	}

	if claims.ExpiresAt.Time.Before(time.Now()) {
		t.Errorf("Access token is already expired")
	}
}
