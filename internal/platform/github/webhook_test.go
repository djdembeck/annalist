package github

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/llm"
	"github.com/djdembeck/annalist/internal/pipeline"
)

// fakePlatform is a fully local pipeline.Platform so webhook dispatch runs
// end to end (clone from local git repo) with no network.
type fakePlatform struct {
	origin   string
	releases map[string]*pipeline.Release
}

func (f *fakePlatform) ReadRepoFile(ctx context.Context, owner, repo, path, ref string) (string, error) {
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
	return f.origin, "Bearer test-token", nil
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

func makePipeline(t *testing.T, baseURL string) (*pipeline.Pipeline, *db.Store, string) {
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
	f := &fakePlatform{origin: makeOrigin(t), releases: map[string]*pipeline.Release{"v0.2.0": {ID: 7, Body: ""}}}
	return pipeline.New(cfg, store, llm.New(cfg.LLM), f, nil), store, dataDir
}

// generatedByReleaseID looks up the generated record by release_id. The
// contract cache key is an opaque digest the test cannot compute, so the poll
// queries the table column directly.
func generatedByReleaseID(dataDir, releaseID string) (*db.GeneratedNote, error) {
	conn, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "app.db"))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var n db.GeneratedNote
	err = conn.QueryRow(
		`SELECT cache_key, platform, owner, repo, release_id, from_tag, to_tag, profile, display_version, config_digest, notes, created_at
		 FROM generated_notes WHERE release_id = ?`, releaseID,
	).Scan(&n.CacheKey, &n.Platform, &n.Owner, &n.Repo, &n.ReleaseID, &n.FromTag, &n.ToTag, &n.Profile, &n.DisplayVersion, &n.ConfigDigest, &n.Notes, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func countGenerated(dataDir string) (int, error) {
	conn, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "app.db"))
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	var n int
	err = conn.QueryRow(`SELECT COUNT(*) FROM generated_notes`).Scan(&n)
	return n, err
}

func llmStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"GITHUB NOTES"}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signGitHub(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

const githubPayload = `{"action":"published","release":{"id":7,"tag_name":"v0.2.0","draft":false},"repository":{"name":"annalist","owner":{"login":"djdembeck"}}}`

// waitForGenerated polls for the async webhook dispatch to land. Webhook
// generation now runs in a background goroutine, so the generated record is
// not guaranteed to exist when the HTTP request returns.
func waitForGenerated(t *testing.T, dataDir, releaseID string) *db.GeneratedNote {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		gn, err := generatedByReleaseID(dataDir, releaseID)
		if err == nil && gn != nil {
			return gn
		}
		time.Sleep(10 * time.Millisecond)
	}
	gn, err := generatedByReleaseID(dataDir, releaseID)
	t.Fatalf("timed out waiting for generated record (%s): %+v (err=%v)", releaseID, gn, err)
	return nil
}

func TestGithubWebhookSignatureAndDispatch(t *testing.T) {
	const secret = "github-secret"
	stub := llmStub(t)
	pip, _, dataDir := makePipeline(t, stub.URL)
	handler := New(config.GitHubConfig{AppID: 1, AppPrivateKeyFile: "", WebhookSecret: secret}).WebhookHandler(pip)

	post := func(t *testing.T, body []byte, sig string, event string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
		if event != "" {
			req.Header.Set("X-GitHub-Event", event)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	correct := signGitHub(secret, []byte(githubPayload))

	t.Run("bad signature is rejected with 401 and nothing is generated", func(t *testing.T) {
		rec := post(t, []byte(githubPayload), "wrong", "release")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if n, err := countGenerated(dataDir); err != nil || n != 0 {
			t.Fatalf("generated record exists after rejected request: count=%d (err=%v)", n, err)
		}
	})

	t.Run("valid signature dispatches published release", func(t *testing.T) {
		rec := post(t, []byte(githubPayload), correct, "release")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		gn := waitForGenerated(t, dataDir, "github:7")
		if gn.ToTag != "v0.2.0" || gn.Owner != "djdembeck" || gn.Repo != "annalist" {
			t.Errorf("generated record = %+v", gn)
		}
		if gn.Notes == "" {
			t.Error("notes should have been generated")
		}
	})

	t.Run("ping event is ignored", func(t *testing.T) {
		rec := post(t, []byte(githubPayload), correct, "ping")
		if rec.Code != http.StatusOK {
			t.Fatalf("ping status = %d, want 200", rec.Code)
		}
	})

	t.Run("draft release is ignored", func(t *testing.T) {
		draft := bytes.Replace([]byte(githubPayload), []byte(`"draft":false`), []byte(`"draft":true`), 1)
		rec := post(t, draft, signGitHub(secret, draft), "release")
		if rec.Code != http.StatusOK {
			t.Fatalf("draft status = %d, want 200", rec.Code)
		}
		// The previously generated record (from the valid dispatch above) is
		// still there and must be untouched by the draft event. May still be
		// landing asynchronously, so wait for it.
		gn := waitForGenerated(t, dataDir, "github:7")
		if gn == nil {
			t.Fatal("expected the earlier record to remain")
		}
	})
}

// TestGithubWebhookPreDispatchRejections covers every branch the handler
// executes BEFORE it ever dispatches into the pipeline: secret-present guard,
// signature verification, event routing, and payload parsing. Because each of
// these short-circuits before p.Resolve / p.GenerateNotes, a nil pipeline is
// safe and proves the handler rejects/stops on its own.
func TestGithubWebhookPreDispatchRejections(t *testing.T) {
	const secret = "github-secret"
	handler := New(config.GitHubConfig{AppID: 1, AppPrivateKeyFile: "", WebhookSecret: secret}).WebhookHandler(nil)

	// post builds a signed request. An empty sig skips the signature header so
	// we can exercise the missing-header path.
	post := func(t *testing.T, body []byte, sig string, event string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		if sig != "" {
			req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
		}
		if event != "" {
			req.Header.Set("X-GitHub-Event", event)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	good := []byte(githubPayload)
	goodSig := signGitHub(secret, good)

	t.Run("missing webhook secret is rejected with 503", func(t *testing.T) {
		h := New(config.GitHubConfig{WebhookSecret: ""}).WebhookHandler(nil)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(good))
		req.Header.Set("X-Hub-Signature-256", "sha256="+goodSig)
		req.Header.Set("X-GitHub-Event", "release")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("missing signature header is rejected with 401", func(t *testing.T) {
		rec := post(t, good, "", "release")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("tampered body (signature mismatch) is rejected with 401", func(t *testing.T) {
		rec := post(t, good, signGitHub("other-secret", good), "release")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("non-release event is ignored with 200 and no dispatch", func(t *testing.T) {
		rec := post(t, good, goodSig, "push")
		if rec.Code != http.StatusOK {
			t.Fatalf("push status = %d, want 200", rec.Code)
		}
	})

	t.Run("malformed JSON payload is rejected with 400", func(t *testing.T) {
		bad := []byte(`{"action":"published",`)
		rec := post(t, bad, signGitHub(secret, bad), "release")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("empty body is rejected at payload stage with 400", func(t *testing.T) {
		// An empty request body reads cleanly and signs cleanly, so it reaches
		// the JSON unmarshal step, which rejects it. Signature must be valid to
		// get past verification, isolating the empty-body path.
		rec := post(t, nil, signGitHub(secret, nil), "release")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("non-published action is ignored with 200", func(t *testing.T) {
		edited := bytes.Replace(good, []byte(`"action":"published"`), []byte(`"action":"deleted"`), 1)
		rec := post(t, edited, signGitHub(secret, edited), "release")
		if rec.Code != http.StatusOK {
			t.Fatalf("deleted status = %d, want 200", rec.Code)
		}
	})
}
