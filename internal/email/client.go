package email

import (
	"fmt"
	"log"
	"time"

	"github.com/resend/resend-go/v2"
)

// Client defines the interface for an email client.
//
// Verification and password-reset emails take a fully-formed link (built by
// the caller from APP_BASE_URL + endpoint + token).
type Client interface {
	SendVerificationEmail(to, link string) error
	SendPasswordResetEmail(to, link string) error
	SendLoginLockoutEmail(to string) error
	SendGoogleOnlyAccountResetNotice(to string) error
	SendWelcomeEmail(to string) error
	SendDigestEmail(to, digestContent string) error
	SendTokenExpiryEmail(to, channelName string, expiresAt time.Time, settingsURL string) error

	// SendLockedAccountEmail is retained for legacy callers; it is an alias
	// for SendLoginLockoutEmail.
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

func (c *ResendClient) SendVerificationEmail(to, link string) error {
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: "Verify your Noumena account",
		Html: fmt.Sprintf(
			`<p>Welcome to Noumena!</p>`+
				`<p>Please verify your email by clicking the link below. The link is valid for 24 hours.</p>`+
				`<p><a href="%s">Verify my email</a></p>`+
				`<p>If you did not create this account, you can ignore this email.</p>`,
			link),
	}
	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}
	return nil
}

func (c *ResendClient) SendPasswordResetEmail(to, link string) error {
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: "Reset your Noumena password",
		Html: fmt.Sprintf(
			`<p>We received a request to reset your Noumena password.</p>`+
				`<p>Click the link below to set a new password. The link is valid for 1 hour.</p>`+
				`<p><a href="%s">Reset my password</a></p>`+
				`<p>If you did not request a reset, you can safely ignore this email.</p>`,
			link),
	}
	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send password reset email: %w", err)
	}
	return nil
}

func (c *ResendClient) SendLoginLockoutEmail(to string) error {
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: "Your Noumena account has been temporarily locked",
		Html: `<p>We detected several failed login attempts on your Noumena account.</p>` +
			`<p>For your protection, the account has been locked for 15 minutes.</p>` +
			`<p>If this was not you, please reset your password from the <a href="/forgot-password">/forgot-password</a> page.</p>`,
	}
	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send lockout email: %w", err)
	}
	return nil
}

func (c *ResendClient) SendGoogleOnlyAccountResetNotice(to string) error {
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: "About your Noumena account",
		Html: `<p>You requested a password reset, but this Noumena account uses Google sign-in.</p>` +
			`<p>To access your account, please click "Continue with Google" on the sign-in page.</p>`,
	}
	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send google-only notice: %w", err)
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

// SendLockedAccountEmail is a legacy alias kept for backwards compatibility.
func (c *ResendClient) SendLockedAccountEmail(to string) error {
	return c.SendLoginLockoutEmail(to)
}

func (c *ResendClient) SendTokenExpiryEmail(to, channelName string, expiresAt time.Time, settingsURL string) error {
	if channelName == "" {
		channelName = "your tracked channel"
	}
	expiresHuman := expiresAt.UTC().Format("January 2, 2006")
	params := &resend.SendEmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: fmt.Sprintf("Reconnect %s before its access expires", channelName),
		Html: fmt.Sprintf(
			`<p>The OAuth access for <strong>%s</strong> will expire on <strong>%s</strong>.</p>`+
				`<p>To keep ingestion running, please reconnect the channel from your <a href="%s">channel settings</a>.</p>`+
				`<p>If you no longer want to track this channel, you can ignore this email and the channel will simply stop syncing.</p>`,
			channelName, expiresHuman, settingsURL),
	}
	_, err := c.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send token-expiry email: %w", err)
	}
	return nil
}

// MockClient implements the Client interface for local testing
type MockClient struct{}

// NewMockClient initializes a mock client
func NewMockClient() *MockClient {
	return &MockClient{}
}

func (m *MockClient) SendVerificationEmail(to, link string) error {
	log.Printf("[MockEmail] Verification email to %s link=%s", to, link)
	return nil
}

func (m *MockClient) SendPasswordResetEmail(to, link string) error {
	log.Printf("[MockEmail] Password reset email to %s link=%s", to, link)
	return nil
}

func (m *MockClient) SendLoginLockoutEmail(to string) error {
	log.Printf("[MockEmail] Login lockout email to %s", to)
	return nil
}

func (m *MockClient) SendGoogleOnlyAccountResetNotice(to string) error {
	log.Printf("[MockEmail] Google-only account reset notice to %s", to)
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
	return m.SendLoginLockoutEmail(to)
}

func (m *MockClient) SendTokenExpiryEmail(to, channelName string, expiresAt time.Time, settingsURL string) error {
	log.Printf("[MockEmail] Token expiry email to %s channel=%s expires=%s settings=%s",
		to, channelName, expiresAt.Format(time.RFC3339), settingsURL)
	return nil
}
