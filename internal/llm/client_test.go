package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/djdembeck/annalist/internal/config"
)

// TestNew verifies New wires the config fields into the client.
func TestNew(t *testing.T) {
	c := New(config.LLMConfig{BaseURL: "https://example.com/v1", APIKey: "sekret", TimeoutS: 7})
	if c.BaseURL != "https://example.com/v1" {
		t.Errorf("BaseURL = %q", c.BaseURL)
	}
	if c.APIKey != "sekret" {
		t.Errorf("APIKey = %q", c.APIKey)
	}
	if c.HTTP == nil {
		t.Fatal("HTTP client is nil")
	}
	if c.HTTP.Timeout != 7e9 {
		t.Errorf("HTTP timeout = %v, want 7s", c.HTTP.Timeout)
	}
}

// llmTestServer records the exact request and returns a canned chat response.
func llmTestServer(t *testing.T, status int, body string) (*httptest.Server, func() *http.Request, func() []byte) {
	t.Helper()
	var mu sync.Mutex
	var gotReq *http.Request
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 0)
		tmp := make([]byte, 4096)
		for {
			n, err := r.Body.Read(tmp)
			b = append(b, tmp[:n]...)
			if err != nil {
				break
			}
		}
		mu.Lock()
		gotReq = r
		gotBody = b
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv,
		func() *http.Request { mu.Lock(); defer mu.Unlock(); return gotReq },
		func() []byte { mu.Lock(); defer mu.Unlock(); return gotBody }
}

func TestChatHappyPath(t *testing.T) {
	srv, getReq, getBody := llmTestServer(t, 200, `{"choices":[{"message":{"content":"  the answer  "}}]}`)

	cfg := config.LLMConfig{BaseURL: srv.URL + "/v1", APIKey: "tok-123", Model: "model-x"}
	c := New(cfg)

	got, err := c.Chat(context.Background(), ChatRequest{
		Model: "model-x", System: "sys", User: "user msg", Temperature: 0.4, MaxTokens: 2048,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "the answer" {
		t.Errorf("content = %q, want %q (trimmed)", got, "the answer")
	}

	r := getReq()
	if r.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", r.Method)
	}
	if r.URL.Path != "/v1/chat/completions" {
		t.Errorf("path = %s, want /v1/chat/completions", r.URL.Path)
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if auth := r.Header.Get("Authorization"); auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", auth)
	}

	var payload struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
	}
	if err := json.Unmarshal(getBody(), &payload); err != nil {
		t.Fatalf("decode wire payload: %v", err)
	}
	if payload.Model != "model-x" {
		t.Errorf("wire model = %q", payload.Model)
	}
	if len(payload.Messages) != 2 || payload.Messages[0].Role != "system" ||
		payload.Messages[1].Role != "user" {
		t.Errorf("wire messages = %+v", payload.Messages)
	}
	if payload.Messages[0].Content != "sys" || payload.Messages[1].Content != "user msg" {
		t.Errorf("wire message contents = %+v", payload.Messages)
	}
	if payload.Temperature != 0.4 {
		t.Errorf("wire temperature = %v", payload.Temperature)
	}
	if payload.MaxTokens != 2048 {
		t.Errorf("wire max_tokens = %d", payload.MaxTokens)
	}
}

// TestChatBaseURLTrim verifies trailing "/" and "/v1" handling so the request
// always hits exactly /v1/chat/completions regardless of how BaseURL is given.
func TestChatBaseURLTrim(t *testing.T) {
	cases := []struct {
		name   string
		suffix string
	}{
		{name: "no v1 suffix", suffix: ""},
		{name: "v1 suffix", suffix: "/v1"},
		{name: "v1 with trailing slash", suffix: "/v1/"},
		{name: "trailing slash only", suffix: "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, getReq, _ := llmTestServer(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
			c := New(config.LLMConfig{BaseURL: srv.URL + tc.suffix, APIKey: "k"})
			if _, err := c.Chat(context.Background(), ChatRequest{User: "u"}); err != nil {
				t.Fatalf("Chat: %v", err)
			}
			// Chained handler assertions are racey within one server; read via
			// a fresh request each run.
			if p := getReq().URL.Path; p != "/v1/chat/completions" {
				t.Fatalf("path = %q, want /v1/chat/completions", p)
			}
		})
	}
}

func TestChatNon2xx(t *testing.T) {
	for _, status := range []int{500, 404} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, _, _ := llmTestServer(t, status, `{"error":"boom"}`)
			c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
			_, err := c.Chat(context.Background(), ChatRequest{User: "u"})
			if err == nil {
				t.Fatalf("Chat(%d) succeeded, want error", status)
			}
			if !strings.Contains(err.Error(), "boom") {
				t.Errorf("error = %q, want status body surfaced", err.Error())
			}
		})
	}
}

func TestChatMalformedJSON(t *testing.T) {
	srv, _, _ := llmTestServer(t, 200, `this is not json`)
	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	if _, err := c.Chat(context.Background(), ChatRequest{User: "u"}); err == nil {
		t.Fatal("Chat succeeded on malformed JSON, want error")
	}
}

func TestChatEmptyChoices(t *testing.T) {
	srv, _, _ := llmTestServer(t, 200, `{"choices":[]}`)
	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	if _, err := c.Chat(context.Background(), ChatRequest{User: "u"}); err == nil {
		t.Fatal("Chat succeeded with empty choices, want error")
	}
}

func TestChatEmptyContent(t *testing.T) {
	srv, _, _ := llmTestServer(t, 200, `{"choices":[{"message":{"content":"   "}}]}`)
	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	if _, err := c.Chat(context.Background(), ChatRequest{User: "u"}); err == nil {
		t.Fatal("Chat succeeded with empty content, want error")
	}
}

func TestChatContextCancelled(t *testing.T) {
	srv, _, _ := llmTestServer(t, 200, `{"choices":[{"message":{"content":"x"}}]}`)
	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Chat(ctx, ChatRequest{User: "u"}); err == nil {
		t.Fatal("Chat succeeded with cancelled context, want error")
	}
}

// TestChatEmptyBaseURL verifies the client has no guard for an empty BaseURL:
// the request URL becomes relative and NewRequest fails.
func TestChatEmptyBaseURL(t *testing.T) {
	c := New(config.LLMConfig{APIKey: "k"})
	if _, err := c.Chat(context.Background(), ChatRequest{User: "u"}); err == nil {
		t.Fatal("Chat succeeded with empty BaseURL, want error")
	}
}
