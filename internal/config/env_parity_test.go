package config

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestEnvExampleParity guards the README claim that every variable read by
// internal/config/config.go is listed in .env.example. It only checks
// presence of keys, not values (the design trade-off chosen to keep the
// test stable).
func TestEnvExampleParity(t *testing.T) {
	src, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("read config.go: %v", err)
	}
	keyRe := regexp.MustCompile(`(?:Getenv|LookupEnv)\("([A-Z0-9_]+)"\)`)
	seen := map[string]bool{}
	for _, m := range keyRe.FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	if len(seen) == 0 {
		t.Fatal("no env keys found in config.go; test may be stale")
	}

	example, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(string(example), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if eq := strings.Index(line, "="); eq > 0 {
			listed[line[:eq]] = true
		}
	}

	var missing []string
	for key := range seen {
		if !listed[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Errorf("env keys read by config.go but missing from .env.example: %s", strings.Join(missing, ", "))
	}
}
