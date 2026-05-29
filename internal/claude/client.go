package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const apiURL = "https://api.anthropic.com/v1/messages"

type Client struct {
	http   *http.Client
	apiKey string
	model  string
	logger *slog.Logger
}

func NewClient(apiKey, model string, logger *slog.Logger) *Client {
	return &Client{
		http:   &http.Client{Timeout: 120 * time.Second},
		apiKey: apiKey,
		model:  model,
		logger: logger,
	}
}

type Result struct {
	Text         string
	Model        string
	InputTokens  int
	OutputTokens int
}

func (c *Client) SendMessage(ctx context.Context, system, userMsg string) (*Result, error) {
	reqBody := Request{
		Model:     c.model,
		MaxTokens: 4096,
		System:    system,
		Messages: []Message{
			{Role: "user", Content: userMsg},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	c.logger.Info("sending request to Claude", "model", c.model)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Claude API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		json.Unmarshal(respBody, &errResp)
		return nil, fmt.Errorf("Claude API error (%d): %s - %s", resp.StatusCode, errResp.Error.Type, errResp.Error.Message)
	}

	var apiResp Response
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from Claude")
	}

	c.logger.Info("Claude response received",
		"input_tokens", apiResp.Usage.InputTokens,
		"output_tokens", apiResp.Usage.OutputTokens)

	return &Result{
		Text:         apiResp.Content[0].Text,
		Model:        apiResp.Model,
		InputTokens:  apiResp.Usage.InputTokens,
		OutputTokens: apiResp.Usage.OutputTokens,
	}, nil
}
