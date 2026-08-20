package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/djdembeck/annalist/internal/config"
)

// Client talks to an OpenAI-compatible chat completions endpoint.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// ChatRequest is a single chat completion request. BaseURL and APIKey, when
// non-empty, override the client's configured endpoint for this call.
type ChatRequest struct {
	Model       string
	System      string
	User        string
	Temperature float64
	MaxTokens   int
	BaseURL     string
	APIKey      string
}

// NormalizeBaseURL strips a trailing slash and any trailing /v1 so the client
// can append its own /v1/... path without doubling the segment.
func NormalizeBaseURL(u string) string {
	u = strings.TrimRight(u, "/")
	if strings.HasSuffix(u, "/v1") {
		u = strings.TrimSuffix(u, "/v1")
	}
	return u
}

// New builds a Client from configuration. The HTTP client carries the
// configured per-request timeout.
func New(cfg config.LLMConfig) *Client {
	return &Client{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		HTTP:    &http.Client{Timeout: time.Duration(cfg.TimeoutS) * time.Second},
	}
}

// chatRequest is the wire payload sent to the endpoint.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Chat performs a chat completion and returns the assistant's content.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (string, error) {
	base := NormalizeBaseURL(req.BaseURL)
	if base == "" {
		base = NormalizeBaseURL(c.BaseURL)
	}
	apiKey := req.APIKey
	if apiKey == "" {
		apiKey = c.APIKey
	}

	payload := chatRequest{
		Model: req.Model,
		Messages: []chatMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("llm: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("llm: decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("llm: response contained no choices")
	}
	content := strings.TrimSpace(result.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("llm: response message content is empty")
	}
	return content, nil
}

// ListModels fetches the model ids the endpoint advertises via /v1/models,
// preserving the server's ordering. baseURL and apiKey are taken verbatim from
// the caller (the HTTP layer resolves the effective endpoint) rather than the
// client's configured values.
func (c *Client) ListModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	base := NormalizeBaseURL(baseURL)
	if base == "" {
		return nil, errors.New("llm: base url not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("llm: build request: %w", err)
	}
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("llm: models endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("llm: decode response: %w", err)
	}
	ids := make([]string, 0, len(result.Data))
	for _, d := range result.Data {
		ids = append(ids, d.ID)
	}
	return ids, nil
}
