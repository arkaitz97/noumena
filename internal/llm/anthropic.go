package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// anthropicAPIKeyHeader is the Anthropic auth header; anthropicVersionHeader
// is the mandatory API version header on every Messages API request.
const (
	anthropicAPIKeyHeader     = "x-api-key"
	anthropicVersionHeader    = "anthropic-version"
	anthropicVersion          = "2023-06-01"
	anthropicDefaultBaseURL   = "https://api.anthropic.com"
	anthropicDefaultModel     = "claude-sonnet-4-20250514"
	anthropicMaxTokensDefault = 4096
)

// anthropicMessagesRequest is the subset of the Anthropic Messages API
// request body we populate. system travels top-level; the classification
// user prompt is the sole content message.
type anthropicMessagesRequest struct {
	Model       string                 `json:"model"`
	MaxTokens   int                    `json:"max_tokens"`
	Temperature float64                `json:"temperature"`
	System      string                 `json:"system"`
	Messages    []anthropicTextMessage `json:"messages"`
}

type anthropicTextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicMessagesResponse is the minimal Messages API envelope we parse:
// the assistant's reply is a list of content blocks, of which we consume the
// first text block.
type anthropicMessagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

// anthropicClient is an optional Anthropic Messages API LLMClient with the
// same classification contract as opencodeClient: one request per batch
// (prompt built by BuildClassificationPrompt), transient retry/backoff, and
// shared ParseClassificationResponse parsing. It is only ever constructed
// when a deployment configures LLM_PRIMARY/LLM_FALLBACK="anthropic" with an
// ANTHROPIC_API_KEY; it is the provider of last resort for failover.
type anthropicClient struct {
	httpClient *http.Client
	baseURL    string // scheme+host only; "/v1/messages" is appended
	apiKey     string
	model      string
	maxTokens  int
}

// NewAnthropicClient constructs an LLMClient backed by the Anthropic
// Messages API. baseURL is scheme+host (default https://api.anthropic.com);
// the client POSTs to {baseURL}/v1/messages. Empty model falls back to the
// package default; apiKey is the ANTHROPIC_API_KEY value and is only sent
// when non-empty.
func NewAnthropicClient(apiKey, baseURL, model string) LLMClient {
	return newAnthropicClientWithHTTP(nil, baseURL, apiKey, model)
}

// newAnthropicClientWithHTTP is the test seam: it accepts an injected
// *http.Client (typically backed by httptest.NewServer).
func newAnthropicClientWithHTTP(client *http.Client, baseURL, apiKey, model string) *anthropicClient {
	if client == nil {
		client = &http.Client{} // no timeout; processor ctx owns budget
	}
	if baseURL == "" {
		baseURL = anthropicDefaultBaseURL
	}
	if model == "" {
		model = anthropicDefaultModel
	}
	return &anthropicClient{
		httpClient: client,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		maxTokens:  anthropicMaxTokensDefault,
	}
}

func (c *anthropicClient) ProviderName() string { return "anthropic" }
func (c *anthropicClient) ModelName() string    { return c.model }

// ClassifyBatch mirrors opencodeClient.ClassifyBatch: build the shared
// classification prompt, POST one Messages API request, retry with the
// shared transient schedule, parse via ParseClassificationResponse.
func (c *anthropicClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, []int, TokenUsage, error) {
	if len(comments) == 0 {
		return nil, nil, TokenUsage{}, nil
	}

	system, user := BuildClassificationPrompt(comments)
	reqBody := anthropicMessagesRequest{
		Model:       c.model,
		MaxTokens:   c.maxTokens,
		Temperature: 0,
		System:      system,
		Messages: []anthropicTextMessage{
			{Role: "user", Content: user},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, TokenUsage{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	endpoint := c.baseURL + "/v1/messages"
	out, err := retryTransient(ctx, "anthropic", "anthropic: all retries exhausted", func(attemptCtx context.Context) (classifyOutcome, time.Duration, bool, error) {
		return httpClassifyOnce(attemptCtx, c.httpClient, endpoint, body, "anthropic",
			func(h http.Header) {
				h.Set(anthropicVersionHeader, anthropicVersion)
				if c.apiKey != "" {
					h.Set(anthropicAPIKeyHeader, c.apiKey)
				}
			},
			func(r io.Reader) (classifyOutcome, error) {
				var parsed anthropicMessagesResponse
				if err := json.NewDecoder(r).Decode(&parsed); err != nil {
					return classifyOutcome{}, fmt.Errorf("anthropic: decode response: %w", err)
				}
				var text string
				for _, block := range parsed.Content {
					if block.Type == "text" && block.Text != "" {
						text = block.Text
						break
					}
				}
				if text == "" {
					return classifyOutcome{}, fmt.Errorf("anthropic: empty response content")
				}
				results, missingIndexes, perr := ParseClassificationResponse(text, len(comments))
				if perr != nil {
					return classifyOutcome{}, terminal(fmt.Errorf("anthropic: parse: %w", perr))
				}
				return classifyOutcome{
					results:        results,
					missingIndexes: missingIndexes,
					usage:          TokenUsage{Input: parsed.Usage.InputTokens, Output: parsed.Usage.OutputTokens},
				}, nil
			})
	})
	return out.results, out.missingIndexes, out.usage, err
}
