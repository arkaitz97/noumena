package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
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
	// sessionID is a stable per-client ID sent as x-opencode-session. OpenCode
	// Go (zen/go) requires it (2026-09-06+) for request routing + token caching.
	sessionID string
}

// NewOpenCodeClient constructs an LLMClient backed by the OpenCode HTTP API.
// Empty arguments fall back to documented defaults.
func NewOpenCodeClient(apiKey, baseURL, model string) *opencodeClient {
	if baseURL == "" {
		baseURL = "https://api.opencode.local/v1"
	}
	if model == "" {
		model = "opencode/default"
	}
	return &opencodeClient{
		httpClient: &http.Client{}, // no timeout; processor ctx owns budget
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		sessionID:  uuid.NewString(),
	}
}

// newOpenCodeClientWithHTTP is the test seam. It accepts an injected
// *http.Client (typically backed by httptest.NewServer) so tests can run
// without reaching the network.
func newOpenCodeClientWithHTTP(client *http.Client, baseURL, apiKey, model string) *opencodeClient {
	if client == nil {
		client = &http.Client{} // no timeout; processor ctx owns budget
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
		sessionID:  uuid.NewString(),
	}
}

func (c *opencodeClient) ProviderName() string { return "opencode" }
func (c *opencodeClient) ModelName() string    { return c.model }

// digestOutcome shuttles (content, usage) through the digest retry loop.
type digestOutcome struct {
	content string
	usage   TokenUsage
}

// GenerateDigestRaw sends a digest prompt to the OpenCode chat completions
// endpoint. Returns the raw model content string and the provider-reported
// token usage (zero counts when the gateway omits the usage object).
func (c *opencodeClient) GenerateDigestRaw(ctx context.Context, prompt string) (string, TokenUsage, error) {
	reqBody := opencodeChatRequest{
		Model: c.model,
		Messages: []opencodeChatMessage{
			{Role: "system", Content: "You are a creator-audience analyst. Return ONLY a single JSON object."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.4,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", TokenUsage{}, fmt.Errorf("opencode: marshal digest request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	out, err := retryTransient(ctx, "opencode", "opencode: digest", func(attemptCtx context.Context) (digestOutcome, time.Duration, bool, error) {
		return c.doOnceDigest(attemptCtx, endpoint, bodyBytes)
	})
	return out.content, out.usage, err
}

// doOnceDigest performs a single HTTP attempt for digest generation.
// Returns (outcome, retryAfter, retryable, err).
func (c *opencodeClient) doOnceDigest(ctx context.Context, endpoint string, body []byte) (digestOutcome, time.Duration, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return digestOutcome{}, 0, false, fmt.Errorf("opencode: build digest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-opencode-session", c.sessionID)
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return digestOutcome{}, 0, true, fmt.Errorf("opencode: digest transport: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		retryAfter := parseRetryAfterSeconds(resp.Header.Get("Retry-After"))
		_, _ = io.Copy(io.Discard, resp.Body)
		return digestOutcome{}, retryAfter, true, fmt.Errorf("opencode: digest HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return digestOutcome{}, 0, false, fmt.Errorf("opencode: digest HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var parsed opencodeChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return digestOutcome{}, 0, false, fmt.Errorf("opencode: digest decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return digestOutcome{}, 0, false, fmt.Errorf("opencode: digest empty response choices")
	}
	return digestOutcome{content: parsed.Choices[0].Message.Content, usage: parsed.Usage.tokenUsage()}, 0, false, nil
}

// chatCompletionRequest mirrors the small subset of the OpenAI-compatible
// /v1/chat/completions request body that we actually populate.
type opencodeChatRequest struct {
	Model       string                `json:"model"`
	Messages    []opencodeChatMessage `json:"messages"`
	Temperature float64               `json:"temperature"`
}

type opencodeChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// opencodeUsage is the OpenAI-compatible usage object. Absent/null in
// responses decodes to zero counts (never guessed, per llm-cost-analytics).
type opencodeUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

func (u opencodeUsage) tokenUsage() TokenUsage {
	return TokenUsage{Input: u.PromptTokens, Output: u.CompletionTokens}
}

type opencodeChatResponse struct {
	Choices []struct {
		Message opencodeChatMessage `json:"message"`
	} `json:"choices"`
	Usage opencodeUsage `json:"usage"`
}

// ClassifyBatch builds the classification prompt, POSTs it to the OpenCode
// chat-completions endpoint, and returns the parsed []LLMResult. Retries are
// handled by retryTransient (shared with OpenAI/Anthropic clients).
func (c *opencodeClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, []int, TokenUsage, error) {
	if len(comments) == 0 {
		return nil, nil, TokenUsage{}, nil
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
		return nil, nil, TokenUsage{}, fmt.Errorf("opencode: marshal request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	out, err := retryTransient(ctx, "opencode", "opencode", func(attemptCtx context.Context) (classifyOutcome, time.Duration, bool, error) {
		return c.doOnce(attemptCtx, endpoint, bodyBytes, len(comments))
	})
	return out.results, out.missingIndexes, out.usage, err
}

// doOnce performs a single HTTP attempt. It returns:
//   - (outcome, 0, false, nil) on success (outcome may carry missing indexes)
//   - (zero, retryAfter, true, err) on 429/5xx (caller should retry)
//   - (zero, 0, false, err) on non-retryable error (caller should abort)
func (c *opencodeClient) doOnce(ctx context.Context, endpoint string, body []byte, expected int) (classifyOutcome, time.Duration, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return classifyOutcome{}, 0, false, fmt.Errorf("opencode: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-opencode-session", c.sessionID)
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Transport-level errors (DNS, connection, TLS, timeout) are treated
		// as retryable; the failover wrapper still gets the same signal if
		// they persist past our retry budget.
		return classifyOutcome{}, 0, true, fmt.Errorf("opencode: transport: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		retryAfter := parseRetryAfterSeconds(resp.Header.Get("Retry-After"))
		// Drain so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return classifyOutcome{}, retryAfter, true, fmt.Errorf("opencode: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		// 4xx other than 429 → non-retryable (auth, malformed request, etc.).
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return classifyOutcome{}, 0, false, terminal(fmt.Errorf("opencode: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet))))
	}

	var parsed opencodeChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return classifyOutcome{}, 0, false, terminal(fmt.Errorf("opencode: decode response: %w", err))
	}
	if len(parsed.Choices) == 0 {
		return classifyOutcome{}, 0, false, terminal(fmt.Errorf("opencode: empty response choices"))
	}
	results, missingIndexes, perr := ParseClassificationResponse(parsed.Choices[0].Message.Content, expected)
	if perr != nil {
		return classifyOutcome{}, 0, false, terminal(fmt.Errorf("opencode: parse: %w", perr))
	}
	return classifyOutcome{results: results, missingIndexes: missingIndexes, usage: parsed.Usage.tokenUsage()}, 0, false, nil
}
