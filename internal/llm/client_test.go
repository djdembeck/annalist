package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
		ThinkingLevel: "low",
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
		Temperature     float64 `json:"temperature"`
		MaxTokens       int     `json:"max_tokens"`
		ReasoningEffort string  `json:"reasoning_effort"`
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
	if payload.ReasoningEffort != "low" {
		t.Errorf("wire reasoning_effort = %q, want low", payload.ReasoningEffort)
	}

	// Unset ThinkingLevel must omit reasoning_effort entirely from the wire.
	if _, err := c.Chat(context.Background(), ChatRequest{Model: "model-x", User: "u"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if strings.Contains(string(getBody()), "reasoning_effort") {
		t.Errorf("wire payload must omit reasoning_effort when unset; got %s", getBody())
	}

	// An explicit "off" must send reasoning_effort "none": omitting the
	// field would leave the model's default behavior in force, which for
	// thinking models is extended thinking.
	if _, err := c.Chat(context.Background(), ChatRequest{Model: "model-x", User: "u", ThinkingLevel: "off"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	var offPayload struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal(getBody(), &offPayload); err != nil {
		t.Fatalf("decode wire payload: %v", err)
	}
	if offPayload.ReasoningEffort != "none" {
		t.Errorf("wire reasoning_effort = %q, want none (explicit off)", offPayload.ReasoningEffort)
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
			c.pause = func(context.Context, time.Duration) bool { return true }
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

// TestListModelsSuccess verifies the happy path: ids are returned in server
// order and the request carries the Bearer key at /v1/models.
func TestListModelsSuccess(t *testing.T) {
	srv, getReq, _ := llmTestServer(t, 200, `{"object":"list","data":[{"id":"a-model"},{"id":"b-model"}]}`)
	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "ignored"})

	ids, err := c.ListModels(context.Background(), srv.URL+"/v1", "key")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(ids) != 2 || ids[0] != "a-model" || ids[1] != "b-model" {
		t.Errorf("ids = %v, want [a-model b-model]", ids)
	}
	r := getReq()
	if r == nil {
		t.Fatal("server recorded no request")
	}
	if r.URL.Path != "/v1/models" {
		t.Errorf("path = %q, want /v1/models", r.URL.Path)
	}
	if auth := r.Header.Get("Authorization"); auth != "Bearer key" {
		t.Errorf("Authorization = %q, want Bearer key", auth)
	}
}

// TestListModelsNon2xx verifies a non-2xx response surfaces the status and
// body in the error.
func TestListModelsNon2xx(t *testing.T) {
	srv, _, _ := llmTestServer(t, 401, `{"error":"bad key"}`)
	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	if _, err := c.ListModels(context.Background(), srv.URL, "k"); err == nil {
		t.Fatal("ListModels succeeded on 401, want error")
	} else if !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Errorf("error = %q, want it to contain the 401 status", err.Error())
	}
}

// TestListModelsEmptyBaseURL verifies the guard fires before any HTTP call.
func TestListModelsEmptyBaseURL(t *testing.T) {
	c := New(config.LLMConfig{APIKey: "k"})
	if _, err := c.ListModels(context.Background(), "", ""); err == nil {
		t.Fatal("ListModels succeeded with empty base URL, want error")
	} else if !strings.Contains(err.Error(), "base url not configured") {
		t.Errorf("error = %q, want 'base url not configured'", err.Error())
	}
}

// TestChatPerRequestOverride verifies Chat uses the per-request BaseURL and
// APIKey over the client's configured values.
func TestChatPerRequestOverride(t *testing.T) {
	srv, getReq, _ := llmTestServer(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
	c := New(config.LLMConfig{BaseURL: "https://ignored.example", APIKey: "client-key"})

	if _, err := c.Chat(context.Background(), ChatRequest{
		Model: "m", User: "u", BaseURL: srv.URL + "/v1", APIKey: "req-key",
	}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	r := getReq()
	if r == nil {
		t.Fatal("server recorded no request; the per-request BaseURL was not used (request never reached the test server)")
	}
	if r.Host != srv.Listener.Addr().String() {
		t.Errorf("request hit host %q, want %q", r.Host, srv.Listener.Addr().String())
	}
	if r.URL.Path != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
	}
	if auth := r.Header.Get("Authorization"); auth != "Bearer req-key" {
		t.Errorf("Authorization = %q, want Bearer req-key", auth)
	}
}

// TestChatAuthorizationHeader verifies Chat only sets the Authorization header
// when the resolved API key is non-empty, so unauthenticated endpoints get no
// empty "Bearer " token (matching ListModels).
func TestChatAuthorizationHeader(t *testing.T) {
	t.Run("empty key omits header", func(t *testing.T) {
		srv, getReq, _ := llmTestServer(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
		c := New(config.LLMConfig{BaseURL: srv.URL})

		// No key at client or request level: the guard must skip the header.
		if _, err := c.Chat(context.Background(), ChatRequest{User: "u"}); err != nil {
			t.Fatalf("Chat: %v", err)
		}
		r := getReq()
		if r == nil {
			t.Fatal("server recorded no request")
		}
		if _, present := r.Header["Authorization"]; present {
			t.Errorf("Authorization header present = %q, want absent", r.Header.Get("Authorization"))
		}
	})

	t.Run("non-empty key sets bearer", func(t *testing.T) {
		srv, getReq, _ := llmTestServer(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
		c := New(config.LLMConfig{BaseURL: srv.URL})

		if _, err := c.Chat(context.Background(), ChatRequest{User: "u", APIKey: "per-req"}); err != nil {
			t.Fatalf("Chat: %v", err)
		}
		r := getReq()
		if r == nil {
			t.Fatal("server recorded no request")
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer per-req" {
			t.Errorf("Authorization = %q, want Bearer per-req", auth)
		}
	})
}

// TestChatRetriesTransientStatus covers the bounded retry: a provider that
// answers 503 once and then succeeds must not fail the run.
func TestChatRetriesTransientStatus(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"quota_exhausted"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"NOTES"}}]}`))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	c.pause = func(context.Context, time.Duration) bool { return true }
	notes, err := c.Chat(context.Background(), ChatRequest{User: "u"})
	if err != nil || notes != "NOTES" {
		t.Fatalf("Chat = %q, %v; want NOTES and no error after retry", notes, err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

// TestChatStopsAfterRetryBudget keeps a sustained outage bounded: three
// attempts, then the provider error surfaces unchanged.
func TestChatStopsAfterRetryBudget(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"quota_exhausted"}}`))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	c.pause = func(context.Context, time.Duration) bool { return true }
	_, err := c.Chat(context.Background(), ChatRequest{User: "u"})
	if err == nil || !strings.Contains(err.Error(), "quota_exhausted") {
		t.Fatalf("error = %v, want the provider body surfaced", err)
	}
	if calls != chatAttempts {
		t.Fatalf("calls = %d, want %d", calls, chatAttempts)
	}
}

// TestChatDoesNotRetryPermanentErrors: an invalid model or auth failure is not
// transient, so it must fail on the first response.
func TestChatDoesNotRetryPermanentErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"permanent"}`))
			}))
			defer srv.Close()

			c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
			c.pause = func(context.Context, time.Duration) bool { return true }
			if _, err := c.Chat(context.Background(), ChatRequest{User: "u"}); err == nil {
				t.Fatalf("status %d succeeded, want error", status)
			}
			if calls != 1 {
				t.Fatalf("calls = %d, want 1 for a permanent error", calls)
			}
		})
	}
}

// TestChatHonorsRetryAfter checks the server-requested delay is used instead
// of the built-in backoff.
func TestChatHonorsRetryAfter(t *testing.T) {
	var calls int
	var waits []time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "4")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	c.pause = func(_ context.Context, d time.Duration) bool { waits = append(waits, d); return true }
	if _, err := c.Chat(context.Background(), ChatRequest{User: "u"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(waits) != 1 || waits[0] != 4*time.Second {
		t.Fatalf("waits = %v, want one 4s pause from Retry-After", waits)
	}
}

// TestChatGrowsBudgetOnEmptyTruncatedAnswer covers a reasoning model that
// spends the whole output budget before writing: the retry raises max_tokens
// instead of failing the release.
func TestChatGrowsBudgetOnEmptyTruncatedAnswer(t *testing.T) {
	var budgets []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		budgets = append(budgets, req.MaxTokens)
		if len(budgets) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"NOTES"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	notes, err := c.Chat(context.Background(), ChatRequest{User: "u", MaxTokens: 4096})
	if err != nil || notes != "NOTES" {
		t.Fatalf("Chat = %q, %v", notes, err)
	}
	if len(budgets) != 2 || budgets[0] != 4096 || budgets[1] != 8192 {
		t.Fatalf("budgets = %v, want [4096 8192]", budgets)
	}
}

// TestChatStopsGrowingAtCap keeps the growth bounded when every attempt is
// truncated, and reports the actionable error.
func TestChatStopsGrowingAtCap(t *testing.T) {
	var budgets []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		budgets = append(budgets, req.MaxTokens)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	_, err := c.Chat(context.Background(), ChatRequest{User: "u", MaxTokens: 16384})
	if err == nil || !strings.Contains(err.Error(), "raise max_tokens") {
		t.Fatalf("error = %v, want the budget guidance", err)
	}
	if len(budgets) != 1 {
		t.Fatalf("budgets = %v, want a single attempt at the cap", budgets)
	}
}

// TestChatEmptyContentWithoutTruncationStillFails: an empty answer with a
// normal finish reason is a provider contract violation, not a budget issue.
func TestChatEmptyContentWithoutTruncationStillFails(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"   "},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	if _, err := c.Chat(context.Background(), ChatRequest{User: "u", MaxTokens: 4096}); err == nil {
		t.Fatal("Chat succeeded on empty content without truncation")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

// TestChatGrowsBudgetForPartialAnswer covers the dangerous truncation: text
// that stops mid-sentence must not be returned as if it were complete.
func TestChatGrowsBudgetForPartialAnswer(t *testing.T) {
	var budgets []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		budgets = append(budgets, req.MaxTokens)
		if len(budgets) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Version 1 adds admin"},"finish_reason":"length"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Version 1 adds admin controls. [[annalist:abc]]"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	notes, err := c.Chat(context.Background(), ChatRequest{User: "u", MaxTokens: 8192})
	if err != nil || !strings.HasSuffix(notes, "[[annalist:abc]]") {
		t.Fatalf("notes = %q, err = %v", notes, err)
	}
	if len(budgets) != 2 || budgets[1] != chatMaxTokens {
		t.Fatalf("budgets = %v, want 8192 then %d", budgets, chatMaxTokens)
	}
}

// TestChatFailsOnTruncatedAnswerAtCap: retrying stops at the cap, and the
// partial text is never returned.
func TestChatFailsOnTruncatedAnswerAtCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{BaseURL: srv.URL, APIKey: "k"})
	notes, err := c.Chat(context.Background(), ChatRequest{User: "u", MaxTokens: 16384})
	if err == nil || notes != "" {
		t.Fatalf("notes = %q, err = %v; want failure with no partial text", notes, err)
	}
	if !strings.Contains(err.Error(), "before the answer completed") {
		t.Fatalf("error = %v", err)
	}
}
