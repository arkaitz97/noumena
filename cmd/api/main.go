package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"

	"noumena/internal/api"
	"noumena/internal/billing"
	"noumena/internal/db"
	"noumena/internal/email"
	"noumena/internal/llm"
)

func main() {
	_ = godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres@localhost:5432/noumena?sslmode=disable"
	}

	if err := db.InitDB(dbURL); err != nil {
		log.Printf("Failed to connect to database: %v. Running in degraded mock mode...", err)
		// Usually we might fatal out, but we allow continuing with a mock if DB is down for early dev
	} else {
		defer db.CloseDB()
	}

	var queries db.DBQuery
	if db.Pool != nil {
		queries = db.NewPostgresQuery(db.Pool)
	} else {
		log.Println("Using mock database queries.") // For now, handle silently if nil or set up explicit mock
		// queries = db.NewMockQuery()
	}

	// Initialize Email Client
	var emailClient email.Client
	if resendKey := os.Getenv("RESEND_API_KEY"); resendKey != "" {
		emailClient = email.NewResendClient(resendKey, "onboarding@resend.dev")
	} else {
		log.Println("RESEND_API_KEY not set, using mock email client")
		emailClient = email.NewMockClient()
	}

	// Initialize Billing Client
	var billingClient billing.Client
	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	stripeWebhook := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if stripeKey != "" && stripeWebhook != "" {
		billingClient = billing.NewStripeClient(stripeKey, stripeWebhook)
	} else {
		log.Println("Stripe keys not set, using mock billing client")
		billingClient = billing.NewMockClient()
	}

	// Start LLM Workers
	openAIKey := os.Getenv("OPENAI_API_KEY")
	go llm.StartBatchProcessor(context.Background(), queries, openAIKey)
	go llm.StartDigestWorker(context.Background(), queries, openAIKey, emailClient)

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	authHandler := &api.AuthHandler{Queries: queries, EmailClient: emailClient}
	channelHandler := &api.ChannelHandler{Queries: queries}
	commentHandler := &api.CommentHandler{Queries: queries}
	billingHandler := &api.BillingHandler{Queries: queries, BillingClient: billingClient}

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
	})

	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/stripe", billingHandler.WebhookStripe)
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(api.AuthMiddleware)

		r.Post("/billing/checkout", billingHandler.CreateCheckoutSession)

		r.Get("/channels", channelHandler.List)
		r.Get("/channels/connect/youtube", channelHandler.ConnectYouTube)
		r.Post("/channels/exchange/youtube", channelHandler.ExchangeYouTubeCode)
		r.Patch("/channels/{id}/pause", channelHandler.Pause)
		r.Get("/comments", commentHandler.Feed)
		r.Post("/comments/{id}/star", commentHandler.Star)
	})

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
