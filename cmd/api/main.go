package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"noumena/internal/api"
	"noumena/internal/auth"
	"noumena/internal/billing"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
	"noumena/internal/lifecycle"
	"noumena/internal/export"
	"noumena/internal/ingestion"
	"noumena/internal/llm"
	"noumena/internal/telemetry"
)

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func main() {
	startTime := time.Now()
	_ = godotenv.Load()

	strict := os.Getenv("STRICT_CONFIG") == "1"
	if _, err := config.Load(strict); err != nil {
		log.Fatalf("config: %v", err)
	}

	otelShutdown, err := telemetry.Init(context.Background(), config.Default)
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./noumena.db"
	}

	if err := db.InitDB(dbPath); err != nil {
		log.Printf("Failed to connect to database: %v. Running in degraded mock mode...", err)
	} else {
		defer db.CloseDB()
		if err := db.ApplyMigrations(context.Background()); err != nil {
			log.Printf("Failed to apply migrations: %v", err)
		}
	}

	var queries db.DBQuery
	if db.DB != nil {
		queries = db.NewSQLiteQuery(db.DB)
	} else {
		log.Println("Using mock database queries.")
	}

	var emailClient email.Client
	var emailPipeline *email.Pipeline
	switch {
	case config.Default.EmailEnabled:
		if os.Getenv("RESEND_API_KEY") != "" {
			log.Println("EMAIL_ENABLED=true overrides RESEND_API_KEY (pipeline+SMTP path active)")
		}
		if queries == nil {
			log.Fatalf("EMAIL_ENABLED=true requires a live database (email_send_log) — refusing to start")
		}
		sender := &email.NetSMTPSender{
			Host:     config.Default.SMTPHost,
			Port:     config.Default.SMTPPort,
			Username: config.Default.SMTPUsername,
			Password: config.Default.SMTPPassword,
		}
		emailPipeline = &email.Pipeline{
			Queries: queries,
			Sender:  sender,
			Cfg:     config.Default,
			Logger:  log.Default(),
		}
		emailClient = email.NewPipelineAdapter(emailPipeline)
		log.Printf("Email: pipeline initialized (provider=%s host=%s)", config.Default.EmailProvider, config.Default.SMTPHost)
		retry := &email.RetryWorker{
			Pipeline: emailPipeline,
			Queries:  queries,
			Sender:   sender,
		}
		retry.Start(context.Background())
	case os.Getenv("RESEND_API_KEY") != "":
		emailClient = email.NewResendClient(os.Getenv("RESEND_API_KEY"), "onboarding@resend.dev")
		log.Println("Email: legacy Resend client (EMAIL_ENABLED=false, RESEND_API_KEY present)")
	default:
		log.Println("Email: mock client (EMAIL_ENABLED=false, RESEND_API_KEY unset)")
		emailClient = email.NewMockClient()
	}

	var billingClient billing.Client
	if config.Default.BillingEnabled {
		prices := billing.PriceMap{
			CreatorMonthly: config.Default.StripePriceCreatorMonthly,
			CreatorAnnual:  config.Default.StripePriceCreatorAnnual,
			StudioMonthly:  config.Default.StripePriceStudioMonthly,
			StudioAnnual:   config.Default.StripePriceStudioAnnual,
			AgencyMonthly:  config.Default.StripePriceAgencyMonthly,
			AgencyAnnual:   config.Default.StripePriceAgencyAnnual,
		}
		billingClient = billing.NewStripeClient(billing.Config{
			SecretKey:     config.Default.StripeSecretKey,
			WebhookSecret: config.Default.StripeWebhookSecret,
			Prices:        prices,
			SuccessURL:    config.Default.BillingCheckoutSuccessURL,
			CancelURL:     config.Default.BillingCheckoutCancelURL,
		})
		log.Println("Billing: Stripe client initialized (BILLING_ENABLED=true)")
	} else {
		log.Println("Billing: disabled (BILLING_ENABLED!=true), using mock billing client")
		billingClient = billing.NewMockClient()
	}

	opencodeKey := os.Getenv("OPENCODE_API_KEY")

	var digestWorker *llm.Worker
	if os.Getenv("DIGEST_WORKER_ENABLED") == "true" || os.Getenv("DIGEST_WORKER_ENABLED") == "1" {
		if opencodeKey == "" {
			log.Println("WARN: DIGEST_WORKER_ENABLED=true but OPENCODE_API_KEY is unset; digest worker will not start")
		} else if queries == nil {
			log.Println("WARN: DIGEST_WORKER_ENABLED=true but no live DB; digest worker will not start")
		} else {
			opencodeClient := llm.NewOpenCodeClient(opencodeKey, config.Default.OpenCodeBaseURL, config.Default.OpenCodeModel)
			digestWorker = &llm.Worker{
				Queries:     queries,
				LLMClient:   opencodeClient,
				EmailClient: emailClient,
				Pipeline:    emailPipeline,
			}
			digestWorker.Start(context.Background())
			log.Println("Weekly-digest worker started")
			defer digestWorker.Stop()
		}
	} else {
		log.Println("Weekly-digest worker: disabled (DIGEST_WORKER_ENABLED!=true)")
	}

	if config.Default.LLMProcessorEnabled {
		if opencodeKey == "" {
			log.Fatalf("LLM_PROCESSOR_ENABLED=true but OPENCODE_API_KEY is unset")
		}
		opencodeClient := llm.NewOpenCodeClient(opencodeKey, config.Default.OpenCodeBaseURL, config.Default.OpenCodeModel)
		proc := llm.NewProcessor(opencodeClient, queries)
		proc.TickInterval = config.Default.LLMProcessorTickInterval
		proc.BatchSize = config.Default.LLMBatchSize
		proc.ConfidenceThreshold = config.Default.LLMConfidenceThreshold
		proc.ShutdownGrace = config.Default.LLMProcessorShutdownGrace
		log.Printf("LLM processor: enabled=true provider=opencode model=%s batch_size=%d threshold=%.2f",
			config.Default.OpenCodeModel,
			config.Default.LLMBatchSize, config.Default.LLMConfidenceThreshold)
		go proc.Run(context.Background())
	} else {
		log.Println("LLM processor: disabled (LLM_PROCESSOR_ENABLED=false)")
	}

	if queries != nil {
		auth.StartUnverifiedAccountCleanup(context.Background(), queries, 0, 0)
		api.StartTokenExpiryNotifier(context.Background(), queries, emailClient, api.TokenExpirySchedule{})
		api.StartChannelPurgeJob(context.Background(), queries, 0)
	}

	if sq, ok := queries.(*db.SQLiteQuery); ok {
		billing.StartQuotaCron(context.Background(), sq, billing.QuotaCronConfig{
			Interval:          24 * time.Hour,
			InitialDelay:      billing.NextRunDelayUTC(8, 0),
			PastDueGraceHours: 72,
			Pipeline:          emailPipeline,
		})
		log.Println("Billing quota cron: started (daily, first run aligned to 08:00 UTC)")

		billing.StartTrialReminder(context.Background(), sq, emailPipeline, billing.TrialReminderConfig{
			Interval:     24 * time.Hour,
			InitialDelay: billing.NextRunDelayUTC(8, 5),
			LeadTime:     72 * time.Hour,
		})
		if emailPipeline != nil {
			log.Println("Trial-expiry reminder cron: started (daily, first run 08:05 UTC)")
		}
	}

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(otelhttp.NewMiddleware("noumena-api"))
	r.Use(api.OtelMiddleware())

	rateLimitCleaner := &api.RateLimitCleaner{
		Queries: queries,
		Tick:    config.Default.RateLimitCleanupTick,
	}
	if config.Default.RateLimitEnabled && queries != nil {
		r.Use(api.RateLimitMiddleware(queries, config.Default))
		rateLimitCleaner.Start(context.Background())
		defer rateLimitCleaner.Stop()
		log.Printf("Rate limiting: enabled (auth=%d/min, unauth=%d/min)",
			config.Default.RateLimitAuthMaxPerMin, config.Default.RateLimitUnauthMaxPerMin)
	} else {
		log.Println("Rate limiting: disabled (RATE_LIMIT_ENABLED=false)")
	}

	workspaceHandler := &api.WorkspaceHandler{Queries: queries, Cfg: config.Default}

	authHandler := &api.AuthHandler{Queries: queries, EmailClient: emailClient, CookieSecure: config.Default.CookieSecure}
	if queries != nil {
		authHandler.DefaultWorkspaceCreator = workspaceHandler.CreateDefaultWorkspace
	}
	channelHandler := &api.ChannelHandler{Queries: queries}
	if sq, ok := queries.(*db.SQLiteQuery); ok {
		channelHandler.SyncStatusReader = sq
	}
	commentHandler := &api.CommentHandler{Queries: queries}
	billingHandler := &api.BillingHandler{Queries: queries, BillingClient: billingClient, Pipeline: emailPipeline}
	if sq, ok := queries.(*db.SQLiteQuery); ok {
		billingHandler.Pool = sq.DB()
	}
	digestHandler := &api.DigestHandler{Queries: queries}

	userHandler := &api.UserHandler{Queries: queries, Cfg: config.Default}

	accountHandler := &api.AccountHandler{
		Queries:  queries,
		Pipeline: emailPipeline,
		Billing:  billingClient,
		Cfg:      config.Default,
	}
	if config.Default.LifecycleWorkersEnabled {
		if sq, ok := queries.(*db.SQLiteQuery); ok && sq.DB() != nil {
			delWorker := &lifecycle.DeletionWorker{
				Queries:  queries,
				Pool:     sq.DB(),
				Pipeline: emailPipeline,
				Billing:  billingClient,
				Cfg:      config.Default,
			}
			delWorker.Start(context.Background())
			defer delWorker.Stop()

			expWorker := &lifecycle.ExportWorker{
				Queries:  queries,
				Pipeline: emailPipeline,
				Cfg:      config.Default,
			}
			expWorker.Start(context.Background())
			defer expWorker.Stop()

			cleanup := &lifecycle.CleanupCron{Queries: queries}
			cleanup.Start(context.Background())
			defer cleanup.Stop()

			log.Println("Lifecycle workers started (deletion + export + cleanup)")
		} else {
			log.Println("LIFECYCLE_WORKERS_ENABLED=true but no live DB; workers not started")
		}
	} else {
		log.Println("Lifecycle workers: disabled (LIFECYCLE_WORKERS_ENABLED=false)")
	}

	var insightsHandler *api.InsightsHandler
	if sq, ok := queries.(*db.SQLiteQuery); ok {
		insightsHandler = &api.InsightsHandler{Queries: sq, Cfg: config.Default}

		if err := config.EnsureExportDir(config.Default.ExportDir); err != nil {
			log.Printf("WARN: EnsureExportDir(%q) failed: %v", config.Default.ExportDir, err)
		}

		if config.Default.ExportWorkerEnabled {
			w := &export.Worker{
				Queries:      sq,
				Pool:         sq.DB(),
				ExportDir:    config.Default.ExportDir,
				TTLHours:     config.Default.ExportTTLHours,
				TickInterval: config.Default.ExportWorkerTickInterval,
			}
			go w.Run(context.Background())
			go w.RunCleanup(context.Background())
			log.Println("Export worker started (drain + cleanup)")
		} else {
			log.Println("Export worker: disabled (EXPORT_WORKER_ENABLED=false)")
		}
	}

	if config.Default.IngestionWorkerEnabled {
		if sq, ok := queries.(*db.SQLiteQuery); ok && sq.DB() != nil {
			scheduler := ingestion.NewScheduler(sq, config.Default.IngestionPollIntervalMinutes)
			worker := ingestion.NewWorker(sq, sq.DB())
			worker.JobDeadline = config.Default.IngestionJobDeadline
			worker.MaxConcurrency = config.Default.IngestionMaxConcurrency
			worker.Alerter = ingestion.PickAlerter(config.Default.IngestionDeadLetterAlertTo)
			go scheduler.Run(context.Background(), config.Default.IngestionTickInterval)
			go worker.Run(context.Background())
			log.Println("Comment-ingestion worker started")
		} else {
			log.Println("Ingestion worker NOT started: no live database")
		}
	}

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/logout", authHandler.Logout)
		r.Post("/verify-email", authHandler.VerifyEmail)
		r.Post("/forgot-password", authHandler.ForgotPassword)
		r.Post("/reset-password", authHandler.ResetPassword)

		r.Get("/google", authHandler.GoogleStart)
		r.Get("/google/callback", authHandler.GoogleCallback)
		r.Post("/google/link", authHandler.LinkGoogleConfirm)
		r.Get("/google/auth-url", authHandler.GetGoogleAuthURL)
		r.Post("/google/callback", authHandler.GoogleCallbackJSON)

		r.Group(func(r chi.Router) {
			r.Use(api.AuthMiddlewareWithQueries(queries))
			r.Post("/logout-all", authHandler.LogoutAll)
			r.Post("/resend-verification", authHandler.ResendVerification)
			r.Get("/me", authHandler.Me)
		})
	})

	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/stripe", billingHandler.WebhookStripe)
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"uptime":  time.Since(startTime).String(),
		})
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		if db.DB != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := db.DB.PingContext(ctx); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
					"status": "error",
					"error":  "database unavailable",
				})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
		})
	})

	r.Get("/invites/accept", workspaceHandler.AcceptInvite)
	r.Get("/metrics", api.MetricsHandler)

	if config.Default.AccountLifecycleAPIEnabled {
		r.Get("/api/exports/{id}/download", accountHandler.DownloadExport)
	}

	r.Route("/api", func(r chi.Router) {
		r.Use(api.AuthMiddlewareWithQueries(queries))
		r.Use(api.RequireVerifiedEmail(queries))

		r.Post("/billing/checkout", billingHandler.CreateCheckoutSession)
		r.Get("/billing/portal", billingHandler.CreatePortalSession)
		r.Get("/billing/status", billingHandler.Status)

		r.Get("/channels", channelHandler.List)
		r.Get("/channels/connect/youtube", channelHandler.ConnectYouTube)
		r.Post("/channels/exchange/youtube", channelHandler.ExchangeYouTubeCode)
		r.Get("/channels/connect/tiktok", channelHandler.ConnectTikTok)
		r.Post("/channels/exchange/tiktok", channelHandler.ExchangeTikTok)
		r.Get("/channels/connect/instagram", channelHandler.ConnectInstagram)
		r.Post("/channels/exchange/instagram", channelHandler.ExchangeInstagram)
		r.Patch("/channels/{id}/pause", channelHandler.Pause)
		r.Post("/channels/{id}/resume", channelHandler.Resume)
		r.Delete("/channels/{id}", channelHandler.Delete)
		r.Patch("/channels/{id}/backfill-window", channelHandler.UpdateBackfillWindow)
		r.Patch("/channels/{id}/custom-prompt", channelHandler.UpdateCustomPrompt)
		r.Get("/channels/{id}/status", channelHandler.Status)
		r.Get("/channels/{id}/sync-status", channelHandler.SyncStatus)
		r.Get("/user/rate-limit-status", userHandler.GetRateLimitStatus)
		r.Get("/comments", commentHandler.Feed)
		r.Post("/comments/{id}/star", commentHandler.Star)

		r.Get("/digests", digestHandler.List)
		r.Get("/digests/latest", digestHandler.Latest)
		r.Patch("/user/preferences", digestHandler.UpdatePreferences)

		r.Get("/workspaces/mine", workspaceHandler.GetMyWorkspaces)
		r.Get("/workspaces", workspaceHandler.ListWorkspaces)
		r.Post("/workspaces", workspaceHandler.CreateWorkspace)
		r.Get("/workspaces/{id}", workspaceHandler.ListWorkspaces)
		r.Patch("/workspaces/{id}", workspaceHandler.UpdateWorkspace)
		r.Delete("/workspaces/{id}", workspaceHandler.DeleteWorkspace)
		r.Put("/workspaces/{id}/activate", workspaceHandler.ActivateWorkspace)
		r.Post("/workspaces/{id}/invites", workspaceHandler.InviteMember)
		r.Delete("/workspaces/{id}/members/{user_id}", workspaceHandler.RemoveMember)
		r.Patch("/workspaces/{id}/members/{user_id}", workspaceHandler.ChangeMemberRole)

		if config.Default.AccountLifecycleAPIEnabled {
			r.Get("/user/me", accountHandler.GetProfile)
			r.Delete("/user/account", accountHandler.DeleteAccount)
			r.Post("/user/account/cancel-deletion", accountHandler.CancelAccountDeletion)
			r.Post("/user/data-export", accountHandler.RequestDataExport)
			r.Get("/user/data-export/{id}/status", accountHandler.GetExportStatus)
		}

		if insightsHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(api.InsightsAPIGate(config.Default))

				r.Get("/comments/feed", insightsHandler.Feed)
				r.Get("/comments/saved", insightsHandler.Saved)

				r.Post("/comments/{id}/star", insightsHandler.Star)
				r.Delete("/comments/{id}/star", insightsHandler.Unstar)

				r.Post("/comments/export", insightsHandler.CreateExport)
				r.Get("/comments/export/{id}/status", insightsHandler.ExportStatus)
				r.Get("/comments/export/{id}/download", insightsHandler.ExportDownload)

				r.Get("/insights/actionable-insights", insightsHandler.ActionableInsights)
				r.Get("/insights/actionable-insights/top", insightsHandler.TopInsights)
				r.Get("/insights/actionable-insights/distribution", insightsHandler.InsightDistribution)
				r.Post("/insights/actionable-insights/refresh", insightsHandler.RefreshInsightsView)
				r.Get("/insights/toxicity-shield", insightsHandler.ToxicityShield)
				r.Get("/insights/trends", insightsHandler.Trends)
				r.Get("/insights/digest/latest", insightsHandler.LatestDigest)

				r.Get("/comments/search", insightsHandler.SearchComments)
				r.Post("/comments/bulk-star", insightsHandler.BulkStarComments)
				r.Post("/comments/bulk-unstar", insightsHandler.BulkUnstarComments)
				r.Get("/comments/{id}/thread", insightsHandler.GetCommentThread)
			})
		}
	})

	log.Println("Server starting on :8080")
	srv := &http.Server{Addr: ":8080", Handler: r}

	listenErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			listenErr <- err
		}
		close(listenErr)
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		log.Println("shutdown: signal received, stopping HTTP server")
	case err := <-listenErr:
		if err != nil {
			log.Printf("shutdown: server error: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: http: %v", err)
	}
	if err := otelShutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: telemetry: %v", err)
	}
}
