package main

import (
	"context"
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

func main() {
	_ = godotenv.Load()

	// Load configuration. We load in non-strict mode so the dev server can
	// still come up without all secrets present, but production deployments
	// should set strict=true.
	strict := os.Getenv("STRICT_CONFIG") == "1"
	if _, err := config.Load(strict); err != nil {
		log.Fatalf("config: %v", err)
	}

	// otel-instrumentation: initialise OpenTelemetry providers before any
	// worker or HTTP server starts so package-level meter handles
	// resolved during package init pick up the configured providers.
	// When OTEL_ENABLED=false (default), Init installs the no-op
	// providers and `shutdown` is a no-op — zero allocations on every
	// instrumentation call site.
	otelShutdown, err := telemetry.Init(context.Background(), config.Default)
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres@localhost:5432/noumena?sslmode=disable"
	}

	if err := db.InitDB(dbURL); err != nil {
		log.Printf("Failed to connect to database: %v. Running in degraded mock mode...", err)
	} else {
		defer db.CloseDB()
		if err := db.ApplyMigrations(context.Background()); err != nil {
			log.Printf("Failed to apply migrations: %v", err)
		}
	}

	var queries db.DBQuery
	if db.Pool != nil {
		queries = db.NewPostgresQuery(db.Pool)
	} else {
		log.Println("Using mock database queries.")
	}

	// Initialize Email Client.
	//
	// data-lifecycle change: introduces a second email path — the
	// pipeline + idempotency log + retry worker. Selection order:
	//
	//  1. EMAIL_ENABLED=true → build email.Pipeline (SMTP sender, retry
	//     worker) and expose it to legacy callers via PipelineAdapter.
	//     This is the production path post-cutover.
	//  2. RESEND_API_KEY set → keep the legacy Resend client (transition
	//     fallback for environments not yet migrated to SMTP).
	//  3. otherwise → mock client (dev/CI default).
	//
	// The two paths are mutually exclusive; if both env knobs are set,
	// EMAIL_ENABLED wins and we log the override so operators notice.
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
		// NetSMTPSender selects TLS mode by port (465=implicit, 587=
		// STARTTLS); plain SMTP is rejected unless AllowInsecure is set
		// (we leave it false in production).
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
		// Retry worker: best-effort drain of pending rows. See
		// design.md D11; currently degraded (no body persistence) so
		// it mostly marks orphaned rows failed. Lifecycle owned by the
		// process context (no explicit Stop).
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

	// Initialize Billing Client. When BILLING_ENABLED=true the config layer
	// has already validated that all required Stripe env vars are present;
	// otherwise we fall back to a mock client so the API still boots in dev.
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

	// Start LLM Workers
	openAIKey := os.Getenv("OPENAI_API_KEY")

	// Weekly-digest cron worker. Gated by DIGEST_WORKER_ENABLED (default
	// "false" for safe rollout — see openspec/changes/weekly-digest).
	// When enabled, OPENAI_API_KEY is required so the worker has an LLM
	// transport; we treat a missing key as a configuration warning rather
	// than a fatal error so the rest of the API still boots.
	var digestWorker *llm.Worker
	if os.Getenv("DIGEST_WORKER_ENABLED") == "true" || os.Getenv("DIGEST_WORKER_ENABLED") == "1" {
		if openAIKey == "" {
			log.Println("WARN: DIGEST_WORKER_ENABLED=true but OPENAI_API_KEY is unset; digest worker will not start")
		} else if queries == nil {
			log.Println("WARN: DIGEST_WORKER_ENABLED=true but no live DB; digest worker will not start")
		} else {
			digestWorker = &llm.Worker{
				Queries:     queries,
				LLMClient:   llm.NewClient(openAIKey),
				EmailClient: emailClient,
				Pipeline:    emailPipeline, // 12.6 weekly_digest send
			}
			digestWorker.Start(context.Background())
			log.Println("Weekly-digest worker started")
			defer digestWorker.Stop()
		}
	} else {
		log.Println("Weekly-digest worker: disabled (DIGEST_WORKER_ENABLED!=true)")
	}

	// Start the new spec-driven LLM batch processor (llm-processing change,
	// extended by use-opencode-as-ai-provider). The primary and fallback
	// providers are selected by name from config; the required API key is
	// determined by the chosen primary.
	if config.Default.LLMProcessorEnabled {
		primaryName := config.Default.LLMPrimaryProvider
		fallbackName := config.Default.LLMFallbackProvider

		provCfg := llm.ProviderConfig{
			OpenAIAPIKey:    openAIKey,
			OpenAIModel:     config.Default.OpenAIModel,
			AnthropicAPIKey: config.Default.AnthropicAPIKey,
			AnthropicModel:  config.Default.AnthropicModel,
			OpenCodeAPIKey:  config.Default.OpenCodeAPIKey,
			OpenCodeBaseURL: config.Default.OpenCodeBaseURL,
			OpenCodeModel:   config.Default.OpenCodeModel,
		}

		primary, err := llm.BuildLLMClientByName(primaryName, provCfg)
		if err != nil {
			log.Fatalf("LLM processor: invalid LLM_PRIMARY_PROVIDER=%q: %v", primaryName, err)
		}
		if primary == nil {
			required := llm.RequiredEnvVarForProvider(primaryName)
			if required == "" {
				log.Fatalf("LLM_PROCESSOR_ENABLED=true but LLM_PRIMARY_PROVIDER=%q is not recognized", primaryName)
			}
			log.Fatalf("LLM_PROCESSOR_ENABLED=true but %s is unset (mandatory for primary=%s)", required, primaryName)
		}

		fallback, ferr := llm.BuildLLMClientByName(fallbackName, provCfg)
		if ferr != nil {
			log.Fatalf("LLM processor: invalid LLM_FALLBACK_PROVIDER=%q: %v", fallbackName, ferr)
		}
		if fallback == nil && fallbackName != "" {
			required := llm.RequiredEnvVarForProvider(fallbackName)
			log.Printf("WARN: LLM_FALLBACK_PROVIDER=%s but %s is unset — provider failover is disabled for this process",
				fallbackName, required)
		}

		failover := llm.NewFailoverClient(llm.FailoverConfig{
			Primary:        primary,
			Fallback:       fallback,
			ErrorThreshold: config.Default.LLMFailoverErrorThreshold,
			WindowSeconds:  config.Default.LLMFailoverWindowSeconds,
		})
		proc := llm.NewProcessor(failover, queries)
		proc.TickInterval = config.Default.LLMProcessorTickInterval
		proc.BatchSize = config.Default.LLMBatchSize
		proc.ConfidenceThreshold = config.Default.LLMConfidenceThreshold
		proc.ShutdownGrace = config.Default.LLMProcessorShutdownGrace
		log.Printf("LLM processor: enabled=true primary=%s fallback=%s fallback_available=%t batch_size=%d threshold=%.2f failover_threshold=%d window_s=%d",
			primaryName, fallbackName, fallback != nil,
			config.Default.LLMBatchSize, config.Default.LLMConfidenceThreshold,
			config.Default.LLMFailoverErrorThreshold, config.Default.LLMFailoverWindowSeconds)
		go proc.Run(context.Background())
	} else {
		log.Println("LLM processor: disabled (LLM_PROCESSOR_ENABLED=false)")
	}

	// Start the unverified-account cleanup job (daily).
	if queries != nil {
		auth.StartUnverifiedAccountCleanup(context.Background(), queries, 0, 0)
		api.StartTokenExpiryNotifier(context.Background(), queries, emailClient, api.TokenExpirySchedule{})
		api.StartChannelPurgeJob(context.Background(), queries, 0)
	}

	// Start the daily billing-quota cron (billing change, task 8.3): resets
	// free-tier quotas and transitions past_due → paused after the grace
	// window. First run is aligned to 08:00 UTC.
	if pq, ok := queries.(*db.PostgresQuery); ok {
		billing.StartQuotaCron(context.Background(), pq, billing.QuotaCronConfig{
			Interval:          24 * time.Hour,
			InitialDelay:      billing.NextRunDelayUTC(8, 0),
			PastDueGraceHours: 72,
			Pipeline:          emailPipeline, // 12.4 payment_grace_final
		})
		log.Println("Billing quota cron: started (daily, first run aligned to 08:00 UTC)")

		// data-lifecycle (12.5): trial-expiry reminder cron. Same 08:00 UTC
		// schedule as the quota cron; gated internally on Pipeline != nil.
		billing.StartTrialReminder(context.Background(), pq, emailPipeline, billing.TrialReminderConfig{
			Interval:     24 * time.Hour,
			InitialDelay: billing.NextRunDelayUTC(8, 5), // stagger 5min after quota cron
			LeadTime:     72 * time.Hour,
		})
		if emailPipeline != nil {
			log.Println("Trial-expiry reminder cron: started (daily, first run 08:05 UTC)")
		}
	}

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// otel-instrumentation: HTTP server metrics + (when traces are
	// enabled) server spans. The two-layer composition is deliberate:
	// otelhttp.NewMiddleware emits the server span (using the global
	// TracerProvider — no-op when traces are disabled), and our
	// OtelMiddleware records the metrics with the chi route pattern
	// (which otelhttp cannot see because it runs before routing).
	r.Use(otelhttp.NewMiddleware("noumena-api"))
	r.Use(api.OtelMiddleware())

	authHandler := &api.AuthHandler{Queries: queries, EmailClient: emailClient}
	channelHandler := &api.ChannelHandler{Queries: queries}
	if pq, ok := queries.(*db.PostgresQuery); ok {
		channelHandler.SyncStatusReader = pq
	}
	commentHandler := &api.CommentHandler{Queries: queries}
	billingHandler := &api.BillingHandler{Queries: queries, BillingClient: billingClient, Pipeline: emailPipeline}
	if pq, ok := queries.(*db.PostgresQuery); ok {
		billingHandler.Pool = pq.Pool()
	}
	digestHandler := &api.DigestHandler{Queries: queries}

	// data-lifecycle change: account/GDPR-export handlers + workers.
	// Handlers are constructed unconditionally (so the type is always
	// available for the router gate below) but only registered when
	// ACCOUNT_LIFECYCLE_API_ENABLED=true. Workers are started under a
	// separate flag (LIFECYCLE_WORKERS_ENABLED) so a deployment can
	// run the API surface in one replica and the destructive workers
	// in another.
	accountHandler := &api.AccountHandler{
		Queries:  queries,
		Pipeline: emailPipeline, // nil when EMAIL_ENABLED=false; handlers no-op the email step
		Billing:  billingClient,
		Cfg:      config.Default,
	}
	if config.Default.LifecycleWorkersEnabled {
		if pq, ok := queries.(*db.PostgresQuery); ok && pq.Pool() != nil {
			delWorker := &lifecycle.DeletionWorker{
				Queries:  queries,
				Pool:     pq.Pool(),
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

	// insights-api change: rich feed/saved/star/exports/insights endpoints.
	// All new routes live behind both the auth+verified-email middleware AND
	// the insights-api feature flag (returns 503 when off). They require the
	// concrete *db.PostgresQuery type because the new methods (BuildFeedQuery
	// driver, exports CRUD, aggregates) are not on the DBQuery interface.
	var insightsHandler *api.InsightsHandler
	if pq, ok := queries.(*db.PostgresQuery); ok {
		insightsHandler = &api.InsightsHandler{Queries: pq, Cfg: config.Default}

		// Ensure EXPORT_DIR exists at startup; mode 0700.
		if err := config.EnsureExportDir(config.Default.ExportDir); err != nil {
			log.Printf("WARN: EnsureExportDir(%q) failed: %v", config.Default.ExportDir, err)
		}

		// Export worker: drains queued exports + sweeps expired files.
		// Gated by EXPORT_WORKER_ENABLED so a single replica owns the queue
		// in production while other API replicas only serve sync exports.
		if config.Default.ExportWorkerEnabled {
			w := &export.Worker{
				Queries:      pq,
				Pool:         pq.Pool(),
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

	// Start the comment-ingestion pipeline (scheduler + worker) if enabled.
	if config.Default.IngestionWorkerEnabled {
		if pq, ok := queries.(*db.PostgresQuery); ok && pq.Pool() != nil {
			scheduler := ingestion.NewScheduler(pq, config.Default.IngestionPollIntervalMinutes)
			worker := ingestion.NewWorker(pq, pq.Pool())
			worker.JobDeadline = config.Default.IngestionJobDeadline
			worker.MaxConcurrency = config.Default.IngestionMaxConcurrency
			worker.Alerter = ingestion.PickAlerter(config.Default.IngestionDeadLetterAlertTo)
			go scheduler.Run(context.Background(), config.Default.IngestionTickInterval)
			go worker.Run(context.Background())
			log.Println("Comment-ingestion worker started")
		} else {
			log.Println("Ingestion worker NOT started: no live Postgres pool")
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

		// Google SSO
		r.Get("/google", authHandler.GoogleStart)
		r.Get("/google/callback", authHandler.GoogleCallback)
		r.Post("/google/link", authHandler.LinkGoogleConfirm)
		// Popup-based JSON twins of the redirect endpoints
		// (auth-google-popup-endpoints change).
		r.Get("/google/auth-url", authHandler.GetGoogleAuthURL)
		r.Post("/google/callback", authHandler.GoogleCallbackJSON)

		// Authenticated /auth routes (no email-verification gate).
		r.Group(func(r chi.Router) {
			r.Use(api.AuthMiddlewareWithQueries(queries))
			r.Post("/logout-all", authHandler.LogoutAll)
			r.Post("/resend-verification", authHandler.ResendVerification)
			// Lightweight session-introspection endpoint for SPA auth-store
			// hydration. Lives under /auth so it is NOT gated by the /api
			// middleware chain (verified-email + ACCOUNT_LIFECYCLE flag).
			r.Get("/me", authHandler.Me)
		})
	})

	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/stripe", billingHandler.WebhookStripe)
	})

	// data-lifecycle change: signed-URL download endpoint lives OUTSIDE
	// the /api auth chain — the URL signature IS the credential.
	// Registered only when the lifecycle API surface is enabled, so
	// disabled deployments can't accidentally serve files even if the
	// signing secret leaks.
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
		r.Patch("/channels/{id}/pause", channelHandler.Pause)
		r.Post("/channels/{id}/resume", channelHandler.Resume)
		r.Delete("/channels/{id}", channelHandler.Delete)
		r.Patch("/channels/{id}/backfill-window", channelHandler.UpdateBackfillWindow)
		r.Get("/channels/{id}/status", channelHandler.Status)
		r.Get("/channels/{id}/sync-status", channelHandler.SyncStatus)
		r.Get("/comments", commentHandler.Feed)
		r.Post("/comments/{id}/star", commentHandler.Star)

		// weekly-digest read API + opt-in toggle. These live on the same
		// auth+verified-email middleware chain as the rest of /api.
		r.Get("/digests", digestHandler.List)
		r.Get("/digests/latest", digestHandler.Latest)
		r.Patch("/user/preferences", digestHandler.UpdatePreferences)

		// data-lifecycle change: account/GDPR endpoints. Gated by
		// ACCOUNT_LIFECYCLE_API_ENABLED so the schema + workers can
		// ship before the user-facing surface is exposed.
		if config.Default.AccountLifecycleAPIEnabled {
			r.Get("/user/me", accountHandler.GetProfile)
			r.Delete("/user/account", accountHandler.DeleteAccount)
			r.Post("/user/account/cancel-deletion", accountHandler.CancelAccountDeletion)
			r.Post("/user/data-export", accountHandler.RequestDataExport)
			r.Get("/user/data-export/{id}/status", accountHandler.GetExportStatus)
		}

		// insights-api routes. Mounted only when we have a live Postgres
		// pool (insightsHandler != nil); otherwise the feature is silently
		// unavailable in mock mode. The InsightsAPIGate middleware further
		// returns 503 when the INSIGHTS_API_ENABLED flag is off so we can
		// roll out per-environment without recompiling.
		if insightsHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(api.InsightsAPIGate(config.Default))

				// Feed + saved (rich response with total/has_more).
				r.Get("/comments/feed", insightsHandler.Feed)
				r.Get("/comments/saved", insightsHandler.Saved)

				// Star/unstar (scoped, idempotent).
				r.Post("/comments/{id}/star", insightsHandler.Star)
				r.Delete("/comments/{id}/star", insightsHandler.Unstar)

				// Exports.
				r.Post("/comments/export", insightsHandler.CreateExport)
				r.Get("/comments/export/{id}/status", insightsHandler.ExportStatus)
				r.Get("/comments/export/{id}/download", insightsHandler.ExportDownload)

				// Aggregates + trends + digest.
				r.Get("/insights/sentiment", insightsHandler.Sentiment)
				r.Get("/insights/toxicity-shield", insightsHandler.ToxicityShield)
				r.Get("/insights/trends", insightsHandler.Trends)
				r.Get("/insights/digest/latest", insightsHandler.LatestDigest)
			})
		}
	})

	log.Println("Server starting on :8080")
	srv := &http.Server{Addr: ":8080", Handler: r}

	// Run the listener on a goroutine so we can intercept SIGTERM /
	// SIGINT and run the graceful-shutdown sequence (otel-instrumentation
	// requires us to flush exporter buffers before exit).
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

	// Bounded shutdown: the OTel exporter is async and tolerant of
	// timeouts, so a 5s deadline is a tradeoff between flushing the
	// last buffer and not hanging SIGTERM. Errors from the telemetry
	// shutdown are LOGGED, never fatal, per the spec.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: http: %v", err)
	}
	if err := otelShutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: telemetry: %v", err)
	}
}
