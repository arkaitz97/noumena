package email

import (
	"fmt"
	"log"

	"github.com/resend/resend-go/v2"
)

// Client defines the interface for an email client
type Client interface {
	SendVerificationEmail(to, code string) error
	SendWelcomeEmail(to string) error
	SendDigestEmail(to, digestContent string) error
	SendLockedAccountEmail(to string) error
}

// ResendClient implements the Client interface using Resend
type ResendClient struct {
	client *resend.Client
	from   string
}

// NewResendClient initializes a real Resend client
func NewResendClient(apiKey, from string) *ResendClient {
	return &ResendClient{
		client: resend.NewClient(apiKey),
		from:   from,
	}
}

func (c *ResendClient) SendVerificationEmail(to, code string) error {
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: "Verify your Noumena account",
		Html:    fmt.Sprintf("<p>Your verification code is: <strong>%s</strong></p>", code),
	}
	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}
	return nil
}

func (c *ResendClient) SendWelcomeEmail(to string) error {
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: "Welcome to Noumena!",
		Html:    "<p>Thanks for joining Noumena. Let's get started!</p>",
	}
	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send welcome email: %w", err)
	}
	return nil
}

func (c *ResendClient) SendDigestEmail(to, digestContent string) error {
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: "Your Weekly Noumena Digest",
		Html:    digestContent,
	}
	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send digest email: %w", err)
	}
	return nil
}

func (c *ResendClient) SendLockedAccountEmail(to string) error {
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: "Action Required: Account Locked",
		Html:    "<p>Your account has been locked due to too many failed login attempts.</p>",
	}
	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send locked account email: %w", err)
	}
	return nil
}

// MockClient implements the Client interface for local testing
type MockClient struct{}

// NewMockClient initializes a mock client
func NewMockClient() *MockClient {
	return &MockClient{}
}

func (m *MockClient) SendVerificationEmail(to, code string) error {
	log.Printf("[MockEmail] Verification email to %s with code %s", to, code)
	return nil
}

func (m *MockClient) SendWelcomeEmail(to string) error {
	log.Printf("[MockEmail] Welcome email to %s", to)
	return nil
}

func (m *MockClient) SendDigestEmail(to, digestContent string) error {
	log.Printf("[MockEmail] Digest email to %s (content length: %d)", to, len(digestContent))
	return nil
}

func (m *MockClient) SendLockedAccountEmail(to string) error {
	log.Printf("[MockEmail] Locked Account email to %s", to)
	return nil
}
