package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// WebhookAlerter posts a JSON body to a configured URL on every dead-letter
// event. It logs and swallows errors so a failing alert pipeline never
// blocks the worker.
type WebhookAlerter struct {
	URL    string
	HTTP   *http.Client
	Source string // optional: identifier included in payload (e.g. instance id)
}

// NewWebhookAlerter constructs an alerter that posts to url. If url is
// empty, callers should fall back to LogAlerter.
func NewWebhookAlerter(url string) *WebhookAlerter {
	return &WebhookAlerter{
		URL:  url,
		HTTP: &http.Client{Timeout: 5 * time.Second},
	}
}

func (a *WebhookAlerter) Alert(ctx context.Context, platformChannelID, jobID, errMsg string) {
	if a == nil || a.URL == "" {
		LogAlerter{}.Alert(ctx, platformChannelID, jobID, errMsg)
		return
	}
	payload := map[string]string{
		"event":               "ingestion.dead_letter",
		"platform_channel_id": platformChannelID,
		"job_id":              jobID,
		"error":               errMsg,
		"source":              a.Source,
		"at":                  time.Now().UTC().Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.URL, bytes.NewReader(body))
	if err != nil {
		log.Printf("alerter: build request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	hc := a.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		log.Printf("alerter: webhook POST failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("alerter: webhook returned HTTP %d for channel=%s job=%s", resp.StatusCode, platformChannelID, jobID)
	}
}

// PickAlerter returns a WebhookAlerter if alertURL is non-empty, otherwise
// the default LogAlerter.
func PickAlerter(alertURL string) DeadLetterAlerter {
	if alertURL == "" {
		return LogAlerter{}
	}
	return NewWebhookAlerter(alertURL)
}
