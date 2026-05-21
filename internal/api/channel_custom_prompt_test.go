package api

import (
	"strings"
	"testing"
)

func TestValidateCustomPrompt_AcceptedUnder500(t *testing.T) {
	prompt := strings.Repeat("a", 500)
	if err := ValidateCustomPrompt(prompt); err != nil {
		t.Fatalf("expected prompt of 500 chars to be accepted, got: %v", err)
	}
}

func TestValidateCustomPrompt_RejectedOver500(t *testing.T) {
	prompt := strings.Repeat("a", 501)
	err := ValidateCustomPrompt(prompt)
	if err == nil {
		t.Fatal("expected error for prompt over 500 chars")
	}
	ae, ok := err.(*apiError)
	if !ok || ae.code != "prompt_too_long" {
		t.Errorf("expected prompt_too_long error, got: %v", err)
	}
}

func TestValidateCustomPrompt_RejectedScriptTag(t *testing.T) {
	err := ValidateCustomPrompt("classify only positive comments <script>alert(1)</script>")
	if err == nil {
		t.Fatal("expected error for script tag")
	}
	ae, ok := err.(*apiError)
	if !ok || ae.code != "prompt_invalid" {
		t.Errorf("expected prompt_invalid error, got: %v", err)
	}
}

func TestValidateCustomPrompt_RejectedHTMLTag(t *testing.T) {
	err := ValidateCustomPrompt("classify <b>bold</b> comments")
	if err == nil {
		t.Fatal("expected error for HTML tag")
	}
	ae, ok := err.(*apiError)
	if !ok || ae.code != "prompt_invalid" {
		t.Errorf("expected prompt_invalid error, got: %v", err)
	}
}

func TestValidateCustomPrompt_EmptyAccepted(t *testing.T) {
	if err := ValidateCustomPrompt(""); err != nil {
		t.Fatalf("expected empty prompt to be accepted, got: %v", err)
	}
}

func TestValidateCustomPrompt_PlainTextAccepted(t *testing.T) {
	if err := ValidateCustomPrompt("Focus on sentiment and questions"); err != nil {
		t.Fatalf("expected plain text prompt to be accepted, got: %v", err)
	}
}
