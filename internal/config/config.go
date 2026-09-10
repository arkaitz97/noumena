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
	GoogleSSOClientID         string
	GoogleSSOClientSecret     string
	GoogleSSORedirectURL      string
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
	IngestionWorkerEnabled             bool
	IngestionTickInterval              time.Duration
	IngestionPollIntervalMinutes       int
	IngestionMaxRetries                int
	IngestionJobDeadline               time.Duration
	IngestionMaxConcurrency            int
	IngestionDeadLetterAlertTo         string
	YouTubeAPIKey                      string
	YouTubeDailyQuotaUnits             int
	IngestionMaxRequestsPerJob         int
	IngestionMaxVideosPerJob           int
	IngestionMaxThreadPagesPerJob      int
	IngestionMaxReplyFetchesPerJob     int
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

	// Billing (billing change)
	BillingEnabled                 bool
	StripeSecretKey                string
	StripeWebhookSecret            string
	StripePriceCreatorMonthly      string
	StripePriceCreatorAnnual       string
	StripePriceStudioMonthly       string
	StripePriceStudioAnnual        string
	StripePriceAgencyMonthly       string
	StripePriceAgencyAnnual        string
	BillingCheckoutSuccessURL      string
	BillingCheckoutCancelURL       string
	BillingOverageInvoicingEnabled bool

	// LLM provider failover
	LLMPrimaryProvider  string
	LLMFallbackProvider string

	// Transactional email pipeline
	EmailEnabled     bool
	EmailProvider    string
	EmailFromAddress string
	EmailFromName    string
	SMTPHost         string
	SMTPPort         int
	SMTPUsername     string
	SMTPPassword     string
	SMTPCmdTimeout   time.Duration
	AppSupportEmail  string

	// Lifecycle workers
	LifecycleWorkersEnabled    bool
	AccountLifecycleAPIEnabled bool
	ExportStorageDir           string
	ExportSignedURLSecret      string
	ExportLinkTTLDays          int
	DeletionReversibleHours    int
	DeletionChunkSize          int
	ExportPreDeleteTimeout     time.Duration

	// OpenTelemetry
	OTELEnabled              bool
	OTELExporterOTLPEndpoint string
	OTELServiceName          string
	OTELResourceAttributes   string

	// Rate limiting
	RateLimitEnabled         bool
	RateLimitAuthMaxPerMin   int
	RateLimitUnauthMaxPerMin int
	RateLimitCleanupTick     time.Duration

	// HTTP hardening
	TrustProxyHeader string
	MaxBodyBytes     int64
	CSPHeader        string

	// Cookie security
	CookieSecure bool
}

// Default holds the process-wide configuration.
var Default = &Config{
	JWTSecret:                 []byte("dev-only-insecure-secret-change-me"),
	JWTAccessTTL:              15 * time.Minute,
	JWTRefreshTTL:             720 * time.Hour,
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
	YouTubeDailyQuotaUnits:             10000,
	IngestionMaxRequestsPerJob:         150,
	IngestionMaxVideosPerJob:           100,
	IngestionMaxThreadPagesPerJob:      25,
	IngestionMaxReplyFetchesPerJob:     100,
	IngestionPollIntervalMaxMultiplier: 8,

	LLMProcessorEnabled:       false,
	LLMProcessorTickInterval:  10 * time.Second,
	LLMProcessorBatchTimeout:  30 * time.Second,
	LLMBatchSize:              50,
	LLMConfidenceThreshold:    0.7,
	LLMProcessorShutdownGrace: 30 * time.Second,
	OpenCodeBaseURL:           "https://api.opencode.local/v1",
	OpenCodeModel:             "opencode/default",

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

	OTELEnabled:     false,
	OTELServiceName: "noumena-api",

	RateLimitEnabled:         false,
	RateLimitAuthMaxPerMin:   100,
	RateLimitUnauthMaxPerMin: 10,
	RateLimitCleanupTick:     5 * time.Minute,

	TrustProxyHeader: "",
	MaxBodyBytes:     2 << 20,
	CSPHeader:        "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; media-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'",
	CookieSecure:     true,

	BillingOverageInvoicingEnabled: false,
	LLMPrimaryProvider:             "opencode",
	LLMFallbackProvider:            "",
}

// Load reads configuration from environment variables, applying defaults
// where values are missing. It returns an error if any *required* value is
// missing or malformed.
func Load(strict bool) (*Config, error) {
	c := *Default

	// --- Mandatory JWT signing secret --------------------------------------
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
	if strings.Trim(secret, secret[:1]) == "" {
		log.Printf("WARN: JWT_SECRET has trivially low entropy (single repeated character); generate one with: openssl rand -base64 32")
	}

	// --- Mandatory encryption key -----------------------------------------
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

	// --- Auth / JWT -------------------------------------------------------
	if v := os.Getenv("JWT_ACCESS_TTL"); v != "" {
		c.JWTAccessTTL, _ = envDuration("JWT_ACCESS_TTL", v)
	}
	if v := os.Getenv("JWT_REFRESH_TTL"); v != "" {
		c.JWTRefreshTTL, _ = envDuration("JWT_REFRESH_TTL", v)
	}
	if v := os.Getenv("BCRYPT_COST"); v != "" {
		n, err := envInt("BCRYPT_COST", v)
		if err != nil {
			return nil, err
		}
		if n < 12 {
			return nil, fmt.Errorf("BCRYPT_COST must be >= 12, got %d", n)
		}
		c.BcryptCost = n
	}
	if v := os.Getenv("EMAIL_VERIFICATION_TTL"); v != "" {
		c.EmailVerificationTTL, _ = envDuration("EMAIL_VERIFICATION_TTL", v)
	}
	if v := os.Getenv("EMAIL_VERIFICATION_REQUIRED"); v != "" {
		c.EmailVerificationRequired = truthy(v)
	}
	if v := os.Getenv("PASSWORD_RESET_TTL"); v != "" {
		c.PasswordResetTTL, _ = envDuration("PASSWORD_RESET_TTL", v)
	}

	appBaseURL, appBaseURLSet := lookupEnv("APP_BASE_URL")
	if appBaseURLSet {
		c.AppBaseURL = appBaseURL
	}
	if !appBaseURLSet && strict {
		return nil, fmt.Errorf("APP_BASE_URL is required")
	}

	// --- Google SSO -------------------------------------------------------
	c.GoogleSSOClientID = os.Getenv("GOOGLE_SSO_CLIENT_ID")
	c.GoogleSSOClientSecret = os.Getenv("GOOGLE_SSO_CLIENT_SECRET")
	c.GoogleSSORedirectURL = os.Getenv("GOOGLE_SSO_REDIRECT_URL")
	c.GoogleSSOPopupRedirectURL = os.Getenv("GOOGLE_SSO_POPUP_REDIRECT_URL")
	if c.GoogleSSOPopupRedirectURL == "" {
		c.GoogleSSOPopupRedirectURL = c.GoogleSSORedirectURL
	}
	if strict {
		if c.GoogleSSOClientID == "" || c.GoogleSSOClientSecret == "" || c.GoogleSSORedirectURL == "" {
			return nil, fmt.Errorf("GOOGLE_SSO_CLIENT_ID, GOOGLE_SSO_CLIENT_SECRET, and GOOGLE_SSO_REDIRECT_URL are required")
		}
	}

	// --- YouTube OAuth ----------------------------------------------------
	c.YouTubeOAuthClientID = os.Getenv("YOUTUBE_OAUTH_CLIENT_ID")
	c.YouTubeOAuthClientSecret = os.Getenv("YOUTUBE_OAUTH_CLIENT_SECRET")
	c.YouTubeOAuthRedirectURL = os.Getenv("YOUTUBE_OAUTH_REDIRECT_URL")
	if strict {
		if c.YouTubeOAuthClientID == "" || c.YouTubeOAuthClientSecret == "" || c.YouTubeOAuthRedirectURL == "" {
			return nil, fmt.Errorf("YOUTUBE_OAUTH_CLIENT_ID, YOUTUBE_OAUTH_CLIENT_SECRET, and YOUTUBE_OAUTH_REDIRECT_URL are required")
		}
	}

	// --- Channel-management -----------------------------------------------
	c.ChannelDeletionGraceHours = envIntDef("CHANNEL_DELETION_GRACE_HOURS", c.ChannelDeletionGraceHours)
	c.TokenExpiryWarningDays = envIntDef("TOKEN_EXPIRY_WARNING_DAYS", c.TokenExpiryWarningDays)
	if v := os.Getenv("OAUTH_STATE_TTL"); v != "" {
		c.OAuthStateTTL, _ = envDuration("OAUTH_STATE_TTL", v)
	}

	// --- Ingestion pipeline -----------------------------------------------
	c.IngestionWorkerEnabled = envBool(os.Getenv("INGESTION_WORKER_ENABLED"))
	if v := os.Getenv("INGESTION_TICK_INTERVAL"); v != "" {
		c.IngestionTickInterval, _ = envDuration("INGESTION_TICK_INTERVAL", v)
	}
	c.IngestionPollIntervalMinutes = envIntDef("INGESTION_POLL_INTERVAL_MINUTES", c.IngestionPollIntervalMinutes)
	c.IngestionMaxRetries = envIntDef("INGESTION_MAX_RETRIES", c.IngestionMaxRetries)
	if v := os.Getenv("INGESTION_JOB_DEADLINE"); v != "" {
		c.IngestionJobDeadline, _ = envDuration("INGESTION_JOB_DEADLINE", v)
	}
	c.IngestionMaxConcurrency = envIntDef("INGESTION_MAX_CONCURRENCY", c.IngestionMaxConcurrency)
	c.IngestionDeadLetterAlertTo = os.Getenv("INGESTION_DEAD_LETTER_ALERT_TO")
	c.YouTubeAPIKey = os.Getenv("YOUTUBE_API_KEY")
	c.YouTubeDailyQuotaUnits = envIntDef("YOUTUBE_DAILY_QUOTA_UNITS", c.YouTubeDailyQuotaUnits)
	for _, e := range []struct {
		name string
		dest *int
	}{
		{"INGESTION_MAX_REQUESTS_PER_JOB", &c.IngestionMaxRequestsPerJob},
		{"INGESTION_MAX_VIDEOS_PER_JOB", &c.IngestionMaxVideosPerJob},
		{"INGESTION_MAX_THREAD_PAGES_PER_JOB", &c.IngestionMaxThreadPagesPerJob},
		{"INGESTION_MAX_REPLY_FETCHES_PER_JOB", &c.IngestionMaxReplyFetchesPerJob},
		{"INGESTION_POLL_INTERVAL_MAX_MULTIPLIER", &c.IngestionPollIntervalMaxMultiplier},
	} {
		*e.dest = envIntDef(e.name, *e.dest)
	}

	// --- LLM processing ---------------------------------------------------
	c.LLMProcessorEnabled = envBool(os.Getenv("LLM_PROCESSOR_ENABLED"))
	if v := os.Getenv("LLM_PROCESSOR_TICK_INTERVAL"); v != "" {
		c.LLMProcessorTickInterval, _ = envDuration("LLM_PROCESSOR_TICK_INTERVAL", v)
	}
	if v := os.Getenv("LLM_PROCESSOR_BATCH_TIMEOUT"); v != "" {
		d, err := envDuration("LLM_PROCESSOR_BATCH_TIMEOUT", v)
		if err != nil {
			return nil, err
		}
		if d <= 0 {
			return nil, fmt.Errorf("LLM_PROCESSOR_BATCH_TIMEOUT must be a positive duration")
		}
		c.LLMProcessorBatchTimeout = d
	}
	c.LLMBatchSize = envIntDef("LLM_BATCH_SIZE", c.LLMBatchSize)
	if v := os.Getenv("LLM_CONFIDENCE_THRESHOLD"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f > 1 {
			return nil, fmt.Errorf("LLM_CONFIDENCE_THRESHOLD must be a float in [0,1]")
		}
		c.LLMConfidenceThreshold = f
	}
	if v := os.Getenv("LLM_PROCESSOR_SHUTDOWN_GRACE"); v != "" {
		c.LLMProcessorShutdownGrace, _ = envDuration("LLM_PROCESSOR_SHUTDOWN_GRACE", v)
	}
	c.OpenCodeAPIKey = os.Getenv("OPENCODE_API_KEY")
	if v := os.Getenv("OPENCODE_BASE_URL"); v != "" {
		c.OpenCodeBaseURL = v
	}
	if v := os.Getenv("OPENCODE_MODEL"); v != "" {
		c.OpenCodeModel = v
	}

	// --- LLM provider failover --------------------------------------------
	if v := os.Getenv("LLM_PRIMARY"); v != "" {
		c.LLMPrimaryProvider = v
	}
	c.LLMFallbackProvider = os.Getenv("LLM_FALLBACK")

	// --- Insights API -----------------------------------------------------
	c.InsightsAPIEnabled = envBool(os.Getenv("INSIGHTS_API_ENABLED"))
	if v := os.Getenv("EXPORT_DIR"); v != "" {
		c.ExportDir = v
	}
	c.ExportSyncThreshold = envIntDef("EXPORT_SYNC_THRESHOLD", c.ExportSyncThreshold)
	c.ExportTTLHours = envIntDef("EXPORT_TTL_HOURS", c.ExportTTLHours)
	if v := os.Getenv("EXPORT_WORKER_TICK_INTERVAL"); v != "" {
		c.ExportWorkerTickInterval, _ = envDuration("EXPORT_WORKER_TICK_INTERVAL", v)
	}
	c.ExportWorkerEnabled = envBool(os.Getenv("EXPORT_WORKER_ENABLED"))
	c.FeedDefaultLimit = envIntDef("FEED_DEFAULT_LIMIT", c.FeedDefaultLimit)
	c.FeedMaxLimit = envIntDef("FEED_MAX_LIMIT", c.FeedMaxLimit)
	if v := os.Getenv("FEED_QUERY_TIMEOUT"); v != "" {
		c.FeedQueryTimeout, _ = envDuration("FEED_QUERY_TIMEOUT", v)
	}
	if v := os.Getenv("COUNT_QUERY_TIMEOUT"); v != "" {
		c.CountQueryTimeout, _ = envDuration("COUNT_QUERY_TIMEOUT", v)
	}

	// --- Billing ----------------------------------------------------------
	c.BillingEnabled = envBool(os.Getenv("BILLING_ENABLED"))
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
	c.BillingOverageInvoicingEnabled = envBool(os.Getenv("BILLING_OVERAGE_INVOICING_ENABLED"))

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

	// --- Transactional email ----------------------------------------------
	c.EmailEnabled = envBool(os.Getenv("EMAIL_ENABLED"))
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
		d, err := envDuration("SMTP_CMD_TIMEOUT", v)
		if err != nil {
			return nil, err
		}
		if d <= 0 {
			return nil, fmt.Errorf("SMTP_CMD_TIMEOUT must be a positive duration (e.g. 10s)")
		}
		c.SMTPCmdTimeout = d
	}
	c.AppSupportEmail = os.Getenv("APP_SUPPORT_EMAIL")

	if c.EmailEnabled {
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

	// --- Lifecycle workers + account API -----------------------------------
	c.LifecycleWorkersEnabled = envBool(os.Getenv("LIFECYCLE_WORKERS_ENABLED"))
	c.AccountLifecycleAPIEnabled = envBool(os.Getenv("ACCOUNT_LIFECYCLE_API_ENABLED"))
	if v := os.Getenv("EXPORT_STORAGE_DIR"); v != "" {
		c.ExportStorageDir = v
	}
	c.ExportSignedURLSecret = os.Getenv("EXPORT_SIGNED_URL_SECRET")
	c.ExportLinkTTLDays = envIntDef("EXPORT_LINK_TTL_DAYS", c.ExportLinkTTLDays)
	c.DeletionReversibleHours = envIntDef("DELETION_REVERSIBLE_HOURS", c.DeletionReversibleHours)
	c.DeletionChunkSize = envIntDef("DELETION_CHUNK_SIZE", c.DeletionChunkSize)
	if v := os.Getenv("EXPORT_PRE_DELETE_TIMEOUT"); v != "" {
		c.ExportPreDeleteTimeout, _ = envDuration("EXPORT_PRE_DELETE_TIMEOUT", v)
	}

	if c.LifecycleWorkersEnabled {
		if c.ExportSignedURLSecret == "" {
			return nil, fmt.Errorf("LIFECYCLE_WORKERS_ENABLED=true but EXPORT_SIGNED_URL_SECRET is unset")
		}
		if c.ExportStorageDir == "" {
			return nil, fmt.Errorf("LIFECYCLE_WORKERS_ENABLED=true but EXPORT_STORAGE_DIR is unset")
		}
		if !c.EmailEnabled {
			return nil, fmt.Errorf("LIFECYCLE_WORKERS_ENABLED=true requires EMAIL_ENABLED=true")
		}
	}

	// --- OpenTelemetry ----------------------------------------------------
	c.OTELEnabled = truthy(os.Getenv("OTEL_ENABLED"))
	c.OTELExporterOTLPEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	c.OTELServiceName = envOr("OTEL_SERVICE_NAME", "noumena-api")
	c.OTELResourceAttributes = os.Getenv("OTEL_RESOURCE_ATTRIBUTES")
	if c.OTELEnabled && c.OTELExporterOTLPEndpoint == "" {
		return nil, fmt.Errorf("OTEL_ENABLED=true but OTEL_EXPORTER_OTLP_ENDPOINT is unset")
	}

	// --- Rate limiting ----------------------------------------------------
	c.RateLimitEnabled = envBool(os.Getenv("RATE_LIMIT_ENABLED"))
	c.RateLimitAuthMaxPerMin = envIntDef("RATE_LIMIT_AUTH_MAX_PER_MIN", c.RateLimitAuthMaxPerMin)
	c.RateLimitUnauthMaxPerMin = envIntDef("RATE_LIMIT_UNAUTH_MAX_PER_MIN", c.RateLimitUnauthMaxPerMin)
	if v := os.Getenv("RATE_LIMIT_CLEANUP_TICK"); v != "" {
		c.RateLimitCleanupTick, _ = envDuration("RATE_LIMIT_CLEANUP_TICK", v)
	}

	// --- HTTP hardening ---------------------------------------------------
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
	c.CookieSecure = envBool(os.Getenv("COOKIE_SECURE"))

	Default = &c
	return &c, nil
}

// ---------------------------------------------------------------------------
// Env helpers
// ---------------------------------------------------------------------------

// envDuration parses v as a time.Duration. Returns (duration, nil) on success
// or (0, error) on parse failure. The caller passes the pre-fetched env value
// so os.Getenv calls stay in Load() for the env-parity test.
func envDuration(key, v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

// envInt parses v as int. Returns (value, nil) on success or (0, error) on
// parse failure. Use for fields with custom validation (e.g. BCRYPT_COST >= 12).
func envInt(key, v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

// envIntDef parses v as int; on parse failure returns the default.
// Use for fields where a bad env value should not crash startup.
func envIntDef(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// envBool reports whether v means "on" (1|true|yes).
func envBool(v string) bool { return v == "1" || v == "true" || v == "yes" }

// lookupEnv returns the environment value when set and non-empty.
func lookupEnv(key string) (string, bool) {
	v := os.Getenv(key)
	return v, v != ""
}

// truthy reports whether an env value means "on" (1|true|yes).
func truthy(v string) bool { return v == "1" || v == "true" || v == "yes" }

// envOr returns the environment value, or fallback when unset/empty.
func envOr(key, fallback string) string {
	if v, ok := lookupEnv(key); ok {
		return v
	}
	return fallback
}

// validAESKeySizes are the byte lengths accepted for ENCRYPTION_KEY.
var validAESKeySizes = map[int]bool{16: true, 24: true, 32: true}

func validateEncryptionKey(key string) error {
	if key == "" {
		return fmt.Errorf("ENCRYPTION_KEY is required (generate one with: openssl rand -base64 24; 16/24/32-byte raw value)")
	}
	if validAESKeySizes[len(key)] {
		return nil
	}
	return fmt.Errorf("ENCRYPTION_KEY must be 16, 24, or 32 bytes (got %d bytes)", len(key))
}

// EnsureExportDir validates that the configured EXPORT_DIR exists and is
// writable, creating it with mode 0700 if missing.
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
	probe, err := os.CreateTemp(path, ".noumena-write-probe-*")
	if err != nil {
		return fmt.Errorf("EXPORT_DIR %q is not writable: %w", path, err)
	}
	probeName := probe.Name()
	_ = probe.Close()
	_ = os.Remove(probeName)
	return nil
}
