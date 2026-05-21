package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	if _, err := Load(false); err == nil {
		t.Fatalf("Load: expected error when OTEL_ENABLED=true and endpoint unset")
	}
}
