package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/auth"
	"noumena/internal/billing"
)

func setupBillingRouter(queries *MockQuery, billingClient billing.Client) *chi.Mux {
	r := chi.NewRouter()

	billingHandler := &api.BillingHandler{Queries: queries, BillingClient: billingClient}

	r.Route("/api", func(r chi.Router) {
		r.Use(api.AuthMiddleware)
		r.Post("/billing/checkout", billingHandler.CreateCheckoutSession)
	})

	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/stripe", billingHandler.WebhookStripe)
	})

	return r
}

func TestBillingCheckout(t *testing.T) {
	queries := &MockQuery{}
	billingClient := billing.NewMockClient()
	r := setupBillingRouter(queries, billingClient)

	// Add test user (Hardcoded ID is test-user-id in MockQuery)
	queries.CreateUser(context.Background(), "test@example.com", "hash")
	
	accessToken, _, _ := auth.GenerateTokens("test-user-id")

	// 1. Unauthenticated
	req, _ := http.NewRequest("POST", "/api/billing/checkout", bytes.NewBuffer([]byte(`{"price_id":"price_123"}`)))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got %v", rr.Code)
	}

	// 2. Authenticated
	req, _ = http.NewRequest("POST", "/api/billing/checkout", bytes.NewBuffer([]byte(`{"price_id":"price_123"}`)))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %v: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		URL string `json:"url"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.URL != "https://checkout.stripe.com/pay/mock_session" {
		t.Errorf("Expected mocked checkout URL, got %s", resp.URL)
	}
}

func TestStripeWebhook(t *testing.T) {
	queries := &MockQuery{}
	billingClient := billing.NewMockClient()
	r := setupBillingRouter(queries, billingClient)

	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer([]byte(`{}`)))
	req.Header.Set("Stripe-Signature", "t=test,v1=test")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %v", rr.Code)
	}
}
