package llm

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// openaiClient is the primary classification provider. It calls
// chat.completions against the configured model (default gpt-4o) and applies
// exponential backoff (1s, 2s, 4s) on 429/5xx for up to three transport-level
// retries before surfacing the error to the failover wrapper.
type openaiClient struct {
	api   openaiChatAPI
	model string
}

// openaiChatAPI is the minimal surface area of the third-party SDK we depend
// on. It exists so tests can inject a fake.
type openaiChatAPI interface {
	CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// NewOpenAIClient constructs an LLMClient backed by github.com/sashabaranov/go-openai.
// `apiKey` is the OpenAI API key; `model` defaults to "gpt-4o" when empty.
func NewOpenAIClient(apiKey, model string) LLMClient {
	if model == "" {
		model = "gpt-4o"
	}
	cfg := openai.DefaultConfig(apiKey)
	return &openaiClient{
		api:   openai.NewClientWithConfig(cfg),
		model: model,
	}
}

// newOpenAIClientWithAPI is for tests.
func newOpenAIClientWithAPI(api openaiChatAPI, model string) *openaiClient {
	if model == "" {
		model = "gpt-4o"
	}
	return &openaiClient{api: api, model: model}
}

func (c *openaiClient) ProviderName() string { return "openai" }
func (c *openaiClient) ModelName() string    { return c.model }

func (c *openaiClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error) {
	if len(comments) == 0 {
		return nil, nil
	}

	system, user := BuildClassificationPrompt(comments)
	req := openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: system},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
		Temperature: 0.2,
	}

	backoff := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error
	for attempt := 0; attempt < len(backoff)+1; attempt++ {
		resp, err := c.api.CreateChatCompletion(ctx, req)
		if err == nil {
			if len(resp.Choices) == 0 {
				return nil, fmt.Errorf("openai: empty response choices")
			}
			content := resp.Choices[0].Message.Content
			results, perr := ParseClassificationResponse(content, len(comments))
			if perr != nil {
				return nil, fmt.Errorf("openai: parse: %w", perr)
			}
			return results, nil
		}
		lastErr = err
		if !isRetryableOpenAIErr(err) || attempt == len(backoff) {
			break
		}
		wait := backoff[attempt]
		if ra := retryAfterFromOpenAIErr(err); ra > 0 {
			wait = ra
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("openai: all retries exhausted: %w", lastErr)
}

// isRetryableOpenAIErr returns true for 429 and 5xx HTTP errors surfaced by
// the SDK's APIError type.
func isRetryableOpenAIErr(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		if apiErr.HTTPStatusCode == 429 || apiErr.HTTPStatusCode >= 500 {
			return true
		}
	}
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		if reqErr.HTTPStatusCode == 429 || reqErr.HTTPStatusCode >= 500 {
			return true
		}
	}
	return false
}

// retryAfterFromOpenAIErr inspects the SDK error for a Retry-After hint.
// The SDK does not expose response headers cleanly; we attempt a best-effort
// parse from APIError.Message when present.
func retryAfterFromOpenAIErr(err error) time.Duration {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		// Some upstream payloads include "retry after N seconds" — try a
		// loose parse so we honor server hints when offered.
		if apiErr.Message != "" {
			for _, tok := range []string{"retry after ", "Retry-After: "} {
				idx := indexCI(apiErr.Message, tok)
				if idx >= 0 {
					rest := apiErr.Message[idx+len(tok):]
					var num string
					for i := 0; i < len(rest); i++ {
						if rest[i] >= '0' && rest[i] <= '9' {
							num += string(rest[i])
							continue
						}
						break
					}
					if n, perr := strconv.Atoi(num); perr == nil && n > 0 {
						return time.Duration(n) * time.Second
					}
				}
			}
		}
	}
	return 0
}

func indexCI(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	if len(s) < len(sub) {
		return -1
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			a := s[i+j]
			b := sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
