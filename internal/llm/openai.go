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

// openAIChatRequest / openAIChatMessage / openAIChatResponse are aliases of
// the shared OpenAI-compatible chat-completions wire schema the OpenCode
// client already speaks (internal/llm/opencode.go). Aliasing keeps the
// OpenAI client's transport self-documenting without duplicating structs.
type openAIChatRequest = opencodeChatRequest
type openAIChatMessage = opencodeChatMessage
type openAIChatResponse = opencodeChatResponse

// openaiDefaultBaseURL is the OpenAI API root; the client POSTs to
// {baseURL}/chat/completions.
const openaiDefaultBaseURL = "https://api.openai.com/v1"

// openaiDefaultModel is used when the constructor receives an empty model.
// The API key is read by the caller from OPENAI_API_KEY (os.Getenv), never
// from config.
const openaiDefaultModel = "gpt-4o-mini"

// openaiClient is an OpenAI Chat Completions LLMClient with the same
// contract as opencodeClient: the caller hands it a batch of PromptComments
// (already grouped by channel at the batch size in processor.go) and it
// sends the whole batch in ONE request built by BuildClassificationPrompt,
// retries transient 429/5xx with 1s/2s/4s backoff (honoring Retry-After),
// and parses the response with the shared ParseClassificationResponse.
//
// It is the LLM_PRIMARY/LLM_FALLBACK="openai" provider. Model selection is
// deployment config (constructor argument); temperature is fixed at 0 for
// deterministic classification.
type openaiClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

// NewOpenAIClient constructs an LLMClient backed by the OpenAI Chat
// Completions API. Empty baseURL/model fall back to documented defaults.
// apiKey is the OPENAI_API_KEY value; it is only sent when non-empty.
func NewOpenAIClient(apiKey, baseURL, model string) LLMClient {
	return newOpenAIClientWithHTTP(nil, baseURL, apiKey, model)
}

// newOpenAIClientWithHTTP is the test seam: it accepts an injected
// *http.Client (typically backed by httptest.NewServer).
func newOpenAIClientWithHTTP(client *http.Client, baseURL, apiKey, model string) *openaiClient {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if baseURL == "" {
		baseURL = openaiDefaultBaseURL
	}
	if model == "" {
		model = openaiDefaultModel
	}
	return &openaiClient{
		httpClient: client,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}
}

func (c *openaiClient) ProviderName() string { return "openai" }
func (c *openaiClient) ModelName() string    { return c.model }

// ClassifyBatch mirrors opencodeClient.ClassifyBatch: build the shared
// classification prompt, POST one request to /chat/completions, retry with
// the shared transient schedule, parse via ParseClassificationResponse.
func (c *openaiClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error) {
	if len(comments) == 0 {
		return nil, nil
	}

	system, user := BuildClassificationPrompt(comments)
	reqBody := openAIChatRequest{
		Model: c.model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	return retryTransient(ctx, "openai: all retries exhausted", func() ([]LLMResult, time.Duration, bool, error) {
		return httpClassifyOnce(ctx, c.httpClient, endpoint, body, "openai",
			func(h http.Header) {
				if c.apiKey != "" {
					h.Set("Authorization", "Bearer "+c.apiKey)
				}
			},
			func(r io.Reader) ([]LLMResult, error) {
				var parsed openAIChatResponse
				if err := json.NewDecoder(r).Decode(&parsed); err != nil {
					return nil, fmt.Errorf("openai: decode response: %w", err)
				}
				if len(parsed.Choices) == 0 {
					return nil, fmt.Errorf("openai: empty response choices")
				}
				results, perr := ParseClassificationResponse(parsed.Choices[0].Message.Content, len(comments))
				if perr != nil {
					return nil, fmt.Errorf("openai: parse: %w", perr)
				}
				return results, nil
			})
	})
}
