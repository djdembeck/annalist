package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/djdembeck/annalist/internal/config"
)

// Client talks to an OpenAI-compatible chat completions endpoint.
//
// Transient upstream conditions (429, 408, 5xx and transport failures) are
// retried a bounded number of times; other 4xx responses fail immediately.
// The pause hook exists so tests do not wait on real backoff.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	pause   func(context.Context, time.Duration) bool
}

// ChatRequest is a single chat completion request. BaseURL and APIKey, when
// non-empty, override the client's configured endpoint for this call.
type ChatRequest struct {
	Model         string
	System        string
	User          string
	Temperature   float64
	MaxTokens     int
	ThinkingLevel string
	BaseURL       string
	APIKey        string
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

// thinkingEffort translates a resolved thinking level to the reasoning_effort
// wire value. "" sends nothing (field omitted) so the endpoint applies its
// own default; "off" sends "none" — omitting the field instead would leave
// the model's default behavior in force, and for thinking models that
// default is extended thinking, whose reasoning tokens consume the whole
// max_tokens budget and come back with empty content.
func thinkingEffort(level string) string {
	if level == "off" {
		return "none"
	}
	return level
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
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	Temperature     float64       `json:"temperature"`
	MaxTokens       int           `json:"max_tokens"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
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
		Temperature:     req.Temperature,
		MaxTokens:       req.MaxTokens,
		ReasoningEffort: thinkingEffort(req.ThinkingLevel),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= chatAttempts; attempt++ {
		content, retryAfter, retryable, err := c.chatOnce(ctx, base, apiKey, body)
		if err == nil {
			return content, nil
		}
		lastErr = err
		if !retryable || attempt == chatAttempts || ctx.Err() != nil {
			return "", err
		}
		wait := chatBackoff << (attempt - 1)
		if retryAfter > 0 {
			wait = retryAfter
		}
		if wait > chatMaxBackoff {
			wait = chatMaxBackoff
		}
		if !c.pauseFor(ctx, wait) {
			return "", lastErr
		}
	}
	return "", lastErr
}

// chatAttempts bounds how many times one completion request is sent. Three
// attempts cover a short provider blip without hiding a sustained outage.
const (
	chatAttempts   = 3
	chatBackoff    = time.Second
	chatMaxBackoff = 10 * time.Second
)

// pauseFor waits for the backoff, returning false when the context ends first.
func (c *Client) pauseFor(ctx context.Context, d time.Duration) bool {
	pause := c.pause
	if pause == nil {
		pause = func(ctx context.Context, d time.Duration) bool {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return false
			case <-timer.C:
				return true
			}
		}
	}
	return pause(ctx, d)
}

// chatOnce performs one attempt. It reports whether the failure is worth
// retrying and any server-requested delay.
func (c *Client) chatOnce(ctx context.Context, base, apiKey string, body []byte) (content string, retryAfter time.Duration, retryable bool, err error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", 0, false, fmt.Errorf("llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return "", 0, ctx.Err() == nil, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", parseRetryAfter(resp.Header.Get("Retry-After")), retryableStatus(resp.StatusCode),
			fmt.Errorf("llm: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, false, fmt.Errorf("llm: decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", 0, false, fmt.Errorf("llm: response contained no choices")
	}
	content = strings.TrimSpace(result.Choices[0].Message.Content)
	if content == "" {
		return "", 0, false, fmt.Errorf("llm: response message content is empty")
	}
	return content, 0, false, nil
}

// retryableStatus reports whether a provider status is transient: rate
// limiting, request timeout, or a server-side failure. Model and auth errors
// (400, 401, 403, 404, 422) are permanent.
func retryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// parseRetryAfter reads a Retry-After header in seconds or HTTP-date form. A
// missing or malformed value yields zero, meaning "use the standard backoff".
func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if wait := time.Until(when); wait > 0 {
			return wait
		}
	}
	return 0
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
