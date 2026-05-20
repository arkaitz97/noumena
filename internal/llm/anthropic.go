package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicClient is the fallback classification provider. It calls
// messages.create against the configured model (default
// claude-3-5-sonnet-latest) and produces the same []LLMResult shape as the
// OpenAI client by routing through the shared parser.
type anthropicClient struct {
	api   anthropicMessagesAPI
	model string
}

// anthropicMessagesAPI captures the minimal SDK surface we use; tests inject
// a fake.
type anthropicMessagesAPI interface {
	New(ctx context.Context, body anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

// NewAnthropicClient constructs an LLMClient backed by the official
// anthropic-sdk-go client. `apiKey` is the Anthropic key; `model` defaults to
// "claude-3-5-sonnet-latest" when empty.
func NewAnthropicClient(apiKey, model string) LLMClient {
	if model == "" {
		model = "claude-3-5-sonnet-latest"
	}
	cli := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &anthropicClient{
		api:   &cli.Messages,
		model: model,
	}
}

// newAnthropicClientWithAPI is for tests.
func newAnthropicClientWithAPI(api anthropicMessagesAPI, model string) *anthropicClient {
	if model == "" {
		model = "claude-3-5-sonnet-latest"
	}
	return &anthropicClient{api: api, model: model}
}

func (c *anthropicClient) ProviderName() string { return "anthropic" }
func (c *anthropicClient) ModelName() string    { return c.model }

func (c *anthropicClient) ClassifyBatch(ctx context.Context, comments []PromptComment) ([]LLMResult, error) {
	if len(comments) == 0 {
		return nil, nil
	}

	system, user := BuildClassificationPrompt(comments)
	params := anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: system, Type: "text"},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
		Temperature: anthropic.Float(0.2),
	}

	backoff := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error
	for attempt := 0; attempt < len(backoff)+1; attempt++ {
		msg, err := c.api.New(ctx, params)
		if err == nil {
			content := extractAnthropicText(msg)
			if content == "" {
				return nil, fmt.Errorf("anthropic: empty response content")
			}
			results, perr := ParseClassificationResponse(content, len(comments))
			if perr != nil {
				return nil, fmt.Errorf("anthropic: parse: %w", perr)
			}
			return results, nil
		}
		lastErr = err
		if !isRetryableAnthropicErr(err) || attempt == len(backoff) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff[attempt]):
		}
	}
	return nil, fmt.Errorf("anthropic: all retries exhausted: %w", lastErr)
}

func extractAnthropicText(msg *anthropic.Message) string {
	if msg == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range msg.Content {
		// Each block is a ContentBlockUnion; AsText() returns the TextBlock
		// variant (zero-value when not text).
		if text := block.AsText().Text; text != "" {
			b.WriteString(text)
		}
	}
	return b.String()
}

// isRetryableAnthropicErr returns true for 429 / 5xx errors surfaced by the
// SDK's anthropic.Error type.
func isRetryableAnthropicErr(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == 429 || apiErr.StatusCode >= 500 {
			return true
		}
	}
	return false
}
