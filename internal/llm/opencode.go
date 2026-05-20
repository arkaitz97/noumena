package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// opencodeClient is a hand-rolled OpenAI-compatible client targeted at the
// internal OpenCode inference gateway. It deliberately does NOT depend on
// github.com/sashabaranov/go-openai so we can evolve auth/headers/body shape
// independently of the OpenAI SDK.
//
// Behaviorally it MUST be a drop-in replacement for the OpenAI primary client
// at the LLMClient seam: same retry/backoff (1s/2s/4s on 429+5xx), same
// JSON classification contract, same parser. See
// openspec/changes/use-opencode-as-ai-provider/specs/llm-provider-failover/spec.md.
type opencodeClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

// NewOpenCodeClient constructs an LLMClient backed by the OpenCode HTTP API.
// Empty arguments fall back to documented defaults.
func NewOpenCodeClient(apiKey, baseURL, model string) LLMClient {
	if baseURL == "" {
		baseURL = "https://api.opencode.local/v1"
	}
	if model == "" {
		model = "opencode/default"
	}
	return &opencodeClient{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}
}

// newOpenCodeClientWithHTTP is the test seam. It accepts an injected
// *http.Client (typically backed by httptest.NewServer) so tests can run
// without reaching the network.
func newOpenCodeClientWithHTTP(client *http.Client, baseURL, apiKey, model string) *opencodeClient {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://api.opencode.local/v1"
	}
	if model == "" {
		model = "opencode/default"
	}
	return &opencodeClient{
		httpClient: client,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}
}

func (c *opencodeClient) ProviderName() string { return "opencode" }
func (c *opencodeClient) ModelName() string    { return c.model }

// chatCompletionRequest mirrors the small subset of the OpenAI-compatible
// /v1/chat/completions request body that we actually populate.
type opencodeChatRequest struct {
	Model       string                  `json:"model"`
	Messages    []opencodeChatMessage   `json:"messages"`
	Temperature float64                 `json:"temperature"`
}

type opencodeChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type opencodeChatResponse struct {
	Choices []struct {
		Message opencodeChatMessage `json:"message"`
	} `json:"choices"`
}

// ClassifyBatch builds the classification prompt, POSTs it to the OpenCode
// chat-completions endpoint, retries up to three times with 1s/2s/4s backoff
// on transient HTTP 429 / 5xx (honoring Retry-After in seconds), and returns
// the parsed []LLMResult. The response parser is shared with the OpenAI
// client.
func (c *opencodeClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error) {
	if len(comments) == 0 {
		return nil, nil
	}

	system, user := BuildClassificationPrompt(comments)
	reqBody := opencodeChatRequest{
		Model: c.model,
		Messages: []opencodeChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.2,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("opencode: marshal request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	backoff := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error
	for attempt := 0; attempt < len(backoff)+1; attempt++ {
		results, retryAfter, retryable, err := c.doOnce(ctx, endpoint, bodyBytes, len(comments))
		if err == nil {
			return results, nil
		}
		lastErr = err
		if !retryable || attempt == len(backoff) {
			break
		}
		wait := backoff[attempt]
		if retryAfter > 0 {
			wait = retryAfter
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("opencode: all retries exhausted: %w", lastErr)
}

// doOnce performs a single HTTP attempt. It returns:
//   - results+nil err on success
//   - (nil, retryAfter, true, err) on 429/5xx (caller should retry)
//   - (nil, 0, false, err) on non-retryable error (caller should abort)
func (c *opencodeClient) doOnce(ctx context.Context, endpoint string, body []byte, expected int) ([]LLMResult, time.Duration, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, false, fmt.Errorf("opencode: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Transport-level errors (DNS, connection, TLS, timeout) are treated
		// as retryable; the failover wrapper still gets the same signal if
		// they persist past our retry budget.
		return nil, 0, true, fmt.Errorf("opencode: transport: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		retryAfter := parseRetryAfterSeconds(resp.Header.Get("Retry-After"))
		// Drain so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, retryAfter, true, fmt.Errorf("opencode: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		// 4xx other than 429 → non-retryable (auth, malformed request, etc.).
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, 0, false, fmt.Errorf("opencode: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var parsed opencodeChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, 0, false, fmt.Errorf("opencode: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, 0, false, fmt.Errorf("opencode: empty response choices")
	}
	results, perr := ParseClassificationResponse(parsed.Choices[0].Message.Content, expected)
	if perr != nil {
		return nil, 0, false, fmt.Errorf("opencode: parse: %w", perr)
	}
	return results, 0, false, nil
}

// parseRetryAfterSeconds returns the Retry-After value as a duration when the
// header is a positive integer count of seconds. HTTP-date Retry-After values
// are not honored (rare for this endpoint); callers fall back to the static
// backoff schedule.
func parseRetryAfterSeconds(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if n, err := strconv.Atoi(h); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}
