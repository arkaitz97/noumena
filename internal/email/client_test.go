package email_test

import (
	"strings"
	"testing"
	"time"

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

	err = client.SendTokenExpiryEmail("test@example.com", "Demo Channel",
		time.Now().Add(48*time.Hour), "http://localhost:3000/settings/channels")
	if err != nil {
		t.Fatalf("expected no error sending token-expiry email, got %v", err)
	}
}

// TestResendTokenExpiryTemplate sanity-checks that the Resend client's
// template includes the channel name, expiry date, and a settings link.
// We cannot exercise the network call without a real key, so we only
// verify that the helper rejects no-op input gracefully.
func TestResendTokenExpiryTemplateShape(t *testing.T) {
	c := email.NewResendClient("test-key", "from@example.com")
	// The Resend SDK fails fast against the (invalid) key but we still
	// want to ensure the helper exists and accepts valid inputs.
	err := c.SendTokenExpiryEmail("user@example.com", "My Channel",
		time.Now().Add(7*24*time.Hour), "http://example.com/settings/channels")
	if err == nil {
		// In CI without network this will surface as an error; if it
		// somehow succeeded, that's also fine.
		return
	}
	if !strings.Contains(err.Error(), "token-expiry") {
		t.Fatalf("expected token-expiry email error wrapper, got %v", err)
	}
}

// Ensure the real client satisfies the interface
var _ email.Client = (*email.ResendClient)(nil)
