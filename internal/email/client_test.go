package email_test

import (
	"testing"

	"noumena/internal/email"
)

func TestMockClient(t *testing.T) {
	client := email.NewMockClient()

	// Just ensure the mock client doesn't panic when we call its methods
	err := client.SendVerificationEmail("test@example.com", "123456")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	err = client.SendWelcomeEmail("test@example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	err = client.SendDigestEmail("test@example.com", "<h1>Digest</h1>")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	err = client.SendLockedAccountEmail("test@example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// Ensure the real client satisfies the interface
var _ email.Client = (*email.ResendClient)(nil)
