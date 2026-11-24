package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setValidJWTSecret sets a JWT_SECRET that satisfies the mandatory
// validation (>=32 bytes, no placeholders) for tests that call Load().
// ENCRYPTION_KEY is also mandatory since fix-crypto-http-hardening; set
// both so Load() succeeds.
func setValidJWTSecret(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret-0123456789abcdef0123456789abcdef")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
}

// TestLoadInsightsAPIDefaults asserts task 2.3: the loader applies the
// documented defaults when no env vars are set.
func TestLoadInsightsAPIDefaults(t *testing.T) {
	// Clear all insights-api env vars so the loader takes defaults.
	for _, k := range []string{
		"INSIGHTS_API_ENABLED", "EXPORT_DIR", "EXPORT_SYNC_THRESHOLD",
		"EXPORT_TTL_HOURS", "EXPORT_WORKER_TICK_INTERVAL", "EXPORT_WORKER_ENABLED",
		"FEED_DEFAULT_LIMIT", "FEED_MAX_LIMIT", "FEED_QUERY_TIMEOUT",
		"COUNT_QUERY_TIMEOUT",
	} {
		_ = os.Unsetenv(k)
	}
	setValidJWTSecret(t)

	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if c.InsightsAPIEnabled {
		t.Errorf("InsightsAPIEnabled default: want false, got true")
	}
	if c.ExportDir != "/var/lib/noumena/exports" {
		t.Errorf("ExportDir default: got %q", c.ExportDir)
	}
	if c.ExportSyncThreshold != 50000 {
		t.Errorf("ExportSyncThreshold default: got %d", c.ExportSyncThreshold)
	}
	if c.ExportTTLHours != 24 {
		t.Errorf("ExportTTLHours default: got %d", c.ExportTTLHours)
	}
	if c.ExportWorkerTickInterval != 5*time.Second {
		t.Errorf("ExportWorkerTickInterval default: got %v", c.ExportWorkerTickInterval)
	}
	if c.ExportWorkerEnabled {
		t.Errorf("ExportWorkerEnabled default: want false")
	}
	if c.FeedDefaultLimit != 20 {
		t.Errorf("FeedDefaultLimit default: got %d", c.FeedDefaultLimit)
	}
	if c.FeedMaxLimit != 100 {
		t.Errorf("FeedMaxLimit default: got %d", c.FeedMaxLimit)
	}
	if c.FeedQueryTimeout != 2000*time.Millisecond {
		t.Errorf("FeedQueryTimeout default: got %v", c.FeedQueryTimeout)
	}
	if c.CountQueryTimeout != 1500*time.Millisecond {
		t.Errorf("CountQueryTimeout default: got %v", c.CountQueryTimeout)
	}
}

// TestEnsureExportDirCreatesMissing asserts task 2.2: a missing directory is
// created with mode 0700, and the writability probe passes.
func TestEnsureExportDirCreatesMissing(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "exports", "nested")

	if err := EnsureExportDir(target); err != nil {
		t.Fatalf("EnsureExportDir: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory")
	}
	if mode := info.Mode().Perm(); mode != 0o700 {
		// On some FS (e.g. Windows) perms differ; tolerate but log.
		t.Logf("EXPORT_DIR mode: %v (expected 0700; tolerated on this FS)", mode)
	}
}

// TestLoadOpenCodeDefaultsAndOverrides asserts the migrate-llm-to-opencode
// change: OpenCode is the sole provider, OPENCODE_BASE_URL/OPENCODE_MODEL
// have sensible defaults, and the env-var loaders override them.
func TestLoadOpenCodeDefaultsAndOverrides(t *testing.T) {
	for _, k := range []string{
		"OPENCODE_API_KEY", "OPENCODE_BASE_URL", "OPENCODE_MODEL",
	} {
		_ = os.Unsetenv(k)
	}
	setValidJWTSecret(t)

	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OpenCodeBaseURL != "https://api.opencode.local/v1" {
		t.Errorf("OpenCodeBaseURL default: got %q", c.OpenCodeBaseURL)
	}
	if c.OpenCodeModel != "opencode/default" {
		t.Errorf("OpenCodeModel default: got %q", c.OpenCodeModel)
	}
	if c.OpenCodeAPIKey != "" {
		t.Errorf("OpenCodeAPIKey: want empty when env unset, got %q", c.OpenCodeAPIKey)
	}

	// Now override via env.
	t.Setenv("OPENCODE_API_KEY", "sk-opencode-xyz")
	t.Setenv("OPENCODE_BASE_URL", "https://opencode.example.test/v1")
	t.Setenv("OPENCODE_MODEL", "opencode/large")

	c2, err := Load(false)
	if err != nil {
		t.Fatalf("Load (with env): %v", err)
	}
	if c2.OpenCodeAPIKey != "sk-opencode-xyz" {
		t.Errorf("OpenCodeAPIKey override: got %q", c2.OpenCodeAPIKey)
	}
	if c2.OpenCodeBaseURL != "https://opencode.example.test/v1" {
		t.Errorf("OpenCodeBaseURL override: got %q", c2.OpenCodeBaseURL)
	}
	if c2.OpenCodeModel != "opencode/large" {
		t.Errorf("OpenCodeModel override: got %q", c2.OpenCodeModel)
	}
}

// TestLoadOTELDefaults verifies the otel-instrumentation defaults match the
// observability spec: OTEL_ENABLED=false, service.name="noumena-api", and
// the endpoint/attributes empty. Zero-config local dev MUST be unaffected.
func TestLoadOTELDefaults(t *testing.T) {
	for _, k := range []string{"OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME", "OTEL_RESOURCE_ATTRIBUTES"} {
		_ = os.Unsetenv(k)
	}
	setValidJWTSecret(t)
	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.OTELEnabled {
		t.Errorf("OTELEnabled default: want false")
	}
	if c.OTELServiceName != "noumena-api" {
		t.Errorf("OTELServiceName default: got %q", c.OTELServiceName)
	}
	if c.OTELExporterOTLPEndpoint != "" {
		t.Errorf("OTELExporterOTLPEndpoint default: got %q", c.OTELExporterOTLPEndpoint)
	}
	if c.OTELResourceAttributes != "" {
		t.Errorf("OTELResourceAttributes default: got %q", c.OTELResourceAttributes)
	}
}

// TestLoadOTELBooleanParsing exercises the 1/true/yes idiom.
func TestLoadOTELBooleanParsing(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	setValidJWTSecret(t)
	for _, truthy := range []string{"1", "true", "yes"} {
		t.Setenv("OTEL_ENABLED", truthy)
		c, err := Load(false)
		if err != nil {
			t.Fatalf("Load(%q): %v", truthy, err)
		}
		if !c.OTELEnabled {
			t.Errorf("OTELEnabled %q: want true", truthy)
		}
	}
	t.Setenv("OTEL_ENABLED", "false")
	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load(false): %v", err)
	}
	if c.OTELEnabled {
		t.Errorf("OTELEnabled \"false\": want false")
	}
}

// TestLoadOTELRequiredWhenEnabled asserts the config layer rejects
// OTEL_ENABLED=true with no endpoint, mirroring the BILLING_ENABLED pattern.
func TestLoadOTELRequiredWhenEnabled(t *testing.T) {
	_ = os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	t.Setenv("OTEL_ENABLED", "true")
	setValidJWTSecret(t)
	if _, err := Load(false); err == nil {
		t.Fatalf("Load: expected error when OTEL_ENABLED=true and endpoint unset")
	}
}

// TestLoadJWTSecretValidation asserts fix-auth-critical-security task 2:
// JWT_SECRET validation is unconditional — missing, placeholder, or short
// secrets fail even with strict=false; a valid 32-byte secret passes.
func TestLoadJWTSecretValidation(t *testing.T) {
	// Unset.
	_ = os.Unsetenv("JWT_SECRET")
	if _, err := Load(false); err == nil {
		t.Fatal("Load: expected error when JWT_SECRET unset (even with strict=false)")
	}

	// Legacy default.
	t.Setenv("JWT_SECRET", "dev-only-insecure-secret-change-me")
	if _, err := Load(false); err == nil {
		t.Fatal("Load: expected error when JWT_SECRET equals the legacy default")
	}

	// Documented placeholders.
	for _, v := range []string{"change-me-0123456789abcdef0123456789", "0123456789abcdef0123456789-replace-me"} {
		t.Setenv("JWT_SECRET", v)
		if _, err := Load(false); err == nil {
			t.Fatalf("Load: expected error for placeholder secret %q", v)
		}
	}

	// Too short.
	t.Setenv("JWT_SECRET", "short-secret")
	if _, err := Load(false); err == nil {
		t.Fatal("Load: expected error when JWT_SECRET shorter than 32 bytes")
	}

	// Exactly 32 bytes, no placeholder substrings.
	thirtyTwo := "0123456789abcdef0123456789abcdef" // 32 chars
	if len(thirtyTwo) != 32 {
		t.Fatalf("test constant must be 32 bytes, got %d", len(thirtyTwo))
	}
	t.Setenv("JWT_SECRET", thirtyTwo)
	t.Setenv("ENCRYPTION_KEY", thirtyTwo)
	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load: valid 32-byte secret should pass: %v", err)
	}
	if string(c.JWTSecret) != thirtyTwo {
		t.Errorf("JWTSecret mismatch: got %q", string(c.JWTSecret))
	}

	// Strict mode also passes with a valid secret.
	t.Setenv("APP_BASE_URL", "http://localhost:3000")
	t.Setenv("GOOGLE_SSO_CLIENT_ID", "client")
	t.Setenv("GOOGLE_SSO_CLIENT_SECRET", "secret")
	t.Setenv("GOOGLE_SSO_REDIRECT_URL", "http://localhost:8080/auth/google/callback")
	t.Setenv("YOUTUBE_OAUTH_CLIENT_ID", "yclient")
	t.Setenv("YOUTUBE_OAUTH_CLIENT_SECRET", "ysecret")
	t.Setenv("YOUTUBE_OAUTH_REDIRECT_URL", "http://localhost:8080/auth/youtube/callback")
	if _, err := Load(true); err != nil {
		t.Fatalf("Load(true): valid secret should not be rejected: %v", err)
	}
}

// TestLoadEncryptionKeyValidation asserts fix-crypto-http-hardening
// task 1.1: Load() refuses to start without ENCRYPTION_KEY (no hardcoded
// fallback), rejects invalid AES key sizes, and accepts an optional
// ENCRYPTION_KEY_PREVIOUS.
func TestLoadEncryptionKeyValidation(t *testing.T) {
	setValidJWTSecret(t)

	// Unset: mandatory, even with strict=false.
	_ = os.Unsetenv("ENCRYPTION_KEY")
	if _, err := Load(false); err == nil {
		t.Fatal("Load: expected error when ENCRYPTION_KEY unset")
	}

	// Invalid key size (AES keys must be 16/24/32 bytes raw).
	t.Setenv("ENCRYPTION_KEY", "too-short")
	if _, err := Load(false); err == nil {
		t.Fatal("Load: expected error for invalid ENCRYPTION_KEY size")
	}

	// Valid current key, optional previous key accepted.
	key := "0123456789abcdef0123456789abcdef"
	t.Setenv("ENCRYPTION_KEY", key)
	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load: valid ENCRYPTION_KEY should pass: %v", err)
	}
	if string(c.EncryptionKey) != key {
		t.Errorf("EncryptionKey mismatch: got %q", string(c.EncryptionKey))
	}
	if c.EncryptionKeyPrevious != nil {
		t.Error("EncryptionKeyPrevious: want nil when env unset")
	}

	t.Setenv("ENCRYPTION_KEY_PREVIOUS", key)
	c2, err := Load(false)
	if err != nil {
		t.Fatalf("Load: ENCRYPTION_KEY_PREVIOUS should be accepted: %v", err)
	}
	if string(c2.EncryptionKeyPrevious) != key {
		t.Errorf("EncryptionKeyPrevious mismatch: got %q", string(c2.EncryptionKeyPrevious))
	}

	// Invalid previous key size rejected.
	t.Setenv("ENCRYPTION_KEY_PREVIOUS", "nope")
	if _, err := Load(false); err == nil {
		t.Fatal("Load: expected error for invalid ENCRYPTION_KEY_PREVIOUS size")
	}
}

// TestLoadBillingOverageInvoicingDefaultsAndOverrides asserts the
// implement-missing-product-features billing toggle: default false, env
// overrides with the 1/true/yes idiom.
func TestLoadBillingOverageInvoicing(t *testing.T) {
	_ = os.Unsetenv("BILLING_OVERAGE_INVOICING_ENABLED")
	setValidJWTSecret(t)

	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.BillingOverageInvoicingEnabled {
		t.Error("BillingOverageInvoicingEnabled default: want false")
	}

	for _, truthy := range []string{"1", "true", "yes"} {
		t.Setenv("BILLING_OVERAGE_INVOICING_ENABLED", truthy)
		c2, err := Load(false)
		if err != nil {
			t.Fatalf("Load(%q): %v", truthy, err)
		}
		if !c2.BillingOverageInvoicingEnabled {
			t.Errorf("BillingOverageInvoicingEnabled %q: want true", truthy)
		}
	}
	t.Setenv("BILLING_OVERAGE_INVOICING_ENABLED", "false")
	c3, err := Load(false)
	if err != nil {
		t.Fatalf("Load(false): %v", err)
	}
	if c3.BillingOverageInvoicingEnabled {
		t.Error("BillingOverageInvoicingEnabled \"false\": want false")
	}
}

// TestLoadLLMProviderDefaultsAndOverrides asserts the failover config:
// LLM_PRIMARY defaults to "opencode", LLM_FALLBACK defaults empty, and env
// overrides are honored.
func TestLoadLLMProviderDefaultsAndOverrides(t *testing.T) {
	_ = os.Unsetenv("LLM_PRIMARY")
	_ = os.Unsetenv("LLM_FALLBACK")
	setValidJWTSecret(t)

	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.LLMPrimaryProvider != "opencode" {
		t.Errorf("LLMPrimaryProvider default: got %q", c.LLMPrimaryProvider)
	}
	if c.LLMFallbackProvider != "" {
		t.Errorf("LLMFallbackProvider default: got %q", c.LLMFallbackProvider)
	}

	t.Setenv("LLM_PRIMARY", "openai")
	t.Setenv("LLM_FALLBACK", "anthropic")
	c2, err := Load(false)
	if err != nil {
		t.Fatalf("Load (overrides): %v", err)
	}
	if c2.LLMPrimaryProvider != "openai" {
		t.Errorf("LLMPrimaryProvider override: got %q", c2.LLMPrimaryProvider)
	}
	if c2.LLMFallbackProvider != "anthropic" {
		t.Errorf("LLMFallbackProvider override: got %q", c2.LLMFallbackProvider)
	}
}

// TestLoadHTTPHardeningDefaults asserts the hardening env vars parse and
// default correctly.
func TestLoadHTTPHardeningDefaults(t *testing.T) {
	setValidJWTSecret(t)
	_ = os.Unsetenv("TRUST_PROXY_HEADER")
	_ = os.Unsetenv("MAX_BODY_BYTES")
	_ = os.Unsetenv("SECURITY_CSP")

	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.TrustProxyHeader != "" {
		t.Errorf("TrustProxyHeader default: want empty, got %q", c.TrustProxyHeader)
	}
	if c.MaxBodyBytes != 2<<20 {
		t.Errorf("MaxBodyBytes default: want %d, got %d", 2<<20, c.MaxBodyBytes)
	}
	if c.CSPHeader == "" {
		t.Error("CSPHeader default: want a base CSP, got empty")
	}

	t.Setenv("TRUST_PROXY_HEADER", "X-Real-IP")
	t.Setenv("MAX_BODY_BYTES", "1048576")
	t.Setenv("SECURITY_CSP", "default-src 'none'")
	c2, err := Load(false)
	if err != nil {
		t.Fatalf("Load (overrides): %v", err)
	}
	if c2.TrustProxyHeader != "X-Real-IP" {
		t.Errorf("TrustProxyHeader override: got %q", c2.TrustProxyHeader)
	}
	if c2.MaxBodyBytes != 1048576 {
		t.Errorf("MaxBodyBytes override: got %d", c2.MaxBodyBytes)
	}
	if c2.CSPHeader != "default-src 'none'" {
		t.Errorf("CSPHeader override: got %q", c2.CSPHeader)
	}

	t.Setenv("MAX_BODY_BYTES", "-5")
	if _, err := Load(false); err == nil {
		t.Fatal("Load: expected error for non-positive MAX_BODY_BYTES")
	}
}

// TestLoadDeletionReversibleHoursDefault guards the account-deletion grace
// window. 720 hours (30 days) is the product policy, stated in
// docs/api/user.md, docs/api/billing.md, the README, .env.example, and the
// account-deletion runbooks. The value can be overridden per deployment.
func TestLoadDeletionReversibleHoursDefault(t *testing.T) {
	_ = os.Unsetenv("DELETION_REVERSIBLE_HOURS")
	setValidJWTSecret(t)

	c, err := Load(false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DeletionReversibleHours != 720 {
		t.Errorf("DeletionReversibleHours default: want 720 (30 days), got %d", c.DeletionReversibleHours)
	}

	t.Setenv("DELETION_REVERSIBLE_HOURS", "48")
	c2, err := Load(false)
	if err != nil {
		t.Fatalf("Load (override): %v", err)
	}
	if c2.DeletionReversibleHours != 48 {
		t.Errorf("DeletionReversibleHours override: want 48, got %d", c2.DeletionReversibleHours)
	}
}

// TestLoadSignedURLSecretRequirement asserts that the signed-URL signing
// secret is mandatory whenever the signed-URL endpoints can be reached, i.e.
// when either the lifecycle workers or the account lifecycle API is enabled.
// Regression for the API-enabled/workers-disabled gap that allowed forging
// HMAC("", userID:jobID:exp) downloads.
func TestLoadSignedURLSecretRequirement(t *testing.T) {
	setValidJWTSecret(t)

	tests := []struct {
		name            string
		apiEnabled      bool
		workersEnabled  bool
		secret          string
		wantErr         bool
		wantErrContains string
	}{
		{
			name:            "account API enabled without secret fails",
			apiEnabled:      true,
			secret:          "",
			wantErr:         true,
			wantErrContains: "EXPORT_SIGNED_URL_SECRET",
		},
		{
			name:       "account API enabled with secret succeeds",
			apiEnabled: true,
			secret:     "signed-url-secret",
			wantErr:    false,
		},
		{
			name:            "workers enabled without secret still fails",
			workersEnabled:  true,
			secret:          "",
			wantErr:         true,
			wantErrContains: "EXPORT_SIGNED_URL_SECRET",
		},
		{
			name:    "all disabled without secret succeeds",
			secret:  "",
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api, workers := "false", "false"
			if tc.apiEnabled {
				api = "true"
			}
			if tc.workersEnabled {
				workers = "true"
			}
			t.Setenv("ACCOUNT_LIFECYCLE_API_ENABLED", api)
			t.Setenv("LIFECYCLE_WORKERS_ENABLED", workers)
			t.Setenv("EXPORT_SIGNED_URL_SECRET", tc.secret)

			c, err := Load(false)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Load: expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Fatalf("Load error %q: want it to name %q", err.Error(), tc.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: unexpected error: %v", err)
			}
			if tc.apiEnabled && !c.AccountLifecycleAPIEnabled {
				t.Error("AccountLifecycleAPIEnabled: want true")
			}
			if tc.secret != "" && c.ExportSignedURLSecret != tc.secret {
				t.Errorf("ExportSignedURLSecret: got %q, want %q", c.ExportSignedURLSecret, tc.secret)
			}
		})
	}
}
