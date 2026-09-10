package oauth

import (
	"testing"

	"noumena/internal/config"
)

func TestValidateIDClaims(t *testing.T) {
	prev := config.Default.GoogleSSOClientID
	config.Default.GoogleSSOClientID = "test-client"
	t.Cleanup(func() { config.Default.GoogleSSOClientID = prev })

	valid := func() *IDClaims {
		return &IDClaims{
			Sub:           "sub-123",
			Email:         "user@example.com",
			EmailVerified: true,
			Aud:           "test-client",
			Iss:           "https://accounts.google.com",
		}
	}

	cases := []struct {
		name    string
		mutate  func(c *IDClaims)
		wantErr bool
	}{
		{"valid", func(c *IDClaims) {}, false},
		{"valid legacy issuer", func(c *IDClaims) { c.Iss = "accounts.google.com" }, false},
		{"email not verified", func(c *IDClaims) { c.EmailVerified = false }, true},
		{"missing email", func(c *IDClaims) { c.Email = "" }, true},
		{"missing sub", func(c *IDClaims) { c.Sub = "" }, true},
		{"aud mismatch", func(c *IDClaims) { c.Aud = "other-client" }, true},
		{"iss mismatch", func(c *IDClaims) { c.Iss = "https://evil.example.com" }, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(c)
			err := validateIDClaims(c)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
		})
	}
}
