package billing

import (
	"log"

	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/webhook"
)

type Client interface {
	CreateCheckoutSession(userID, email, priceID, successURL, cancelURL string) (string, error)
	ConstructEvent(payload []byte, signature string) (stripe.Event, error)
}

type StripeClient struct {
	webhookSecret string
}

func NewStripeClient(secretKey, webhookSecret string) Client {
	stripe.Key = secretKey
	return &StripeClient{
		webhookSecret: webhookSecret,
	}
}

func (c *StripeClient) CreateCheckoutSession(userID, email, priceID, successURL, cancelURL string) (string, error) {
	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL:        stripe.String(successURL),
		CancelURL:         stripe.String(cancelURL),
		CustomerEmail:     stripe.String(email),
		ClientReferenceID: stripe.String(userID),
	}

	s, err := session.New(params)
	if err != nil {
		log.Printf("Failed to create Stripe session: %v", err)
		return "", err
	}
	return s.URL, nil
}

func (c *StripeClient) ConstructEvent(payload []byte, signature string) (stripe.Event, error) {
	return webhook.ConstructEvent(payload, signature, c.webhookSecret)
}

// MockClient for testing
type MockClient struct{}

func NewMockClient() Client {
	return &MockClient{}
}

func (m *MockClient) CreateCheckoutSession(userID, email, priceID, successURL, cancelURL string) (string, error) {
	return "https://checkout.stripe.com/pay/mock_session", nil
}

func (m *MockClient) ConstructEvent(payload []byte, signature string) (stripe.Event, error) {
	return stripe.Event{}, nil
}
