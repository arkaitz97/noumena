package api

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"noumena/internal/billing"
	"noumena/internal/db"
)

type BillingHandler struct {
	Queries       db.DBQuery
	BillingClient billing.Client
}

type CheckoutRequest struct {
	PriceID string `json:"price_id"`
}

func (h *BillingHandler) CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.PriceID == "" {
		http.Error(w, "price_id is required", http.StatusBadRequest)
		return
	}

	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "http://localhost:3000"
	}
	successURL := origin + "/settings/billing/success"
	cancelURL := origin + "/settings/billing/cancel"

	url, err := h.BillingClient.CreateCheckoutSession(userID, user.Email, req.PriceID, successURL, cancelURL)
	if err != nil {
		http.Error(w, "failed to create checkout session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

func (h *BillingHandler) WebhookStripe(w http.ResponseWriter, r *http.Request) {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error reading request body", http.StatusServiceUnavailable)
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")
	event, err := h.BillingClient.ConstructEvent(payload, sigHeader)
	if err != nil {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "checkout.session.completed":
		if err := h.handleCheckoutSessionCompleted(r.Context(), event.Data.Raw); err != nil {
			log.Printf("Error processing checkout.session.completed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	case "invoice.payment_failed":
		if err := h.handleInvoicePaymentFailed(r.Context(), event.Data.Raw); err != nil {
			log.Printf("Error processing invoice.payment_failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (h *BillingHandler) handleCheckoutSessionCompleted(ctx context.Context, rawData json.RawMessage) error {
	var session struct {
		ClientReferenceID string `json:"client_reference_id"`
		CustomerID        string `json:"customer"`
	}
	if err := json.Unmarshal(rawData, &session); err != nil {
		return err
	}

	if session.ClientReferenceID == "" {
		return nil
	}

	// Move user to pro tier with 10000 quota
	return h.Queries.UpdateUserSubscription(ctx, session.ClientReferenceID, "pro", session.CustomerID, 10000)
}

func (h *BillingHandler) handleInvoicePaymentFailed(ctx context.Context, rawData json.RawMessage) error {
	var invoice struct {
		CustomerID string `json:"customer"`
	}
	if err := json.Unmarshal(rawData, &invoice); err != nil {
		return err
	}

	log.Printf("Payment failed for customer %s, handle downgrade logic here if a full reverse mapping exists", invoice.CustomerID)
	return nil
}
