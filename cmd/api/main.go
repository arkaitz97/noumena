package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"

	"noumena/internal/api"
	"noumena/internal/auth"
	"noumena/internal/billing"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/email"
	"noumena/internal/export"
	"noumena/internal/ingestion"
	"noumena/internal/lifecycle"
	"noumena/internal/llm"
	"noumena/internal/telemetry"
)

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type sqliteBackfillEnqueuer struct{ q *db.SQLiteQuery }

func (b *sqliteBackfillEnqueuer) EnqueueBackfill(ctx context.Context, platformChannelID string, days int) error {
	_, err := b.q.EnqueueIngestionJob(ctx, platformChannelID, "backfill")
	return err
}

func main() {
	startTime := time.Now()
	_ = godotenv.Load()

	strict := os.Getenv("STRICT_CONFIG") == "1"
	if _, err := config.Load(strict); err != nil {
		log.Fatalf("config: %v", err)
	}

	// JWT_SECRET is now mandatory and validated by config.Load (>=32 bytes,
	// no placeholders) — the process cannot boot without it.
	log.Println("Config: JWT_SECRET validated (mandatory, >=32 bytes, from environment)")

	// Root cancellable context wired to every worker. SIGTERM/SIGINT cancels
	// it so workers drain before HTTP/telemetry/DB teardown.
	rootCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	var workerWg sync.WaitGroup
	runWorker := func(fn func(ctx context.Context)) {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			fn(rootCtx)
		}()
	}

	otelShutdown, err := telemetry.Init(context.Background(), config.Default)
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./noumena.db"
	}

	if abs, err := filepath.Abs(dbPath); err == nil {
		dbPath = abs
	}
	log.Printf("Database: path=%s", dbPath)

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
		// Attach the dedicated query_only export handle so long export
		// reads never hold the main pool's single connection
		// (fix-export-pipeline 2.x).
		if sq, ok := queries.(*db.SQLiteQuery); ok {
			sq.SetExportReadDB(db.ExportDB)
		}
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
			Host:       config.Default.SMTPHost,
			Port:       config.Default.SMTPPort,
			Username:   config.Default.SMTPUsername,
			Password:   config.Default.SMTPPassword,
			CmdTimeout: config.Default.SMTPCmdTimeout,
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
		retry.Start(rootCtx)
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
			digestWorker.Start(rootCtx)
			log.Println("Weekly-digest worker started")
			defer digestWorker.Stop()
		}
	} else {
		log.Println("Weekly-digest worker: disabled (DIGEST_WORKER_ENABLED!=true)")
	}

	if config.Default.LLMProcessorEnabled {
		primary := config.Default.LLMPrimaryProvider
		if primary == "" {
			primary = "opencode"
		}
		fallback := config.Default.LLMFallbackProvider
		keyFor := func(name string) string {
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "openai":
				return os.Getenv("OPENAI_API_KEY")
			case "anthropic":
				return os.Getenv("ANTHROPIC_API_KEY")
			default:
				return opencodeKey
			}
		}
		var procClient llm.LLMClient
		if primary == "opencode" && fallback == "" {
			// Legacy single-provider path: honors OPENCODE_BASE_URL/OPENCODE_MODEL.
			if opencodeKey == "" {
				log.Fatalf("LLM_PROCESSOR_ENABLED=true but OPENCODE_API_KEY is unset")
			}
			procClient = llm.NewOpenCodeClient(opencodeKey, config.Default.OpenCodeBaseURL, config.Default.OpenCodeModel)
		} else {
			primaryKey := keyFor(primary)
			if primaryKey == "" {
				log.Fatalf("LLM_PROCESSOR_ENABLED=true but no API key for primary LLM provider %q", primary)
			}
			fallbackKey := keyFor(fallback)
			var err error
			procClient, err = llm.NewConfiguredClient(primary, fallback, primaryKey, fallbackKey)
			if err != nil {
				log.Fatalf("LLM processor: client init failed: %v", err)
			}
			if fallback != "" {
				log.Printf("LLM processor: failover enabled primary=%s fallback=%s", primary, fallback)
			}
		}
		proc := llm.NewProcessor(procClient, queries)
		proc.TickInterval = config.Default.LLMProcessorTickInterval
		proc.BatchTimeout = config.Default.LLMProcessorBatchTimeout
		proc.BatchSize = config.Default.LLMBatchSize
		proc.ConfidenceThreshold = config.Default.LLMConfidenceThreshold
		proc.ShutdownGrace = config.Default.LLMProcessorShutdownGrace
		log.Printf("LLM processor: enabled=true provider=%s batch_size=%d threshold=%.2f",
			primary,
			config.Default.LLMBatchSize, config.Default.LLMConfidenceThreshold)
		runWorker(proc.Run)
	} else {
		log.Println("LLM processor: disabled (LLM_PROCESSOR_ENABLED=false)")
	}

	if queries != nil {
		auth.StartUnverifiedAccountCleanup(rootCtx, queries, 0, 0)
		api.StartTokenExpiryNotifier(rootCtx, queries, emailClient, api.TokenExpirySchedule{})
		api.StartChannelPurgeJob(rootCtx, queries, 0)
	}

	if sq, ok := queries.(*db.SQLiteQuery); ok {
		billing.StartQuotaCron(rootCtx, sq, billing.QuotaCronConfig{
			Interval:          24 * time.Hour,
			InitialDelay:      billing.NextRunDelayUTC(8, 0),
			PastDueGraceHours: 72,
			Pipeline:          emailPipeline,
		})
		log.Println("Billing quota cron: started (daily, first run aligned to 08:00 UTC)")

		billing.StartTrialReminder(rootCtx, sq, emailPipeline, billing.TrialReminderConfig{
			Interval:     24 * time.Hour,
			InitialDelay: billing.NextRunDelayUTC(8, 5),
			LeadTime:     72 * time.Hour,
		})
		if emailPipeline != nil {
			log.Println("Trial-expiry reminder cron: started (daily, first run 08:05 UTC)")
		}

		billing.StartOverageInvoicingCron(rootCtx, sq, billingClient, billing.OverageInvoicingConfig{
			Enabled:      config.Default.BillingOverageInvoicingEnabled,
			InitialDelay: billing.NextRunDelayUTC(8, 10),
		})
	}

	r := chi.NewRouter()

	// Access logging (path only — never the query string, so ?token= from
	// verify/reset links cannot leak) and HTTP observability (metrics +
	// spans with sanitized http.target). Both wrap Recoverer so panics
	// are still observed.
	r.Use(api.AccessLogMiddleware(nil))
	r.Use(api.OtelMiddleware())
	r.Use(middleware.Recoverer)

	// HTTP hardening: security headers on every response and a global
	// request-body cap (Stripe webhook keeps its own stricter 1 MiB cap).
	r.Use(api.SecurityHeadersMiddleware(config.Default.CSPHeader))
	r.Use(api.MaxBytesMiddleware(config.Default.MaxBodyBytes))

	rateLimitCleaner := &api.RateLimitCleaner{
		Queries: queries,
		Tick:    config.Default.RateLimitCleanupTick,
	}
	if queries != nil {
		rateLimitCleaner.Start(rootCtx)
		defer rateLimitCleaner.Stop()
	}
	if config.Default.RateLimitEnabled {
		log.Printf("Rate limiting: enabled (auth=%d/min, unauth=%d/min)",
			config.Default.RateLimitAuthMaxPerMin, config.Default.RateLimitUnauthMaxPerMin)
	} else {
		log.Println("Rate limiting: global flag off; auth endpoints still rate-limited (always-on)")
	}

	workspaceHandler := &api.WorkspaceHandler{Queries: queries, Cfg: config.Default}

	authHandler := &api.AuthHandler{Queries: queries, EmailClient: emailClient, CookieSecure: config.Default.CookieSecure}
	if queries != nil {
		authHandler.DefaultWorkspaceCreator = workspaceHandler.CreateDefaultWorkspace
	}
	channelHandler := &api.ChannelHandler{Queries: queries}
	if config.Default.YouTubeAPIKey != "" {
		yt := ingestion.NewYouTubeClient()
		yt.APIKey = config.Default.YouTubeAPIKey
		channelHandler.YouTubeResolver = yt
	}
	if sq, ok := queries.(*db.SQLiteQuery); ok {
		channelHandler.SyncStatusReader = sq
		channelHandler.BackfillEnqueuer = &sqliteBackfillEnqueuer{q: sq}
	}
	billingHandler := &api.BillingHandler{Queries: queries, BillingClient: billingClient, Pipeline: emailPipeline}
	if sq, ok := queries.(*db.SQLiteQuery); ok {
		billingHandler.Pool = sq.DB()
	}
	digestHandler := &api.DigestHandler{Queries: queries, Cfg: config.Default}

	userHandler := &api.UserHandler{Queries: queries, Cfg: config.Default}

	eventHandler := &api.EventHandler{}

	accountHandler := &api.AccountHandler{
		Queries:  queries,
		Pipeline: emailPipeline,
		Billing:  billingClient,
		Cfg:      config.Default,
	}
	profileHandler := &api.ProfileHandler{
		Queries: queries,
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
			delWorker.Start(rootCtx)
			defer delWorker.Stop()

			expWorker := &lifecycle.ExportWorker{
				Queries:  sq,
				Pipeline: emailPipeline,
				Cfg:      config.Default,
			}
			expWorker.Start(rootCtx)
			defer expWorker.Stop()

			cleanup := &lifecycle.CleanupCron{Queries: queries, StorageDir: config.Default.ExportStorageDir}
			cleanup.Start(rootCtx)
			defer cleanup.Stop()

			revokeSweeper := &lifecycle.RevokeSweeper{Queries: queries}
			revokeSweeper.Start(rootCtx)
			defer revokeSweeper.Stop()

			log.Println("Lifecycle workers started (deletion + export + cleanup + revoke sweeper)")
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
				Pool:         sq.ExportReadDB(),
				ExportDir:    config.Default.ExportDir,
				TTLHours:     config.Default.ExportTTLHours,
				TickInterval: config.Default.ExportWorkerTickInterval,
			}
			runWorker(w.Run)
			runWorker(w.RunCleanup)
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
			worker.Client.APIKey = config.Default.YouTubeAPIKey
			workerWg.Add(1)
			go func() {
				defer workerWg.Done()
				scheduler.Run(rootCtx, config.Default.IngestionTickInterval)
			}()
			workerWg.Add(1)
			go func() {
				defer workerWg.Done()
				worker.Run(rootCtx)
			}()
			log.Println("Comment-ingestion worker started")
		} else {
			log.Println("Ingestion worker NOT started: no live database")
		}
	}

	// Auth endpoints are rate-limited by default (always on, independent
	// of RATE_LIMIT_ENABLED): anonymous credential/token endpoints are
	// keyed by the trusted client IP, while authenticated endpoints get a
	// per-user bucket after AuthMiddleware runs.
	r.Route("/auth", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(api.AuthRateLimitMiddleware(queries, config.Default))

			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/refresh", authHandler.Refresh)
			r.Post("/logout", authHandler.Logout)
			r.Post("/verify-email", authHandler.VerifyEmail)
			r.Post("/forgot-password", authHandler.ForgotPassword)
			r.Post("/reset-password", authHandler.ResetPassword)

			r.Get("/google", authHandler.GoogleStart)
			r.Get("/google/callback", authHandler.GoogleCallback)
			r.Get("/google/auth-url", authHandler.GetGoogleAuthURL)
			r.Post("/google/callback", authHandler.GoogleCallbackJSON)
		})

		r.Group(func(r chi.Router) {
			r.Use(api.AuthMiddlewareWithQueries(queries))
			r.Use(api.RateLimitMiddleware(queries, config.Default))
			r.Post("/google/link", authHandler.LinkGoogleConfirm)
			r.Post("/logout-all", authHandler.LogoutAll)
			r.Post("/password", authHandler.ChangePassword)
			r.Post("/resend-verification", authHandler.ResendVerification)
			r.Get("/me", authHandler.Me)
		})
	})

	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/stripe", billingHandler.WebhookStripe)
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"uptime": time.Since(startTime).String(),
		})
	})
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"uptime": time.Since(startTime).String(),
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

	r.Group(func(r chi.Router) {
		// Invite acceptance is a one-time-token endpoint; protect it with
		// the always-on anonymous limiter like the auth endpoints.
		r.Use(api.AuthRateLimitMiddleware(queries, config.Default))
		r.Get("/invites/accept", workspaceHandler.AcceptInvite)
		// Unsubscribe is self-authenticating (HMAC-signed link in the
		// email); the anonymous limiter protects it from link-spam.
		r.Get("/unsubscribe", digestHandler.Unsubscribe)
	})
	r.Get("/metrics", api.MetricsHandler)

	if config.Default.AccountLifecycleAPIEnabled {
		r.Get("/api/exports/{id}/download", accountHandler.DownloadExport)
	}

	r.Route("/api", func(r chi.Router) {
		r.Use(api.AuthMiddlewareWithQueries(queries))
		r.Use(api.RequireVerifiedEmail(queries))
		// Per-user bucket for authenticated requests: mounted after auth so
		// the user id is in context (fix-crypto-http-hardening).
		r.Use(api.RateLimitMiddleware(queries, config.Default))
		r.Use(api.WithWorkspaceID(queries))

		r.Post("/billing/checkout", billingHandler.CreateCheckoutSession)
		r.Get("/billing/portal", billingHandler.CreatePortalSession)
		r.Get("/billing/status", billingHandler.Status)
		r.Get("/billing/usage", billingHandler.Usage)

		r.Get("/workspaces/mine", workspaceHandler.GetMyWorkspaces)
		r.Get("/workspaces", workspaceHandler.ListWorkspaces)
		r.Post("/workspaces", workspaceHandler.CreateWorkspace)
		r.Put("/workspaces/{id}/activate", workspaceHandler.ActivateWorkspace)

		if config.Default.AccountLifecycleAPIEnabled {
			r.Delete("/user/account", accountHandler.DeleteAccount)
			r.Post("/user/account/cancel-deletion", accountHandler.CancelAccountDeletion)
			r.Post("/user/data-export", accountHandler.RequestDataExport)
			r.Get("/user/data-export/{id}/status", accountHandler.GetExportStatus)
		}

		r.Get("/user/rate-limit-status", userHandler.GetRateLimitStatus)
		r.Get("/user/token-expiry", userHandler.GetTokenExpiry)
		r.Get("/user/me", accountHandler.GetProfile)
		r.Patch("/user/me", accountHandler.PatchProfile)
		r.Get("/user/profile", profileHandler.GetProfile)
		r.Patch("/user/profile", profileHandler.PatchProfile)
		r.Patch("/user/preferences", digestHandler.UpdatePreferences)

		r.Group(func(r chi.Router) {
			r.Use(api.WorkspaceMiddleware(queries))

			r.Get("/channels", channelHandler.List)
			r.Get("/channels/{id}/status", channelHandler.Status)
			r.Get("/channels/{id}/sync-status", channelHandler.SyncStatus)

			r.Get("/digests", digestHandler.List)
			r.Get("/digests/latest", digestHandler.Latest)

			r.Post("/events", eventHandler.Track)

			r.Get("/workspaces/{id}", workspaceHandler.GetWorkspaceByID)
			r.Get("/workspaces/active/role", workspaceHandler.ActiveWorkspaceRole)

			if insightsHandler != nil {
				r.Group(func(r chi.Router) {
					r.Use(api.InsightsAPIGate(config.Default))

					r.Get("/comments/feed", insightsHandler.Feed)
					r.Get("/comments/saved", insightsHandler.Saved)
					r.Get("/comments/search", insightsHandler.SearchComments)
					r.Get("/comments/{id}/thread", insightsHandler.GetCommentThread)
					r.Get("/insights/digest/latest", insightsHandler.LatestDigest)
				})
			}
		})

		r.Group(func(r chi.Router) {
			r.Use(api.WorkspaceMiddleware(queries))
			r.Use(api.RequireWorkspaceRole("analyst", "admin"))

			r.Post("/workspaces/{id}/channels/{channelId}/grant", workspaceHandler.GrantChannel)
			r.Delete("/workspaces/{id}/channels/{channelId}/grant", workspaceHandler.RevokeChannel)

			if insightsHandler != nil {
				r.Group(func(r chi.Router) {
					r.Use(api.InsightsAPIGate(config.Default))

					r.Post("/comments/export", insightsHandler.CreateExport)
					r.Get("/comments/export/recent", insightsHandler.ListRecentExports)
					r.Get("/comments/export/{id}/status", insightsHandler.ExportStatus)
					r.Get("/comments/export/{id}/download", insightsHandler.ExportDownload)

					r.Get("/insights/actionable-insights", insightsHandler.ActionableInsights)
					r.Get("/insights/actionable-insights/top", insightsHandler.TopInsights)
					r.Get("/insights/actionable-insights/distribution", insightsHandler.InsightDistribution)
					r.Get("/insights/toxicity-shield", insightsHandler.ToxicityShield)
					r.Get("/insights/trends", insightsHandler.Trends)
					r.Get("/insights/sentiment", insightsHandler.Sentiment)
				})
			}
		})

		r.Group(func(r chi.Router) {
			r.Use(api.WorkspaceMiddleware(queries))
			r.Use(api.RequireWorkspaceRole("admin"))

			r.Get("/channels/connect/youtube", channelHandler.ConnectYouTube)
			r.Post("/channels/connect/youtube/by-name", channelHandler.ConnectYouTubeByName)
			r.Post("/channels/exchange/youtube", channelHandler.ExchangeYouTubeCode)
			r.Get("/channels/connect/tiktok", channelHandler.ConnectTikTok)
			r.Post("/channels/exchange/tiktok", channelHandler.ExchangeTikTok)
			r.Get("/channels/connect/instagram", channelHandler.ConnectInstagram)
			r.Post("/channels/exchange/instagram", channelHandler.ExchangeInstagram)
			r.Patch("/channels/{id}/pause", channelHandler.Pause)
			r.Post("/channels/{id}/resume", channelHandler.Resume)
			r.Post("/channels/{id}/reauthorize", channelHandler.Reauthorize)
			r.Post("/channels/reauthorize/exchange", channelHandler.ExchangeReauthorize)
			r.Delete("/channels/{id}", channelHandler.Delete)
			r.Patch("/channels/{id}/backfill-window", channelHandler.UpdateBackfillWindow)
			r.Patch("/channels/{id}/custom-prompt", channelHandler.UpdateCustomPrompt)

			r.Patch("/workspaces/{id}", workspaceHandler.UpdateWorkspace)
			r.Delete("/workspaces/{id}", workspaceHandler.DeleteWorkspace)
			r.Post("/workspaces/{id}/invites", workspaceHandler.InviteMember)
			r.Get("/workspaces/{id}/members", workspaceHandler.ListMembers)
			r.Delete("/workspaces/{id}/members/{user_id}", workspaceHandler.RemoveMember)
			r.Patch("/workspaces/{id}/members/{user_id}", workspaceHandler.ChangeMemberRole)

			if insightsHandler != nil {
				r.Group(func(r chi.Router) {
					r.Use(api.InsightsAPIGate(config.Default))

					r.Post("/comments/{id}/star", insightsHandler.Star)
					r.Delete("/comments/{id}/star", insightsHandler.Unstar)
					r.Post("/comments/bulk-star", insightsHandler.BulkStarComments)
					r.Post("/comments/bulk-unstar", insightsHandler.BulkUnstarComments)
					r.Post("/insights/actionable-insights/refresh", insightsHandler.RefreshInsightsView)
				})
			}
		})
	})

	// Serve frontend SPA for unmatched routes (production only —
	// dev uses the Vite proxy). Must be registered last so chi matches
	// API routes first.
	if _, err := os.Stat("dist/index.html"); err == nil {
		fs := http.FileServer(http.Dir("dist"))
		r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := filepath.Join("dist", r.URL.Path)
			if _, err := os.Stat(p); err != nil {
				r.URL.Path = "/"
			}
			fs.ServeHTTP(w, r)
		}))
		log.Println("Frontend: serving SPA from ./dist/")
	}

	log.Println("Server starting on :8080")
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

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
		log.Println("shutdown: signal received, draining workers")
	case err := <-listenErr:
		if err != nil {
			log.Printf("shutdown: server error: %v", err)
		}
	}

	// Ordered teardown: workers drain → HTTP stop → telemetry flush → DB close.
	stopWorkers()
	if digestWorker != nil {
		digestWorker.Stop()
	}
	if rateLimitCleaner != nil {
		rateLimitCleaner.Stop()
	}
	drainDone := make(chan struct{})
	go func() {
		workerWg.Wait()
		close(drainDone)
	}()
	select {
	case <-drainDone:
		log.Println("shutdown: workers drained")
	case <-time.After(30 * time.Second):
		log.Println("shutdown: worker drain timeout, forcing stop")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: http: %v", err)
	}
	if err := otelShutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: telemetry: %v", err)
	}
	db.CloseDB()
}
