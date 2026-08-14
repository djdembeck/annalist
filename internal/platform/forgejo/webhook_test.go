package forgejo

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/llm"
	"github.com/djdembeck/annalist/internal/pipeline"
)

// fakePlatform is a fully local pipeline.Platform: it clones from a local git
// repo and serves the release from a map so webhook dispatch runs end to end
// with no network.
type fakePlatform struct {
	origin   string
	releases map[string]*pipeline.Release
}

func (f *fakePlatform) ReadRepoFile(ctx context.Context, owner, repo, path string) (string, error) {
	return "", fmt.Errorf("%w: %s", pipeline.ErrNotFound, path)
}
func (f *fakePlatform) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*pipeline.Release, error) {
	if rl, ok := f.releases[tag]; ok {
		return rl, nil
	}
	return nil, fmt.Errorf("%w: %s", pipeline.ErrNotFound, tag)
}
func (f *fakePlatform) EditReleaseBody(ctx context.Context, owner, repo string, releaseID int64, body string) error {
	return nil
}
func (f *fakePlatform) CloneInfo(ctx context.Context, owner, repo string) (string, string, error) {
	return f.origin, "token test-token", nil
}

func gitNoAsk(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@i.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@i.invalid",
		"GIT_CEILING_DIRECTORIES="+dir,
		"PRE_COMMIT_ALLOW_NO_CONFIG=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func makeOrigin(t *testing.T) string {
	origin := filepath.Join(t.TempDir(), "origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	gitNoAsk(t, origin, "init", "-q")
	for i, subject := range []string{"first", "second"} {
		if err := os.WriteFile(filepath.Join(origin, "f.txt"), []byte(subject), 0o644); err != nil {
			t.Fatal(err)
		}
		gitNoAsk(t, origin, "add", "-A")
		gitNoAsk(t, origin, "commit", "-q", "-m", subject)
		if i == 0 {
			gitNoAsk(t, origin, "tag", "v0.1.0")
		} else {
			gitNoAsk(t, origin, "tag", "v0.2.0")
		}
	}
	return origin
}

func makePipeline(t *testing.T, baseURL string) (*pipeline.Pipeline, *db.Store) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := db.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	cfg := &config.Config{
		Data: config.DataConfig{Dir: dataDir},
		LLM: config.LLMConfig{
			BaseURL: baseURL, APIKey: "k", Model: "m", Temperature: 0.5, MaxTokens: 4096,
		},
	}
	f := &fakePlatform{origin: makeOrigin(t), releases: map[string]*pipeline.Release{"v0.2.0": {ID: 42, Body: ""}}}
	return pipeline.New(cfg, store, llm.New(cfg.LLM), nil, f), store
}

func llmStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"WEBHOOK NOTES"}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signForgejo(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// waitForGenerated polls for the async webhook dispatch to land. Webhook
// generation now runs in a background goroutine, so the generated record is
// not guaranteed to exist when the HTTP request returns.
func waitForGenerated(t *testing.T, store *db.Store, platform, releaseID string) *db.GeneratedNote {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		gn, err := store.GetGenerated(platform, releaseID)
		if err == nil && gn != nil {
			return gn
		}
		time.Sleep(10 * time.Millisecond)
	}
	gn, err := store.GetGenerated(platform, releaseID)
	t.Fatalf("timed out waiting for generated record (%s/%s): %+v (err=%v)", platform, releaseID, gn, err)
	return nil
}

const forgejoPayload = `{"action":"created","release":{"id":42,"tag_name":"v0.2.0","draft":false},"repository":{"full_name":"djdembeck/annalist","name":"annalist","owner":{"login":"djdembeck","username":"djdembeck"}}}`

func TestForgejoWebhookSignatureAndDispatch(t *testing.T) {
	const secret = "forgejo-secret"
	stub := llmStub(t)
	pip, store := makePipeline(t, stub.URL)
	handler := New(config.ForgejoConfig{URL: "https://git.example.invalid", Token: "tok", WebhookSecret: secret}).WebhookHandler(pip)

	post := func(t *testing.T, body []byte, sigs map[string]string, event string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		for k, v := range sigs {
			req.Header.Set(k, v)
		}
		if event != "" {
			req.Header.Set("X-Gitea-Event", event)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	correct := signForgejo(secret, []byte(forgejoPayload))

	t.Run("bad signature is rejected with 401 and nothing is generated", func(t *testing.T) {
		rec := post(t, []byte(forgejoPayload), map[string]string{"X-Gitea-Signature": "deadbeef"}, "release")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if gn, err := store.GetGenerated("forgejo", "forgejo:42"); err != nil || gn != nil {
			t.Fatalf("generated record exists after rejected request: %+v, %v", gn, err)
		}
	})

	t.Run("valid X-Gitea-Signature dispatches release event", func(t *testing.T) {
		rec := post(t, []byte(forgejoPayload), map[string]string{"X-Gitea-Signature": correct}, "release")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		gn := waitForGenerated(t, store, "forgejo", "forgejo:42")
		if gn.Tag != "v0.2.0" || gn.Owner != "djdembeck" || gn.Repo != "annalist" {
			t.Errorf("generated record = %+v", gn)
		}
		if !strings.Contains(gn.Notes, "WEBHOOK NOTES") {
			t.Errorf("notes = %q", gn.Notes)
		}
	})

	t.Run("valid X-Hub-Signature-256 also accepted", func(t *testing.T) {
		payload := strings.ReplaceAll(forgejoPayload, `"id":42`, `"id":43`)
		body := []byte(payload)
		rec := post(t, body, map[string]string{"X-Hub-Signature-256": "sha256=" + signForgejo(secret, body)}, "release")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		_ = waitForGenerated(t, store, "forgejo", "forgejo:43")
	})

	t.Run("draft or non-release action is ignored", func(t *testing.T) {
		draft := strings.Replace(forgejoPayload, `"id":42`, `"id":99`, 1)
		draft = strings.Replace(draft, `"draft":false`, `"draft":true`, 1)
		body := []byte(draft)
		rec := post(t, body, map[string]string{"X-Gitea-Signature": signForgejo(secret, body)}, "release")
		if rec.Code != http.StatusOK {
			t.Fatalf("draft status = %d, want 200", rec.Code)
		}
		if gn, _ := store.GetGenerated("forgejo", "forgejo:99"); gn != nil {
			t.Error("draft release must not be generated")
		}

		deleted := strings.Replace(forgejoPayload, `"id":42`, `"id":100`, 1)
		deleted = strings.Replace(deleted, `"action":"created"`, `"action":"deleted"`, 1)
		body2 := []byte(deleted)
		rec2 := post(t, body2, map[string]string{"X-Gitea-Signature": signForgejo(secret, body2)}, "release")
		if rec2.Code != http.StatusOK {
			t.Fatalf("deleted status = %d, want 200", rec2.Code)
		}
		if gn, _ := store.GetGenerated("forgejo", "100"); gn != nil {
			t.Error("non-created/updated action must not be generated")
		}
	})
}

// TestForgejoWebhookPreDispatchRejections covers every branch the handler
// executes BEFORE it dispatches into the pipeline: secret-present guard,
// signature verification, event routing, and payload parsing. Each of these
// short-circuits before p.Resolve / p.GenerateNotes, so a nil pipeline is safe
// and proves the handler rejects/stops on its own.
func TestForgejoWebhookPreDispatchRejections(t *testing.T) {
	const secret = "forgejo-secret"
	handler := New(config.ForgejoConfig{URL: "https://git.example.invalid", Token: "tok", WebhookSecret: secret}).WebhookHandler(nil)

	// post sends a request with the given signature headers and optional event.
	// An empty sigs map exercises the missing-header path.
	post := func(t *testing.T, body []byte, sigs map[string]string, event string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		for k, v := range sigs {
			req.Header.Set(k, v)
		}
		if event != "" {
			req.Header.Set("X-Gitea-Event", event)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	good := []byte(forgejoPayload)
	goodSig := signForgejo(secret, good)

	t.Run("missing webhook secret is rejected with 503", func(t *testing.T) {
		h := New(config.ForgejoConfig{URL: "https://git.example.invalid", Token: "tok"}).WebhookHandler(nil)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(good))
		req.Header.Set("X-Gitea-Signature", goodSig)
		req.Header.Set("X-Gitea-Event", "release")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("missing signature header is rejected with 401", func(t *testing.T) {
		rec := post(t, good, nil, "release")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("signature under unknown header name is rejected with 401", func(t *testing.T) {
		rec := post(t, good, map[string]string{"X-Madeup-Signature": goodSig}, "release")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("malformed hex signature is rejected with 401", func(t *testing.T) {
		rec := post(t, good, map[string]string{"X-Gitea-Signature": "not-hex!!"}, "release")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("tampered body (signature mismatch) is rejected with 401", func(t *testing.T) {
		rec := post(t, good, map[string]string{"X-Gitea-Signature": signForgejo("other-secret", good)}, "release")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("non-release event is ignored with 200", func(t *testing.T) {
		rec := post(t, good, map[string]string{"X-Gitea-Signature": goodSig}, "push")
		if rec.Code != http.StatusOK {
			t.Fatalf("push status = %d, want 200", rec.Code)
		}
	})

	t.Run("missing event header is ignored with 200", func(t *testing.T) {
		// No event header at all → webhookEvent returns "" → not "release".
		rec := post(t, good, map[string]string{"X-Gitea-Signature": goodSig}, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("malformed JSON payload is rejected with 400", func(t *testing.T) {
		bad := []byte(`{"action":"created",`)
		rec := post(t, bad, map[string]string{"X-Gitea-Signature": signForgejo(secret, bad)}, "release")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("empty body is rejected at payload stage with 400", func(t *testing.T) {
		// Empty body reads cleanly and signs cleanly, so it reaches JSON
		// unmarshal, which rejects it. Signature must be valid to isolate the
		// empty-body path.
		rec := post(t, nil, map[string]string{"X-Gitea-Signature": signForgejo(secret, nil)}, "release")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("non-created/updated action is ignored with 200", func(t *testing.T) {
		deleted := bytes.Replace(good, []byte(`"action":"created"`), []byte(`"action":"deleted"`), 1)
		rec := post(t, deleted, map[string]string{"X-Gitea-Signature": signForgejo(secret, deleted)}, "release")
		if rec.Code != http.StatusOK {
			t.Fatalf("deleted status = %d, want 200", rec.Code)
		}
	})

	t.Run("empty tag_name is ignored with 200", func(t *testing.T) {
		notag := bytes.Replace(good, []byte(`"tag_name":"v0.2.0"`), []byte(`"tag_name":""`), 1)
		rec := post(t, notag, map[string]string{"X-Gitea-Signature": signForgejo(secret, notag)}, "release")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("missing owner is ignored with 200", func(t *testing.T) {
		noowner := bytes.Replace(good, []byte(`"full_name":"djdembeck/annalist"`), []byte(`"full_name":""`), 1)
		noowner = bytes.Replace(noowner, []byte(`"login":"djdembeck"`), []byte(`"login":""`), 1)
		noowner = bytes.Replace(noowner, []byte(`"username":"djdembeck"`), []byte(`"username":""`), 1)
		rec := post(t, noowner, map[string]string{"X-Gitea-Signature": signForgejo(secret, noowner)}, "release")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})
}
