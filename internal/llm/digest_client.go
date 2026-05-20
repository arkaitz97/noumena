package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the legacy direct-HTTP OpenAI client retained for the weekly
// digest worker. New per-comment classification goes through the LLMClient
// interface in client.go (with concrete openaiClient/anthropicClient
// implementations) rather than this type.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		baseURL:    "https://api.openai.com/v1/chat/completions",
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// DigestResult is the shape of the weekly insight digest produced by the
// digest worker. Unrelated to per-comment classification.
type DigestResult struct {
	TopIdeas       []string `json:"top_ideas"`
	TopQuestions   []string `json:"top_questions"`
	SentimentTrend string   `json:"sentiment_trend"`
	HealthScore    int      `json:"health_score"`
}

type openaiReqMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiReq struct {
	Model          string         `json:"model"`
	Messages       []openaiReqMsg `json:"messages"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
	Temperature    float64        `json:"temperature"`
}

func (c *Client) doRequest(ctx context.Context, reqBody openaiReq) ([]byte, error) {
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	var resp *http.Response
	backoff := 1 * time.Second
	for i := 0; i < 3; i++ {
		resp, err = c.httpClient.Do(req)
		if err == nil {
			if resp.StatusCode == 429 || resp.StatusCode >= 500 {
				resp.Body.Close()
				err = fmt.Errorf("openai error: %d", resp.StatusCode)
			} else {
				break
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}

	if err != nil {
		return nil, fmt.Errorf("openai request failed after retries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) parseJSONResponse(body []byte, v any) error {
	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return err
	}

	if len(openaiResp.Choices) == 0 {
		return fmt.Errorf("no response choices returned")
	}

	content := openaiResp.Choices[0].Message.Content
	return json.Unmarshal([]byte(content), v)
}

func (c *Client) GenerateDigest(ctx context.Context, commentsJSON string) (*DigestResult, error) {
	reqBody := openaiReq{
		Model: "gpt-4o",
		Messages: []openaiReqMsg{
			{Role: "system", Content: DigestPrompt},
			{Role: "user", Content: commentsJSON},
		},
		ResponseFormat: map[string]any{"type": "json_object"},
		Temperature:    0.7,
	}

	respBytes, err := c.doRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	var parsed DigestResult
	if err := c.parseJSONResponse(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse digest: %w", err)
	}

	return &parsed, nil
}

// GenerateDigestRaw is the new weekly-digest entry point: instead of asking
// the legacy DigestPrompt for a small fixed shape, the caller supplies the
// fully-built prompt (BuildDigestPrompt) and we return the model's raw
// content string for the caller to parse + validate. This decouples the
// HTTP transport from the digest content schema, which the worker
// (CallDigestLLM) is responsible for.
func (c *Client) GenerateDigestRaw(ctx context.Context, prompt string) (string, error) {
	reqBody := openaiReq{
		Model: "gpt-4o",
		Messages: []openaiReqMsg{
			{Role: "system", Content: "You are a creator-audience analyst. Return ONLY a single JSON object."},
			{Role: "user", Content: prompt},
		},
		ResponseFormat: map[string]any{"type": "json_object"},
		Temperature:    0.4,
	}
	respBytes, err := c.doRequest(ctx, reqBody)
	if err != nil {
		return "", err
	}
	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &openaiResp); err != nil {
		return "", err
	}
	if len(openaiResp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}
	return openaiResp.Choices[0].Message.Content, nil
}
