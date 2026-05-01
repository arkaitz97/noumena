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

type ClassificationResult struct {
	Category       string  `json:"category"`
	Topic          string  `json:"topic"`
	SentimentScore float64 `json:"sentiment_score"`
	Confidence     float64 `json:"confidence"`
	Error          string  `json:"error,omitempty"`
}

type bulkResponse struct {
	Results []ClassificationResult `json:"results"`
}

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

	// Add exponential backoff for resilience (PRD: retry logic)
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

func (c *Client) ClassifyBatch(ctx context.Context, comments []string) ([]ClassificationResult, error) {
	if len(comments) == 0 {
		return nil, nil
	}

	commentsJSON, err := json.Marshal(comments)
	if err != nil {
		return nil, err
	}

	reqBody := openaiReq{
		Model: "gpt-4o",
		Messages: []openaiReqMsg{
			{Role: "system", Content: BatchClassificationPrompt},
			{Role: "user", Content: string(commentsJSON)},
		},
		ResponseFormat: map[string]any{"type": "json_object"},
		Temperature:    0.3,
	}

	respBytes, err := c.doRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	var parsed bulkResponse
	if err := c.parseJSONResponse(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse bulk classification: %w", err)
	}

	if len(parsed.Results) != len(comments) {
		return nil, fmt.Errorf("expected %d results, got %d", len(comments), len(parsed.Results))
	}

	return parsed.Results, nil
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
