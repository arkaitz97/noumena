package email

import (
	"strings"
	"testing"
)

// TestTokenExpiryHTMLEscapesChannelName verifies task 6.3: a
// YouTube-supplied channel name containing markup is rendered escaped, so
// the legacy Resend path cannot inject HTML into the mail body.
func TestTokenExpiryHTMLEscapesChannelName(t *testing.T) {
	got := tokenExpiryHTML("<script>alert(1)</script>", "January 1, 2026", "https://app.example/settings")
	if strings.Contains(got, "<script>") {
		t.Fatalf("channel name not escaped:\n%s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected escaped channel name, got:\n%s", got)
	}
	if !strings.Contains(got, "January 1, 2026") {
		t.Errorf("expiry missing from body:\n%s", got)
	}
}

// Settings URLs are internal but still escaped for defence in depth.
func TestTokenExpiryHTMLEscapesSettingsURL(t *testing.T) {
	got := tokenExpiryHTML("chan", "Jan 1", `https://app.example/x?a="><img src=x onerror=alert(1)>`)
	if strings.Contains(got, `"><img`) {
		t.Fatalf("settings URL not escaped:\n%s", got)
	}
}
