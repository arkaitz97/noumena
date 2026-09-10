package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	// Auth / JWT
	JWTSecret                 []byte
	JWTAccessTTL              time.Duration
	JWTRefreshTTL             time.Duration
	BcryptCost                int
	EmailVerificationTTL      time.Duration
	EmailVerificationRequired bool
	PasswordResetTTL          time.Duration
	AppBaseURL                string

	// Encryption keys (fix-crypto-http-hardening). ENCRYPTION_KEY is
	// mandatory and validated in Load(); ENCRYPTION_KEY_PREVIOUS is
	// optional and only used for decryption during key rotation. Values
	// are used as raw AES key bytes: 16, 24, or 32 bytes.
	EncryptionKey         []byte
	EncryptionKeyPrevious []byte

	// Google SSO (separate OAuth client from YouTube)
	GoogleSSOClientID     string
	GoogleSSOClientSecret string
	GoogleSSORedirectURL  string
	// GoogleSSOPopupRedirectURL is the SPA-side `/oauth/callback` URL used by
	// the popup-based JSON OAuth flow (auth-google-popup-endpoints change).
	// When unset it falls back to GoogleSSORedirectURL so single-redirect-uri
	// deployments continue to work unchanged.
	GoogleSSOPopupRedirectURL string

	// YouTube OAuth (separate Google OAuth client from Google SSO)
	YouTubeOAuthClientID     string
	YouTubeOAuthClientSecret string
	YouTubeOAuthRedirectURL  string

	// Channel-management lifecycle
	ChannelDeletionGraceHours int
	TokenExpiryWarningDays    int
	OAuthStateTTL             time.Duration

	// Comment-ingestion pipeline
	IngestionWorkerEnabled       bool
	IngestionTickInterval        time.Duration
	IngestionPollIntervalMinutes int
	IngestionMaxRetries          int
	IngestionJobDeadline         time.Duration
	IngestionMaxConcurrency      int
	IngestionDeadLetterAlertTo   string
	// YouTubeAPIKey enables key-based (OAuth-free) ingestion reads: when set,
	// the worker skips per-user OAuth token acquisition for search.list /
	// commentThreads.list / comments.list. Public-data only.
	YouTubeAPIKey string
	// YouTube quota budget (optimize-youtube-api-quota). YouTubeDailyQuotaUnits
	// mirrors the project's daily Data API v3 quota (default 10,000 units);
	// the client stops issuing requests once the local accounting hits the
	// budget. The per-job ceilings cap how much of the daily budget a single
	// channel job may consume.
	YouTubeDailyQuotaUnits         int
	IngestionMaxRequestsPerJob     int
	IngestionMaxVideosPerJob       int
	IngestionMaxThreadPagesPerJob  int
	IngestionMaxReplyFetchesPerJob int
	// Adaptive polling bounds: inactive channels back off from
	// IngestionPollIntervalMinutes up to x IngestionPollIntervalMaxMultiplier.
	IngestionPollIntervalMaxMultiplier int

	// LLM batch-processing (llm-processing change)
	LLMProcessorEnabled       bool
	LLMProcessorTickInterval  time.Duration
	LLMProcessorBatchTimeout  time.Duration
	LLMBatchSize              int
	LLMConfidenceThreshold    float64
	LLMProcessorShutdownGrace time.Duration
	OpenCodeAPIKey            string
	OpenCodeBaseURL           string
	OpenCodeModel             string

	// Insights API (insights-api change)
	InsightsAPIEnabled       bool
	ExportDir                string
	ExportSyncThreshold      int
	ExportTTLHours           int
	ExportWorkerTickInterval time.Duration
	ExportWorkerEnabled      bool
	FeedDefaultLimit         int
	FeedMaxLimit             int
	FeedQueryTimeout         time.Duration
	CountQueryTimeout        time.Duration

	// Billing (billing change). All Stripe-related values are required only
	// when BillingEnabled = true. Load() validates this; see task 2.4.
	BillingEnabled            bool
	StripeSecretKey           string
	StripeWebhookSecret       string
	StripePriceCreatorMonthly string
	StripePriceCreatorAnnual  string
	StripePriceStudioMonthly  string
	StripePriceStudioAnnual   string
	StripePriceAgencyMonthly  string
	StripePriceAgencyAnnual   string
	BillingCheckoutSuccessURL string
	BillingCheckoutCancelURL  string
	// BillingOverageInvoicingEnabled gates the overage-invoicing cron
	// (implement-missing-product-features 8.x). Default false: rollout in
	// Stripe test mode only, per the change roadmap.
	BillingOverageInvoicingEnabled bool

	// LLM provider failover (implement-missing-product-features 6.x).
	// LLMPrimaryProvider is the default provider (opencode|openai|anthropic);
	// LLMFallbackProvider empty means no fallback. Consumers: internal/llm.
	LLMPrimaryProvider  string
	LLMFallbackProvider string

	// Transactional email pipeline (data-lifecycle change). When EmailEnabled
	// is false every call to email.Send is a logged no-op and no row is
	// written to email_send_log; this is the documented rollout/rollback
	// switch (design.md, Migration Plan).
	EmailEnabled     bool
	EmailProvider    string // "smtp" today; reserved for future provider expansion
	EmailFromAddress string
	EmailFromName    string
	SMTPHost         string
	SMTPPort         int
	SMTPUsername     string
	SMTPPassword     string
	SMTPCmdTimeout   time.Duration
	AppSupportEmail  string

	// Lifecycle workers (data-lifecycle change). When LifecycleWorkersEnabled
	// is false the deletion + export + cleanup goroutines are not started;
	// the API endpoints can still be exposed (controlled separately by
	// AccountLifecycleAPIEnabled) so users can enqueue jobs that workers will
	// process once enabled.
	LifecycleWorkersEnabled    bool
	AccountLifecycleAPIEnabled bool
	ExportStorageDir           string
	ExportSignedURLSecret      string
	ExportLinkTTLDays          int
	DeletionReversibleHours    int
	DeletionChunkSize          int
	ExportPreDeleteTimeout     time.Duration

	// OpenTelemetry (otel-instrumentation change). When OTELEnabled=false
	// (the default) Init installs no-op providers and instrumentation call
	// sites become zero-cost no-ops. When true, OTELExporterOTLPEndpoint
	// MUST be non-empty; Load() validates this and returns a typed error
	// otherwise.
	OTELEnabled              bool
	OTELExporterOTLPEndpoint string
	OTELServiceName          string
	OTELResourceAttributes   string

	// Rate limiting (spec-gap-improvements). PostgreSQL-backed sliding window.
	RateLimitEnabled         bool
	RateLimitAuthMaxPerMin   int
	RateLimitUnauthMaxPerMin int
	RateLimitCleanupTick     time.Duration

	// HTTP hardening (fix-crypto-http-hardening).
	// TrustProxyHeader: when non-empty, the client IP is read from this
	// single header (set by the reverse proxy) instead of client-supplied
	// X-Forwarded-For. Empty means trust RemoteAddr only.
	TrustProxyHeader string
	// MaxBodyBytes caps every request body (http.MaxBytesReader). The
	// Stripe webhook keeps its own stricter cap.
	MaxBodyBytes int64
	// CSPHeader is the Content-Security-Policy sent on every response;
	// empty disables the header.
	CSPHeader string

	// Cookie security. Secure cookies require HTTPS; set false for local HTTP dev.
	CookieSecure bool
}

// Default holds the process-wide configuration. Tests may override fields
// directly. Production code should call Load() once during startup.
var Default = &Config{
	JWTSecret:                 []byte("dev-only-insecure-secret-change-me"),
	JWTAccessTTL:              15 * time.Minute,
	JWTRefreshTTL:             720 * time.Hour, // 30 days
	BcryptCost:                12,
	EmailVerificationTTL:      24 * time.Hour,
	EmailVerificationRequired: true,
	PasswordResetTTL:          1 * time.Hour,
	AppBaseURL:                "http://localhost:3000",
	ChannelDeletionGraceHours: 24,
	TokenExpiryWarningDays:    7,
	OAuthStateTTL:             10 * time.Minute,

	IngestionWorkerEnabled:             true,
	IngestionTickInterval:              30 * time.Second,
	IngestionPollIntervalMinutes:       30,
	IngestionMaxRetries:                5,
	IngestionJobDeadline:               5 * time.Minute,
	IngestionMaxConcurrency:            8,
	IngestionDeadLetterAlertTo:         "",
	YouTubeDailyQuotaUnits:             10000,
	IngestionMaxRequestsPerJob:         150,
	IngestionMaxVideosPerJob:           100,
	IngestionMaxThreadPagesPerJob:      25,
	IngestionMaxReplyFetchesPerJob:     100,
	IngestionPollIntervalMaxMultiplier: 8,

	// LLM processor defaults from llm-processing/proposal.md.
	LLMProcessorEnabled:       false, // gated until smoke-tested
	LLMProcessorTickInterval:  10 * time.Second,
	LLMProcessorBatchTimeout:  30 * time.Second,
	LLMBatchSize:              50,
	LLMConfidenceThreshold:    0.7,
	LLMProcessorShutdownGrace: 30 * time.Second,
	OpenCodeBaseURL:           "https://api.opencode.local/v1",
	OpenCodeModel:             "opencode/default",

	// Insights API defaults from insights-api/proposal.md.
	InsightsAPIEnabled:       false,
	ExportDir:                "/var/lib/noumena/exports",
	ExportSyncThreshold:      50000,
	ExportTTLHours:           24,
	ExportWorkerTickInterval: 5 * time.Second,
	ExportWorkerEnabled:      false,
	FeedDefaultLimit:         20,
	FeedMaxLimit:             100,
	FeedQueryTimeout:         2000 * time.Millisecond,
	CountQueryTimeout:        1500 * time.Millisecond,

	// data-lifecycle defaults. EmailEnabled and LifecycleWorkersEnabled are
	// the rollout flags; they default off so deployments can land the schema
	// migration first, then flip the flags after smoke-testing SMTP and the
	// workers in isolation.
	EmailEnabled:               false,
	EmailProvider:              "smtp",
	SMTPCmdTimeout:             10 * time.Second,
	LifecycleWorkersEnabled:    false,
	AccountLifecycleAPIEnabled: false,
	ExportStorageDir:           "/var/lib/noumena/data-exports",
	ExportLinkTTLDays:          7,
	DeletionReversibleHours:    720,
	DeletionChunkSize:          10000,
	ExportPreDeleteTimeout:     12 * time.Hour,

	// OpenTelemetry defaults. Disabled by default so existing deployments
	// and `go test ./...` see no behavior change. OTELServiceName seeds the
	// `service.name` resource attribute.
	OTELEnabled:     false,
	OTELServiceName: "noumena-api",

	// Rate limiting defaults. 100 req/min authenticated, 10 req/min unauthenticated.
	RateLimitEnabled:         false,
	RateLimitAuthMaxPerMin:   100,
	RateLimitUnauthMaxPerMin: 10,
	RateLimitCleanupTick:     5 * time.Minute,

	// HTTP hardening defaults (fix-crypto-http-hardening). No proxy header
	// is trusted by default (RemoteAddr only), global body cap is 2 MiB,
	// and a base CSP is emitted. Override with env vars.
	TrustProxyHeader: "",
	MaxBodyBytes:     2 << 20, // 2 MiB
	CSPHeader:        "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; media-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'",

	// Cookie security. Secure by default (HTTPS only); set COOKIE_SECURE=false for local HTTP dev.
	CookieSecure: true,

	// implement-missing-product-features defaults.
	// Overage invoicing is a Stripe test-mode-only rollout: off by default.
	BillingOverageInvoicingEnabled: false,
	// LLM provider failover: OpenCode remains the sole default provider;
	// LLMFallbackProvider empty = no fallback (backward compatible).
	LLMPrimaryProvider:  "opencode",
	LLMFallbackProvider: "",
}

// Load reads configuration from environment variables, applying defaults
// where values are missing. It returns an error if any *required* value is
// missing or malformed.
//
// JWT_SECRET safety is unconditional (not gated by strict): the server must
// never boot with a missing, placeholder, or weak signing key — see
// fix-auth-critical-security. strict=true additionally fails on other missing
// required values (APP_BASE_URL, Google/YouTube OAuth config).
func Load(strict bool) (*Config, error) {
	c := *Default // copy of defaults

	// --- Mandatory JWT signing secret --------------------------------------
	// Unconditional: a forgeable signing key is a critical vulnerability, so
	// validation happens regardless of STRICT_CONFIG.
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required (generate one with: openssl rand -base64 32)")
	}
	if strings.Contains(secret, "change-me") || strings.Contains(secret, "replace-me") {
		return nil, fmt.Errorf("JWT_SECRET must not be the legacy default or a placeholder value")
	}
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 bytes, got %d", len(secret))
	}
	c.JWTSecret = []byte(secret)
	// Warning only: trivially low entropy (all one character) is technically
	// valid length-wise but should never appear in a real deployment.
	if strings.Trim(secret, secret[:1]) == "" {
		log.Printf("WARN: JWT_SECRET has trivially low entropy (single repeated character); generate one with: openssl rand -base64 32")
	}

	// --- Mandatory encryption key -----------------------------------------
	// ENCRYPTION_KEY encrypts OAuth tokens at rest. Unconditional (like
	// JWT_SECRET): there is no hardcoded fallback, so the server must never
	// boot without a real key — see fix-crypto-http-hardening. Values are
	// used as raw AES key bytes, so a valid AES key size is required.
	if err := validateEncryptionKey(os.Getenv("ENCRYPTION_KEY")); err != nil {
		return nil, err
	}
	c.EncryptionKey = []byte(os.Getenv("ENCRYPTION_KEY"))
	if prev := os.Getenv("ENCRYPTION_KEY_PREVIOUS"); prev != "" {
		if err := validateEncryptionKey(prev); err != nil {
			return nil, fmt.Errorf("ENCRYPTION_KEY_PREVIOUS: %w", err)
		}
		c.EncryptionKeyPrevious = []byte(prev)
	}

	if v := os.Getenv("JWT_ACCESS_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("JWT_ACCESS_TTL: %w", err)
		}
		c.JWTAccessTTL = d
	}

	if v := os.Getenv("JWT_REFRESH_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("JWT_REFRESH_TTL: %w", err)
		}
		c.JWTRefreshTTL = d
	}

	if v := os.Getenv("BCRYPT_COST"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("BCRYPT_COST: %w", err)
		}
		if n < 12 {
			return nil, fmt.Errorf("BCRYPT_COST must be >= 12, got %d", n)
		}
		c.BcryptCost = n
	}

	if v := os.Getenv("EMAIL_VERIFICATION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("EMAIL_VERIFICATION_TTL: %w", err)
		}
		c.EmailVerificationTTL = d
	}

	if v := os.Getenv("EMAIL_VERIFICATION_REQUIRED"); v != "" {
		c.EmailVerificationRequired = v == "1" || v == "true" || v == "yes"
	}

	if v := os.Getenv("PASSWORD_RESET_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("PASSWORD_RESET_TTL: %w", err)
		}
		c.PasswordResetTTL = d
	}

	if v := os.Getenv("APP_BASE_URL"); v != "" {
		c.AppBaseURL = v
	} else if strict {
		return nil, fmt.Errorf("APP_BASE_URL is required")
	}

	c.GoogleSSOClientID = os.Getenv("GOOGLE_SSO_CLIENT_ID")
	c.GoogleSSOClientSecret = os.Getenv("GOOGLE_SSO_CLIENT_SECRET")
	c.GoogleSSORedirectURL = os.Getenv("GOOGLE_SSO_REDIRECT_URL")
	// Popup-flow redirect URI (auth-google-popup-endpoints change). Defaults
	// to the redirect-flow URI when unset so existing deployments are not
	// required to register a second URI before the popup frontend ships.
	c.GoogleSSOPopupRedirectURL = os.Getenv("GOOGLE_SSO_POPUP_REDIRECT_URL")
	if c.GoogleSSOPopupRedirectURL == "" {
		c.GoogleSSOPopupRedirectURL = c.GoogleSSORedirectURL
	}

	if strict {
		if c.GoogleSSOClientID == "" || c.GoogleSSOClientSecret == "" || c.GoogleSSORedirectURL == "" {
			return nil, fmt.Errorf("GOOGLE_SSO_CLIENT_ID, GOOGLE_SSO_CLIENT_SECRET, and GOOGLE_SSO_REDIRECT_URL are required")
		}
	}

	c.YouTubeOAuthClientID = os.Getenv("YOUTUBE_OAUTH_CLIENT_ID")
	c.YouTubeOAuthClientSecret = os.Getenv("YOUTUBE_OAUTH_CLIENT_SECRET")
	c.YouTubeOAuthRedirectURL = os.Getenv("YOUTUBE_OAUTH_REDIRECT_URL")

	if strict {
		if c.YouTubeOAuthClientID == "" || c.YouTubeOAuthClientSecret == "" || c.YouTubeOAuthRedirectURL == "" {
			return nil, fmt.Errorf("YOUTUBE_OAUTH_CLIENT_ID, YOUTUBE_OAUTH_CLIENT_SECRET, and YOUTUBE_OAUTH_REDIRECT_URL are required")
		}
	}

	if v := os.Getenv("CHANNEL_DELETION_GRACE_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("CHANNEL_DELETION_GRACE_HOURS must be a non-negative integer")
		}
		c.ChannelDeletionGraceHours = n
	}

	if v := os.Getenv("TOKEN_EXPIRY_WARNING_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("TOKEN_EXPIRY_WARNING_DAYS must be a non-negative integer")
		}
		c.TokenExpiryWarningDays = n
	}

	if v := os.Getenv("OAUTH_STATE_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("OAUTH_STATE_TTL: %w", err)
		}
		c.OAuthStateTTL = d
	}

	if v := os.Getenv("INGESTION_WORKER_ENABLED"); v != "" {
		c.IngestionWorkerEnabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("INGESTION_TICK_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("INGESTION_TICK_INTERVAL: %w", err)
		}
		c.IngestionTickInterval = d
	}
	if v := os.Getenv("INGESTION_POLL_INTERVAL_MINUTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("INGESTION_POLL_INTERVAL_MINUTES must be a positive integer")
		}
		c.IngestionPollIntervalMinutes = n
	}
	if v := os.Getenv("INGESTION_MAX_RETRIES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("INGESTION_MAX_RETRIES must be a non-negative integer")
		}
		c.IngestionMaxRetries = n
	}
	if v := os.Getenv("INGESTION_JOB_DEADLINE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("INGESTION_JOB_DEADLINE: %w", err)
		}
		c.IngestionJobDeadline = d
	}
	if v := os.Getenv("INGESTION_MAX_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("INGESTION_MAX_CONCURRENCY must be a positive integer")
		}
		c.IngestionMaxConcurrency = n
	}
	c.IngestionDeadLetterAlertTo = os.Getenv("INGESTION_DEAD_LETTER_ALERT_TO")
	c.YouTubeAPIKey = os.Getenv("YOUTUBE_API_KEY")
	if v := os.Getenv("YOUTUBE_DAILY_QUOTA_UNITS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("YOUTUBE_DAILY_QUOTA_UNITS must be a non-negative integer")
		}
		c.YouTubeDailyQuotaUnits = n
	}
	for _, env := range []struct {
		name string
		dest *int
	}{
		{"INGESTION_MAX_REQUESTS_PER_JOB", &c.IngestionMaxRequestsPerJob},
		{"INGESTION_MAX_VIDEOS_PER_JOB", &c.IngestionMaxVideosPerJob},
		{"INGESTION_MAX_THREAD_PAGES_PER_JOB", &c.IngestionMaxThreadPagesPerJob},
		{"INGESTION_MAX_REPLY_FETCHES_PER_JOB", &c.IngestionMaxReplyFetchesPerJob},
		{"INGESTION_POLL_INTERVAL_MAX_MULTIPLIER", &c.IngestionPollIntervalMaxMultiplier},
	} {
		if v := os.Getenv(env.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%s must be a non-negative integer", env.name)
			}
			*env.dest = n
		}
	}

	// --- LLM processing ----------------------------------------------------
	if v := os.Getenv("LLM_PROCESSOR_ENABLED"); v != "" {
		c.LLMProcessorEnabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("LLM_PROCESSOR_TICK_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("LLM_PROCESSOR_TICK_INTERVAL: %w", err)
		}
		c.LLMProcessorTickInterval = d
	}
	if v := os.Getenv("LLM_PROCESSOR_BATCH_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("LLM_PROCESSOR_BATCH_TIMEOUT must be a positive duration")
		}
		c.LLMProcessorBatchTimeout = d
	}
	if v := os.Getenv("LLM_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("LLM_BATCH_SIZE must be a positive integer")
		}
		c.LLMBatchSize = n
	}
	if v := os.Getenv("LLM_CONFIDENCE_THRESHOLD"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f > 1 {
			return nil, fmt.Errorf("LLM_CONFIDENCE_THRESHOLD must be a float in [0,1]")
		}
		c.LLMConfidenceThreshold = f
	}
	if v := os.Getenv("LLM_PROCESSOR_SHUTDOWN_GRACE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("LLM_PROCESSOR_SHUTDOWN_GRACE: %w", err)
		}
		c.LLMProcessorShutdownGrace = d
	}
	c.OpenCodeAPIKey = os.Getenv("OPENCODE_API_KEY")
	if v := os.Getenv("OPENCODE_BASE_URL"); v != "" {
		c.OpenCodeBaseURL = v
	}
	if v := os.Getenv("OPENCODE_MODEL"); v != "" {
		c.OpenCodeModel = v
	}

	// --- LLM provider failover (implement-missing-product-features 6.4) ----
	// LLM_PRIMARY defaults to "opencode" (sole provider today); LLM_FALLBACK
	// empty = no failover. Values validated by the LLM package consumers.
	if v := os.Getenv("LLM_PRIMARY"); v != "" {
		c.LLMPrimaryProvider = v
	}
	c.LLMFallbackProvider = os.Getenv("LLM_FALLBACK")

	// --- Insights API ------------------------------------------------------
	if v := os.Getenv("INSIGHTS_API_ENABLED"); v != "" {
		c.InsightsAPIEnabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("EXPORT_DIR"); v != "" {
		c.ExportDir = v
	}
	if v := os.Getenv("EXPORT_SYNC_THRESHOLD"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("EXPORT_SYNC_THRESHOLD must be a positive integer")
		}
		c.ExportSyncThreshold = n
	}
	if v := os.Getenv("EXPORT_TTL_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("EXPORT_TTL_HOURS must be a positive integer")
		}
		c.ExportTTLHours = n
	}
	if v := os.Getenv("EXPORT_WORKER_TICK_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("EXPORT_WORKER_TICK_INTERVAL: %w", err)
		}
		c.ExportWorkerTickInterval = d
	}
	if v := os.Getenv("EXPORT_WORKER_ENABLED"); v != "" {
		c.ExportWorkerEnabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("FEED_DEFAULT_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("FEED_DEFAULT_LIMIT must be a positive integer")
		}
		c.FeedDefaultLimit = n
	}
	if v := os.Getenv("FEED_MAX_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("FEED_MAX_LIMIT must be a positive integer")
		}
		c.FeedMaxLimit = n
	}
	if v := os.Getenv("FEED_QUERY_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("FEED_QUERY_TIMEOUT: %w", err)
		}
		c.FeedQueryTimeout = d
	}
	if v := os.Getenv("COUNT_QUERY_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("COUNT_QUERY_TIMEOUT: %w", err)
		}
		c.CountQueryTimeout = d
	}

	// --- Billing (billing change) -----------------------------------------
	if v := os.Getenv("BILLING_ENABLED"); v != "" {
		c.BillingEnabled = v == "1" || v == "true" || v == "yes"
	}
	c.StripeSecretKey = os.Getenv("STRIPE_SECRET_KEY")
	c.StripeWebhookSecret = os.Getenv("STRIPE_WEBHOOK_SECRET")
	c.StripePriceCreatorMonthly = os.Getenv("STRIPE_PRICE_CREATOR_MONTHLY")
	c.StripePriceCreatorAnnual = os.Getenv("STRIPE_PRICE_CREATOR_ANNUAL")
	c.StripePriceStudioMonthly = os.Getenv("STRIPE_PRICE_STUDIO_MONTHLY")
	c.StripePriceStudioAnnual = os.Getenv("STRIPE_PRICE_STUDIO_ANNUAL")
	c.StripePriceAgencyMonthly = os.Getenv("STRIPE_PRICE_AGENCY_MONTHLY")
	c.StripePriceAgencyAnnual = os.Getenv("STRIPE_PRICE_AGENCY_ANNUAL")
	c.BillingCheckoutSuccessURL = os.Getenv("BILLING_CHECKOUT_SUCCESS_URL")
	c.BillingCheckoutCancelURL = os.Getenv("BILLING_CHECKOUT_CANCEL_URL")
	if v := os.Getenv("BILLING_OVERAGE_INVOICING_ENABLED"); v != "" {
		c.BillingOverageInvoicingEnabled = v == "1" || v == "true" || v == "yes"
	}

	// Task 2.4: when billing is enabled all Stripe vars are mandatory.
	if c.BillingEnabled {
		required := map[string]string{
			"STRIPE_SECRET_KEY":            c.StripeSecretKey,
			"STRIPE_WEBHOOK_SECRET":        c.StripeWebhookSecret,
			"STRIPE_PRICE_CREATOR_MONTHLY": c.StripePriceCreatorMonthly,
			"STRIPE_PRICE_CREATOR_ANNUAL":  c.StripePriceCreatorAnnual,
			"STRIPE_PRICE_STUDIO_MONTHLY":  c.StripePriceStudioMonthly,
			"STRIPE_PRICE_STUDIO_ANNUAL":   c.StripePriceStudioAnnual,
			"STRIPE_PRICE_AGENCY_MONTHLY":  c.StripePriceAgencyMonthly,
			"STRIPE_PRICE_AGENCY_ANNUAL":   c.StripePriceAgencyAnnual,
			"BILLING_CHECKOUT_SUCCESS_URL": c.BillingCheckoutSuccessURL,
			"BILLING_CHECKOUT_CANCEL_URL":  c.BillingCheckoutCancelURL,
		}
		for k, v := range required {
			if v == "" {
				return nil, fmt.Errorf("BILLING_ENABLED=true but %s is unset", k)
			}
		}
	}

	// --- Transactional email (data-lifecycle change) ----------------------
	// Tasks 2.1, 2.3. Required-when-enabled validation mirrors the billing
	// pattern above so misconfigured deployments fail fast at startup rather
	// than at first email send.
	if v := os.Getenv("EMAIL_ENABLED"); v != "" {
		c.EmailEnabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("EMAIL_PROVIDER"); v != "" {
		c.EmailProvider = v
	}
	c.EmailFromAddress = os.Getenv("EMAIL_FROM_ADDRESS")
	c.EmailFromName = os.Getenv("EMAIL_FROM_NAME")
	c.SMTPHost = os.Getenv("SMTP_HOST")
	if v := os.Getenv("SMTP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			return nil, fmt.Errorf("SMTP_PORT must be a TCP port in [1,65535]")
		}
		c.SMTPPort = n
	}
	c.SMTPUsername = os.Getenv("SMTP_USERNAME")
	c.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	if v := os.Getenv("SMTP_CMD_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("SMTP_CMD_TIMEOUT must be a positive duration (e.g. 10s)")
		}
		c.SMTPCmdTimeout = d
	}
	c.AppSupportEmail = os.Getenv("APP_SUPPORT_EMAIL")

	if c.EmailEnabled {
		// EMAIL_PROVIDER=smtp is the only supported provider today; reject
		// unknown values rather than silently no-op.
		if c.EmailProvider != "smtp" {
			return nil, fmt.Errorf("EMAIL_ENABLED=true but EMAIL_PROVIDER=%q is unsupported (only \"smtp\")", c.EmailProvider)
		}
		required := map[string]string{
			"EMAIL_FROM_ADDRESS": c.EmailFromAddress,
			"EMAIL_FROM_NAME":    c.EmailFromName,
			"SMTP_HOST":          c.SMTPHost,
			"SMTP_USERNAME":      c.SMTPUsername,
			"SMTP_PASSWORD":      c.SMTPPassword,
			"APP_SUPPORT_EMAIL":  c.AppSupportEmail,
		}
		for k, v := range required {
			if v == "" {
				return nil, fmt.Errorf("EMAIL_ENABLED=true but %s is unset", k)
			}
		}
		if c.SMTPPort == 0 {
			return nil, fmt.Errorf("EMAIL_ENABLED=true but SMTP_PORT is unset")
		}
	}

	// --- Lifecycle workers + account API (data-lifecycle change) ----------
	// Tasks 2.2, 2.4.
	if v := os.Getenv("LIFECYCLE_WORKERS_ENABLED"); v != "" {
		c.LifecycleWorkersEnabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("ACCOUNT_LIFECYCLE_API_ENABLED"); v != "" {
		c.AccountLifecycleAPIEnabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("EXPORT_STORAGE_DIR"); v != "" {
		c.ExportStorageDir = v
	}
	c.ExportSignedURLSecret = os.Getenv("EXPORT_SIGNED_URL_SECRET")
	if v := os.Getenv("EXPORT_LINK_TTL_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("EXPORT_LINK_TTL_DAYS must be a positive integer")
		}
		c.ExportLinkTTLDays = n
	}
	if v := os.Getenv("DELETION_REVERSIBLE_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("DELETION_REVERSIBLE_HOURS must be a non-negative integer")
		}
		c.DeletionReversibleHours = n
	}
	if v := os.Getenv("DELETION_CHUNK_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("DELETION_CHUNK_SIZE must be a positive integer")
		}
		c.DeletionChunkSize = n
	}
	if v := os.Getenv("EXPORT_PRE_DELETE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("EXPORT_PRE_DELETE_TIMEOUT: %w", err)
		}
		c.ExportPreDeleteTimeout = d
	}

	if c.LifecycleWorkersEnabled {
		// Required when workers run: signing secret (export download URLs)
		// and a writable storage dir. The API endpoints alone can be
		// enabled without these set, but the workers cannot operate.
		if c.ExportSignedURLSecret == "" {
			return nil, fmt.Errorf("LIFECYCLE_WORKERS_ENABLED=true but EXPORT_SIGNED_URL_SECRET is unset")
		}
		if c.ExportStorageDir == "" {
			return nil, fmt.Errorf("LIFECYCLE_WORKERS_ENABLED=true but EXPORT_STORAGE_DIR is unset")
		}
		// The deletion/export workers always emit transactional email
		// (deletion_scheduled, export_ready, deletion_completed); operating
		// the workers without the email pipeline would silently drop those
		// notifications, so we reject the misconfiguration up front.
		if !c.EmailEnabled {
			return nil, fmt.Errorf("LIFECYCLE_WORKERS_ENABLED=true requires EMAIL_ENABLED=true")
		}
	}

	// --- OpenTelemetry (otel-instrumentation change) ---------------------
	// Parsed last so any future required-when-enabled validation can refer
	// to other already-parsed fields. Defaults preserve zero-config local
	// dev: OTEL_ENABLED=false, no OTLP traffic.
	if v := os.Getenv("OTEL_ENABLED"); v != "" {
		c.OTELEnabled = v == "1" || v == "true" || v == "yes"
	} else {
		c.OTELEnabled = false
	}
	c.OTELExporterOTLPEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if v := os.Getenv("OTEL_SERVICE_NAME"); v != "" {
		c.OTELServiceName = v
	} else {
		c.OTELServiceName = "noumena-api"
	}
	c.OTELResourceAttributes = os.Getenv("OTEL_RESOURCE_ATTRIBUTES")
	if c.OTELEnabled && c.OTELExporterOTLPEndpoint == "" {
		return nil, fmt.Errorf("OTEL_ENABLED=true but OTEL_EXPORTER_OTLP_ENDPOINT is unset")
	}

	// --- Rate limiting (spec-gap-improvements) ----------------------------
	if v := os.Getenv("RATE_LIMIT_ENABLED"); v != "" {
		c.RateLimitEnabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("RATE_LIMIT_AUTH_MAX_PER_MIN"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("RATE_LIMIT_AUTH_MAX_PER_MIN must be a positive integer")
		}
		c.RateLimitAuthMaxPerMin = n
	}
	if v := os.Getenv("RATE_LIMIT_UNAUTH_MAX_PER_MIN"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("RATE_LIMIT_UNAUTH_MAX_PER_MIN must be a positive integer")
		}
		c.RateLimitUnauthMaxPerMin = n
	}
	if v := os.Getenv("RATE_LIMIT_CLEANUP_TICK"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("RATE_LIMIT_CLEANUP_TICK: %w", err)
		}
		c.RateLimitCleanupTick = d
	}

	// --- HTTP hardening (fix-crypto-http-hardening) ----------------------
	c.TrustProxyHeader = os.Getenv("TRUST_PROXY_HEADER")
	if v := os.Getenv("MAX_BODY_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("MAX_BODY_BYTES must be a positive integer")
		}
		c.MaxBodyBytes = n
	}
	if v := os.Getenv("SECURITY_CSP"); v != "" {
		c.CSPHeader = v
	}

	if v := os.Getenv("COOKIE_SECURE"); v != "" {
		c.CookieSecure = v == "1" || v == "true" || v == "yes"
	}

	Default = &c
	return &c, nil
}

// validateEncryptionKey rejects empty or invalid-size AES keys. The env
// value is used as raw key bytes (16/24/32 bytes for AES-128/192/256).
func validateEncryptionKey(key string) error {
	if key == "" {
		return fmt.Errorf("ENCRYPTION_KEY is required (generate one with: openssl rand -base64 24; 16/24/32-byte raw value)")
	}
	switch len(key) {
	case 16, 24, 32:
		return nil
	default:
		return fmt.Errorf("ENCRYPTION_KEY must be 16, 24, or 32 bytes (got %d bytes)", len(key))
	}
}

// EnsureExportDir validates that the configured EXPORT_DIR exists and is
// writable, creating it with mode 0700 if missing. Task 2.2.
func EnsureExportDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat EXPORT_DIR %q: %w", path, err)
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create EXPORT_DIR %q: %w", path, err)
		}
		return nil
	}
	if !info.IsDir() {
		return fmt.Errorf("EXPORT_DIR %q is not a directory", path)
	}
	// Probe writability with a unique tempfile.
	probe, err := os.CreateTemp(path, ".noumena-write-probe-*")
	if err != nil {
		return fmt.Errorf("EXPORT_DIR %q is not writable: %w", path, err)
	}
	probeName := probe.Name()
	_ = probe.Close()
	_ = os.Remove(probeName)
	return nil
}
