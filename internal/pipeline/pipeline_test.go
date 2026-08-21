package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/engine"
	"github.com/djdembeck/annalist/internal/llm"
)

// fakePlatform satisfies pipeline.Platform without any network. It points the
// clone at a local git repository and serves repo files / releases from maps.
type fakePlatform struct {
	origin     string // local dir used as the clone source
	files      map[string]string
	releases   map[string]*Release // keyed by tag
	edited     string
	cloneErr   error // if set, CloneInfo returns it
	editErr    error // if set, EditReleaseBody returns it
	readErr    error // if set, ReadRepoFile returns it (before checking files)
	releaseErr error // if set, GetReleaseByTag returns it (before checking releases)
}

func (f *fakePlatform) ReadRepoFile(ctx context.Context, owner, repo, path string) (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	if c, ok := f.files[path]; ok {
		return c, nil
	}
	return "", fmt.Errorf("%w: %s", ErrNotFound, path)
}

func (f *fakePlatform) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*Release, error) {
	if f.releaseErr != nil {
		return nil, f.releaseErr
	}
	if rl, ok := f.releases[tag]; ok {
		return rl, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, tag)
}

func (f *fakePlatform) EditReleaseBody(ctx context.Context, owner, repo string, releaseID int64, body string) error {
	if f.editErr != nil {
		return f.editErr
	}
	f.edited = body
	return nil
}

func (f *fakePlatform) CloneInfo(ctx context.Context, owner, repo string) (string, string, error) {
	if f.cloneErr != nil {
		return "", "", f.cloneErr
	}
	return f.origin, "Bearer test-token", nil
}

// llmStub records every chat request and returns a fixed, non-empty answer.
type llmStub struct {
	srv       *httptest.Server
	body      []byte
	calls     int
	answer    string
	errStatus int // if > 0, respond with this HTTP status instead of 200
}

func newStub(answer string) *llmStub {
	s := &llmStub{answer: answer}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.body, _ = io.ReadAll(r.Body)
		s.calls++
		if s.errStatus > 0 {
			w.WriteHeader(s.errStatus)
			_, _ = fmt.Fprint(w, "boom")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, s.answer)
	}))
	return s
}

func (s *llmStub) close() { s.srv.Close() }

type wireReq struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Temperature     float64 `json:"temperature"`
	MaxTokens       int     `json:"max_tokens"`
	ReasoningEffort string  `json:"reasoning_effort"`
}

func (s *llmStub) request() (wireReq, error) {
	var r wireReq
	err := json.Unmarshal(s.body, &r)
	return r, err
}

func (s *llmStub) system() string {
	r, err := s.request()
	if err != nil || len(r.Messages) == 0 {
		return ""
	}
	return r.Messages[0].Content
}

func (s *llmStub) user() string {
	r, err := s.request()
	if err != nil || len(r.Messages) < 2 {
		return ""
	}
	return r.Messages[1].Content
}

// gitRun executes git in dir without a *testing.T, for use in TestMain.
func gitRun(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		"GIT_CEILING_DIRECTORIES="+dir,
		"PRE_COMMIT_ALLOW_NO_CONFIG=1")
	return cmd.CombinedOutput()
}

func gitNoAsk(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := gitRun(dir, args...); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// buildOriginErr creates an origin repo directory at dir/origin populated with
// two commits tagged v0.1.0 and v0.2.0.
func buildOriginErr(dir string) (string, error) {
	origin := filepath.Join(dir, "origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		return "", err
	}
	if out, err := gitRun(origin, "init", "-q"); err != nil {
		return "", fmt.Errorf("git init: %v\n%s", err, out)
	}

	commit := func(subject string, tags ...string) error {
		if err := os.WriteFile(filepath.Join(origin, "f.txt"), []byte(subject), 0o644); err != nil {
			return err
		}
		if out, err := gitRun(origin, "add", "-A"); err != nil {
			return fmt.Errorf("git add: %v\n%s", err, out)
		}
		if out, err := gitRun(origin, "commit", "-q", "-m", subject); err != nil {
			return fmt.Errorf("git commit: %v\n%s", err, out)
		}
		for _, tag := range tags {
			if out, err := gitRun(origin, "tag", tag); err != nil {
				return fmt.Errorf("git tag: %v\n%s", err, out)
			}
		}
		return nil
	}

	if err := commit("first: scaffolding", "v0.1.0"); err != nil {
		return "", err
	}
	if err := commit("second: feature x", "v0.2.0"); err != nil {
		return "", err
	}
	return origin, nil
}

// buildOrigin creates an origin repo at dir/origin (t.Fatal on failure), for
// tests that need a bespoke repo layout.
func buildOrigin(t *testing.T, dir string) string {
	t.Helper()
	origin, err := buildOriginErr(dir)
	if err != nil {
		t.Fatal(err)
	}
	return origin
}

// makeOrigin builds a fresh origin in dir (kept for tests that need a
// bespoke repo layout).
func makeOrigin(t *testing.T, dir string) string {
	return buildOrigin(t, dir)
}

// sharedOrigin is the standard two-commit origin, built once in TestMain and
// cloned read-only by every GenerateNotes call. Sharing the source cuts
// per-test git init/commit/tag subprocess churn; each clone lands in its own
// workdir so isolation is preserved.
var sharedOrigin string

// TestMain builds the shared origin once for the whole package run.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pipeline-shared-origin-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pipeline: create shared origin: %v\n", err)
		os.Exit(1)
	}
	sharedOrigin, err = buildOriginErr(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pipeline: build shared origin: %v\n", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// sharedStandardOrigin returns the origin built once in TestMain.
func sharedStandardOrigin(t *testing.T) string {
	t.Helper()
	return sharedOrigin
}

// fixture wires a full pipeline: temp store + config, LLM stub, local origin
// repo, and a fake platform.
func fixture(t *testing.T, files map[string]string, releases map[string]*Release) (*Pipeline, *llmStub, *fakePlatform, *db.Store) {
	return fixtureWithStub(t, newStub("FLOWING PROSE"), files, releases)
}

func fixtureWithStub(t *testing.T, stub *llmStub, files map[string]string, releases map[string]*Release) (*Pipeline, *llmStub, *fakePlatform, *db.Store) {
	t.Helper()
	dir := t.TempDir()

	t.Cleanup(stub.close)

	store, err := db.New(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	cfg := &config.Config{
		Data: config.DataConfig{Dir: filepath.Join(dir, "data")},
		LLM: config.LLMConfig{
			BaseURL:     stub.srv.URL,
			APIKey:      "key",
			Model:       "qwen3.5-397b-a17b",
			Temperature: 0.85,
			MaxTokens:   4096,
		},
	}

	origin := sharedStandardOrigin(t)
	f := &fakePlatform{origin: origin, files: files, releases: releases}
	pip := New(cfg, store, llm.New(cfg.LLM), f, nil)
	return pip, stub, f, store
}

func genSpec(tag, releaseID string) Spec {
	return Spec{Platform: "github", Owner: "djdembeck", Repo: "annalist", ToTag: tag, ReleaseID: releaseID}
}

// TestResolvePrecedence covers the resolution chain per field:
// repo row ?? global ?? LLM config default, plus the enabled flag defaulting to
// true when no row exists.
func TestResolvePrecedence(t *testing.T) {
	_, _, _, store := fixture(t, nil, nil)
	pip := &Pipeline{Cfg: &config.Config{LLM: config.LLMConfig{Model: "default-model", Temperature: 0.7}}, DB: store}

	t0, t1, t2 := 0.2, 0.9, 0.1
	if err := store.UpsertSettings(db.Settings{
		Tone: "global-tone", Instructions: "global-instructions", Model: "global-model",
		Temperature: &t0,
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("no row falls back to global", func(t *testing.T) {
		enabled, eff, r, err := pip.Resolve(context.Background(), "github", "o", "r")
		if err != nil {
			t.Fatal(err)
		}
		if !enabled {
			t.Error("enabled should default to true for a missing row")
		}
		if r.Tone != "global-tone" || r.Instructions != "global-instructions" ||
			r.Model != "global-model" || r.Temperature != t0 {
			t.Errorf("resolved = %+v", r)
		}
		// The fixture config has no LLM BaseURL/APIKey, so the effective
		// endpoint is empty (nothing to inherit).
		if eff.BaseURL != "" || eff.APIKey != "" {
			t.Errorf("effective endpoint = (%q, %q), want empty (no cfg base url)", eff.BaseURL, eff.APIKey)
		}
	})

	t.Run("row overrides tone/model/temperature, inherits instructions", func(t *testing.T) {
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "o", Repo: "r", Enabled: true,
			Tone: "repo-tone", Instructions: "", Model: "repo-model", Temperature: &t1,
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r")
		if err != nil {
			t.Fatal(err)
		}
		if r.Tone != "repo-tone" || r.Model != "repo-model" || r.Temperature != t1 {
			t.Errorf("row overrides not applied: %+v", r)
		}
		if r.Instructions != "global-instructions" {
			t.Errorf("instructions should inherit from global: %q", r.Instructions)
		}
	})

	t.Run("row with empty tone inherits global tone", func(t *testing.T) {
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "forgejo", Owner: "o", Repo: "r2", Enabled: true,
			Tone: "", Model: "", Temperature: &t2,
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "forgejo", "o", "r2")
		if err != nil {
			t.Fatal(err)
		}
		if r.Tone != "global-tone" || r.Temperature != t2 {
			t.Errorf("mixed inheritance wrong: %+v", r)
		}
	})

	t.Run("disabled row reports enabled=false", func(t *testing.T) {
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "o", Repo: "r", Enabled: false,
			Tone: "", Instructions: "", Model: "", Temperature: nil,
		}); err != nil {
			t.Fatal(err)
		}
		enabled, _, _, err := pip.Resolve(context.Background(), "github", "o", "r")
		if err != nil {
			t.Fatal(err)
		}
		if enabled {
			t.Error("enabled should be false for a disabled row")
		}
	})

	t.Run("commit types: no row falls back to global", func(t *testing.T) {
		if err := store.UpsertSettings(db.Settings{
			Tone: "global-tone", CommitTypes: "fix,feat",
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-new")
		if err != nil {
			t.Fatal(err)
		}
		if len(r.CommitTypes) != 2 || r.CommitTypes[0] != "fix" || r.CommitTypes[1] != "feat" {
			t.Errorf("commit types = %v, want [fix feat]", r.CommitTypes)
		}
	})

	t.Run("commit types: repo overrides global", func(t *testing.T) {
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "o", Repo: "r-new",
			Enabled: true, CommitTypes: "fix",
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-new")
		if err != nil {
			t.Fatal(err)
		}
		if len(r.CommitTypes) != 1 || r.CommitTypes[0] != "fix" {
			t.Errorf("commit types = %v, want [fix]", r.CommitTypes)
		}
	})

	t.Run("commit types: empty repo inherits global", func(t *testing.T) {
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "o", Repo: "r-new",
			Enabled: true, CommitTypes: "",
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-new")
		if err != nil {
			t.Fatal(err)
		}
		if len(r.CommitTypes) != 2 || r.CommitTypes[0] != "fix" || r.CommitTypes[1] != "feat" {
			t.Errorf("commit types = %v, want [fix feat]", r.CommitTypes)
		}
	})

	t.Run("commit types: fresh store falls back to config", func(t *testing.T) {
		fresh, err := db.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { fresh.Close() })
		pip3 := &Pipeline{Cfg: &config.Config{LLM: config.LLMConfig{Model: "default-model", CommitTypes: "fix,perf"}}, DB: fresh}
		_, _, r, err := pip3.Resolve(context.Background(), "github", "o", "r-fresh")
		if err != nil {
			t.Fatal(err)
		}
		if len(r.CommitTypes) != 2 || r.CommitTypes[0] != "fix" || r.CommitTypes[1] != "perf" {
			t.Errorf("commit types = %v, want [fix perf]", r.CommitTypes)
		}
		// Twin: config also empty → keep-all (nil).
		pip4 := &Pipeline{Cfg: &config.Config{LLM: config.LLMConfig{Model: "default-model"}}, DB: fresh}
		_, _, r, err = pip4.Resolve(context.Background(), "github", "o", "r-fresh")
		if err != nil {
			t.Fatal(err)
		}
		if r.CommitTypes != nil {
			t.Errorf("commit types = %v, want nil (keep-all)", r.CommitTypes)
		}
	})

	t.Run("commit types: no global falls back to config", func(t *testing.T) {
		pip2 := &Pipeline{Cfg: &config.Config{LLM: config.LLMConfig{Model: "default-model", Temperature: 0.7, CommitTypes: "perf"}}, DB: store}
		if err := store.UpsertSettings(db.Settings{
			Tone: "", CommitTypes: "",
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "o", Repo: "r-new",
			Enabled: true, CommitTypes: "",
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip2.Resolve(context.Background(), "github", "o", "r-new")
		if err != nil {
			t.Fatal(err)
		}
		if len(r.CommitTypes) != 1 || r.CommitTypes[0] != "perf" {
			t.Errorf("commit types = %v, want [perf]", r.CommitTypes)
		}
	})

	t.Run("saved db endpoint overrides config", func(t *testing.T) {
		// A fresh pipeline whose config LLM has no BaseURL/APIKey.
		pip3 := &Pipeline{Cfg: &config.Config{LLM: config.LLMConfig{Model: "default-model"}}, DB: store}
		// Reset the global row, then set a saved endpoint.
		if err := store.UpsertSettings(db.Settings{Tone: "", CommitTypes: ""}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertSettings(db.Settings{BaseURL: "https://saved", APIKey: "saved-key"}); err != nil {
			t.Fatal(err)
		}
		_, eff, _, err := pip3.Resolve(context.Background(), "github", "o", "r")
		if err != nil {
			t.Fatal(err)
		}
		if eff.BaseURL != "https://saved" || eff.APIKey != "saved-key" {
			t.Errorf("effective endpoint = (%q, %q), want (https://saved, saved-key)", eff.BaseURL, eff.APIKey)
		}
	})
}

// TestResolveModePrecedence covers the mode resolution chain:
// repo row → global row → default "lite".
func TestResolveModePrecedence(t *testing.T) {
	_, _, _, store := fixture(t, nil, nil)
	pip := &Pipeline{Cfg: &config.Config{LLM: config.LLMConfig{Model: "default-model", Temperature: 0.7}}, DB: store}

	t.Run("global mode applies with no repo row", func(t *testing.T) {
		if err := store.UpsertSettings(db.Settings{Mode: "deep"}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r")
		if err != nil {
			t.Fatal(err)
		}
		if r.Mode != "deep" {
			t.Errorf("mode = %q, want deep", r.Mode)
		}
	})

	t.Run("repo row beats global", func(t *testing.T) {
		if err := store.UpsertSettings(db.Settings{Mode: "lite"}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "o", Repo: "r", Enabled: true, Mode: "deep",
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r")
		if err != nil {
			t.Fatal(err)
		}
		if r.Mode != "deep" {
			t.Errorf("mode = %q, want deep (row wins)", r.Mode)
		}
	})

	t.Run("both empty defaults to lite", func(t *testing.T) {
		if err := store.UpsertSettings(db.Settings{Mode: ""}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "forgejo", "o", "r-empty")
		if err != nil {
			t.Fatal(err)
		}
		if r.Mode != "lite" {
			t.Errorf("mode = %q, want lite", r.Mode)
		}
	})
}

// TestResolveMaxTokensThinkingPrecedence covers the max_tokens and
// thinking_level resolution chains: repo row → global row → config.
func TestResolveMaxTokensThinkingPrecedence(t *testing.T) {
	_, _, _, store := fixture(t, nil, nil)
	pip := &Pipeline{Cfg: &config.Config{LLM: config.LLMConfig{
		Model: "default-model", Temperature: 0.7, MaxTokens: 3000, ThinkingLevel: "low",
	}}, DB: store}

	t.Run("no rows resolve to config", func(t *testing.T) {
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-none")
		if err != nil {
			t.Fatal(err)
		}
		if r.MaxTokens != 3000 || r.ThinkingLevel != "low" {
			t.Errorf("resolved = (%d, %q), want (3000, low)", r.MaxTokens, r.ThinkingLevel)
		}
	})

	t.Run("global row beats config", func(t *testing.T) {
		if err := store.UpsertSettings(db.Settings{MaxTokens: 5000, ThinkingLevel: "medium"}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-global")
		if err != nil {
			t.Fatal(err)
		}
		if r.MaxTokens != 5000 || r.ThinkingLevel != "medium" {
			t.Errorf("resolved = (%d, %q), want (5000, medium)", r.MaxTokens, r.ThinkingLevel)
		}
	})

	t.Run("repo row beats global", func(t *testing.T) {
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "o", Repo: "r-repo", Enabled: true,
			MaxTokens: 7000, ThinkingLevel: "high",
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-repo")
		if err != nil {
			t.Fatal(err)
		}
		if r.MaxTokens != 7000 || r.ThinkingLevel != "high" {
			t.Errorf("resolved = (%d, %q), want (7000, high)", r.MaxTokens, r.ThinkingLevel)
		}
	})

	t.Run("unset repo row inherits global", func(t *testing.T) {
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "o", Repo: "r-repo", Enabled: true,
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-repo")
		if err != nil {
			t.Fatal(err)
		}
		if r.MaxTokens != 5000 || r.ThinkingLevel != "medium" {
			t.Errorf("resolved = (%d, %q), want (5000, medium) from global", r.MaxTokens, r.ThinkingLevel)
		}
	})

	t.Run("global off suppresses config level", func(t *testing.T) {
		if err := store.UpsertSettings(db.Settings{MaxTokens: 0, ThinkingLevel: "off"}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-off-global")
		if err != nil {
			t.Fatal(err)
		}
		// Config says "low" but the explicit global off must win.
		if r.ThinkingLevel != "" {
			t.Errorf("thinking = %q, want empty (explicit off suppresses config)", r.ThinkingLevel)
		}
	})

	t.Run("repo off suppresses global level", func(t *testing.T) {
		if err := store.UpsertSettings(db.Settings{MaxTokens: 0, ThinkingLevel: "medium"}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "o", Repo: "r-off-repo", Enabled: true,
			ThinkingLevel: "off",
		}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-off-repo")
		if err != nil {
			t.Fatal(err)
		}
		// Global says "medium" but the explicit repo off must win.
		if r.ThinkingLevel != "" {
			t.Errorf("thinking = %q, want empty (explicit off suppresses global)", r.ThinkingLevel)
		}
	})

	t.Run("empty rows still inherit config level", func(t *testing.T) {
		if err := store.UpsertSettings(db.Settings{MaxTokens: 0, ThinkingLevel: ""}); err != nil {
			t.Fatal(err)
		}
		_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r-unsup")
		if err != nil {
			t.Fatal(err)
		}
		if r.ThinkingLevel != "low" {
			t.Errorf("thinking = %q, want low (unset inherits config)", r.ThinkingLevel)
		}
	})
}

// TestResolveOffConfigLevel verifies a config-level "off" (e.g.
// LLM_THINKING_LEVEL=off) also suppresses reasoning_effort.
func TestResolveOffConfigLevel(t *testing.T) {
	_, _, _, store := fixture(t, nil, nil)
	pip := &Pipeline{Cfg: &config.Config{LLM: config.LLMConfig{
		Model: "default-model", ThinkingLevel: "off",
	}}, DB: store}
	_, _, r, err := pip.Resolve(context.Background(), "github", "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if r.ThinkingLevel != "" {
		t.Errorf("thinking = %q, want empty (config off)", r.ThinkingLevel)
	}
}

// TestGenerateNotesMaxTokensThinkingWire verifies the resolved max_tokens and
// thinking level reach the wire (max_tokens + reasoning_effort) through the
// full GenerateNotes flow.
func TestGenerateNotesMaxTokensThinkingWire(t *testing.T) {
	pip, stub, _, store := fixture(t, nil, nil)
	if err := store.UpsertRepoSettings(db.RepoSetting{
		Platform: "github", Owner: "djdembeck", Repo: "annalist", Enabled: true,
		MaxTokens: 1234, ThinkingLevel: "high",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-tok"), Options{}); err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	req, err := stub.request()
	if err != nil {
		t.Fatalf("decode stub request: %v", err)
	}
	if req.MaxTokens != 1234 {
		t.Errorf("wire max_tokens = %d, want 1234", req.MaxTokens)
	}
	if req.ReasoningEffort != "high" {
		t.Errorf("wire reasoning_effort = %q, want high", req.ReasoningEffort)
	}
}

// TestGenerateNotesOffWire verifies an explicit "off" thinking level omits
// reasoning_effort from the wire even when the config sets a level.
func TestGenerateNotesOffWire(t *testing.T) {
	pip, stub, _, store := fixture(t, nil, nil)
	if err := store.UpsertSettings(db.Settings{ThinkingLevel: "off"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-off"), Options{}); err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	req, err := stub.request()
	if err != nil {
		t.Fatalf("decode stub request: %v", err)
	}
	if req.ReasoningEffort != "" {
		t.Errorf("wire reasoning_effort = %q, want omitted (explicit off)", req.ReasoningEffort)
	}
}

// TestGenerateNotesDeepMode verifies the full GenerateNotes flow: deep mode
// sends the diff to the LLM, the default (lite) does not.
func TestGenerateNotesDeepMode(t *testing.T) {
	t.Run("deep mode includes the diff in the prompt", func(t *testing.T) {
		pip, stub, _, _ := fixture(t, nil, nil)
		notes, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-deep"), Options{Mode: "deep"})
		if err != nil {
			t.Fatalf("GenerateNotes deep: %v", err)
		}
		if notes != "FLOWING PROSE" {
			t.Errorf("notes = %q, want FLOWING PROSE", notes)
		}
		if stub.calls != 1 {
			t.Fatalf("stub calls = %d, want 1", stub.calls)
		}
		u := stub.user()
		for _, want := range []string{"<diff>", "f.txt", "</diff>"} {
			if !strings.Contains(u, want) {
				t.Errorf("deep user message missing %q; got:\n%s", want, u)
			}
		}
	})

	t.Run("default mode stays lite (no diff block)", func(t *testing.T) {
		pip, stub, _, _ := fixture(t, nil, nil)
		if _, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-default"), Options{}); err != nil {
			t.Fatalf("GenerateNotes default: %v", err)
		}
		if stub.calls != 1 {
			t.Fatalf("stub calls = %d, want 1", stub.calls)
		}
		u := stub.user()
		if strings.Contains(u, "<diff>") {
			t.Errorf("lite user message must not contain <diff>; got:\n%s", u)
		}
	})

	t.Run("resolved mode deep from global settings sends the diff", func(t *testing.T) {
		pip, stub, _, store := fixture(t, nil, nil)
		if err := store.UpsertSettings(db.Settings{Mode: engine.ModeDeep}); err != nil {
			t.Fatalf("UpsertSettings: %v", err)
		}
		if _, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-resolved-deep"), Options{}); err != nil {
			t.Fatalf("GenerateNotes resolved deep: %v", err)
		}
		if stub.calls != 1 {
			t.Fatalf("stub calls = %d, want 1", stub.calls)
		}
		u := stub.user()
		if !strings.Contains(u, "<diff>") || !strings.Contains(u, "f.txt") {
			t.Errorf("resolved-deep user message missing diff block or file path; got:\n%s", u)
		}
	})

	t.Run("repo row lite overrides global deep (no diff block)", func(t *testing.T) {
		pip, stub, _, store := fixture(t, nil, nil)
		if err := store.UpsertSettings(db.Settings{Mode: engine.ModeDeep}); err != nil {
			t.Fatalf("UpsertSettings: %v", err)
		}
		if err := store.UpsertRepoSettings(db.RepoSetting{
			Platform: "github", Owner: "djdembeck", Repo: "annalist", Enabled: true, Mode: engine.ModeLite,
		}); err != nil {
			t.Fatalf("UpsertRepoSettings: %v", err)
		}
		if _, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-resolved-lite"), Options{}); err != nil {
			t.Fatalf("GenerateNotes repo-lite: %v", err)
		}
		if stub.calls != 1 {
			t.Fatalf("stub calls = %d, want 1", stub.calls)
		}
		u := stub.user()
		if strings.Contains(u, "<diff>") {
			t.Errorf("repo-lite user message must not contain <diff>; got:\n%s", u)
		}
	})
}

// instructionsPathFor matches the pipeline's per-platform in-repo file path.
func instructionsPathFor(platform string) string {
	if platform == "forgejo" {
		return ".forgejo/release-notes.md"
	}
	return ".github/release-notes-instructions.md"
}

// TestInstructionsPrecedence drives the full GenerateNotes flow and asserts the
// system prompt carries in-repo file > repo row > global instructions.
func TestInstructionsPrecedence(t *testing.T) {
	run := func(t *testing.T, files map[string]string, rowInstructions, globalInstructions string, wantContains, wantAbsent string) {
		t.Helper()
		pip, stub, _, store := fixture(t, files, map[string]*Release{"v0.2.0": {ID: 99, Body: ""}})

		if globalInstructions != "" {
			if err := store.UpsertSettings(db.Settings{
				Instructions: globalInstructions, Tone: "chronicler",
			}); err != nil {
				t.Fatal(err)
			}
		}
		if rowInstructions != "" {
			if err := store.UpsertRepoSettings(db.RepoSetting{
				Platform: "github", Owner: "djdembeck", Repo: "annalist", Enabled: true,
				Instructions: rowInstructions, Tone: "chronicler",
			}); err != nil {
				t.Fatal(err)
			}
		}

		notes, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-"+wantContains), Options{})
		if err != nil {
			t.Fatalf("GenerateNotes: %v", err)
		}
		if notes != "FLOWING PROSE" {
			t.Errorf("notes = %q", notes)
		}
		sys := stub.system()
		if wantContains != "" && !strings.Contains(sys, wantContains) {
			t.Errorf("system prompt missing %q:\n%s", wantContains, sys)
		}
		if wantAbsent != "" && strings.Contains(sys, wantAbsent) {
			t.Errorf("system prompt unexpectedly contains %q", wantAbsent)
		}
		// The user prompt must name the version and the collected commit log.
		user := stub.user()
		if !strings.Contains(user, "Generate release notes for version v0.2.0.") {
			t.Errorf("user prompt missing version: %q", user)
		}
		if !strings.Contains(user, "- second: feature x") {
			t.Errorf("user prompt missing commit log: %q", user)
		}
	}

	t.Run("in-repo file beats row beats global", func(t *testing.T) {
		run(t,
			map[string]string{instructionsPathFor("github"): "IN-REPO FILE INSTRUCTIONS"},
			"ROW INSTRUCTIONS", "GLOBAL INSTRUCTIONS",
			"IN-REPO FILE INSTRUCTIONS", "ROW INSTRUCTIONS")
	})
	t.Run("row beats global when no file", func(t *testing.T) {
		run(t, nil, "ROW INSTRUCTIONS", "GLOBAL INSTRUCTIONS",
			"ROW INSTRUCTIONS", "GLOBAL INSTRUCTIONS")
	})
	t.Run("global when no file and no row", func(t *testing.T) {
		run(t, nil, "", "GLOBAL INSTRUCTIONS",
			"GLOBAL INSTRUCTIONS", "")
	})
}

// TestGenerateNotesEmptyLog verifies an empty commit log short-circuits to the
// prose-releaser fallback without ever calling the LLM.
func TestGenerateNotesEmptyLog(t *testing.T) {
	pip, stub, f, _ := fixture(t, nil, nil)

	// Both tags point at the same commit: v1.0.0..v1.1.0 has no commits.
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	gitNoAsk(t, origin, "init", "-q")
	if err := os.WriteFile(filepath.Join(origin, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitNoAsk(t, origin, "add", "-A")
	gitNoAsk(t, origin, "commit", "-q", "-m", "only commit")
	gitNoAsk(t, origin, "tag", "v1.0.0")
	gitNoAsk(t, origin, "tag", "v1.1.0")
	// Swap the fake platform's origin to the empty-range repo.
	f.origin = origin

	notes, err := pip.GenerateNotes(context.Background(), genSpec("v1.1.0", "rel-empty"), Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	if notes != "No changes documented." {
		t.Errorf("notes = %q, want %q", notes, "No changes documented.")
	}
	if stub.calls != 0 {
		t.Errorf("LLM called %d times for empty log, want 0", stub.calls)
	}
}

// TestGenerateNotesDisallowedBaseURL verifies the base-URL format guard on
// the generation path: a saved base URL that the settings PUT guard would
// never have allowed (a path beyond the trailing /v1 form) must be rejected
// before anything is dialed. The check runs before clone and before the LLM
// call, so a sentinel clone error proves the clone was never attempted.
func TestGenerateNotesDisallowedBaseURL(t *testing.T) {
	pip, stub, f, store := fixture(t, nil, nil)
	if err := store.UpsertSettings(db.Settings{BaseURL: "https://100.100.100.200/v1/chat", APIKey: "k"}); err != nil {
		t.Fatal(err)
	}
	// Sentinel: if the guard did not fail fast, the clone would surface this.
	f.cloneErr = errors.New("clone attempted")

	_, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-bad-url"), Options{})
	if err == nil || !strings.Contains(err.Error(), "pipeline: llm base url not allowed") {
		t.Fatalf("err = %v, want 'pipeline: llm base url not allowed'", err)
	}
	if strings.Contains(err.Error(), "clone attempted") {
		t.Errorf("guard did not fail before clone: %v", err)
	}
	if stub.calls != 0 {
		t.Errorf("LLM called %d times, want 0 (generation aborted before dial)", stub.calls)
	}
}

// TestGenerateNotesCommitTypeFilter verifies the full GenerateNotes flow with
// commit-type filtering: included types, excluded types, breaking changes
// (always kept with body), and untyped commits (always kept).
func TestGenerateNotesCommitTypeFilter(t *testing.T) {
	pip, stub, f, store := fixture(t, nil, nil)

	// Build a local repo with mixed commit types
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	gitNoAsk(t, origin, "init", "-q")
	gitNoAsk(t, origin, "commit", "-q", "--allow-empty", "-m", "baseline")
	gitNoAsk(t, origin, "tag", "v1.0.0")
	gitNoAsk(t, origin, "commit", "-q", "--allow-empty", "-m", "feat: add feature a")
	gitNoAsk(t, origin, "commit", "-q", "--allow-empty", "-m", "fix: repair b")
	gitNoAsk(t, origin, "commit", "-q", "--allow-empty", "-m", "chore: tidy c")
	gitNoAsk(t, origin, "commit", "-q", "--allow-empty", "-m", "feat!: break api d", "-m", "BREAKING CHANGE: d broke everything")
	gitNoAsk(t, origin, "commit", "-q", "--allow-empty", "-m", "docs: note e")
	gitNoAsk(t, origin, "tag", "v1.1.0")
	f.origin = origin

	// Global setting: only fix and feat
	if err := store.UpsertSettings(db.Settings{CommitTypes: "fix,feat"}); err != nil {
		t.Fatal(err)
	}

	notes, err := pip.GenerateNotes(context.Background(), genSpec("v1.1.0", "rel-filter"), Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("LLM called %d times, want 1", stub.calls)
	}

	userMsg := stub.user()
	for _, want := range []string{"feat: add feature a", "fix: repair b", "feat!: break api d", "BREAKING CHANGE: d broke everything"} {
		if !strings.Contains(userMsg, want) {
			t.Errorf("user message missing %q; got:\n%s", want, userMsg)
		}
	}
	for _, absent := range []string{"chore: tidy c", "docs: note e"} {
		if strings.Contains(userMsg, absent) {
			t.Errorf("user message should not contain %q; got:\n%s", absent, userMsg)
		}
	}
	// Notes are returned (LLM returned "FLOWING PROSE")
	if notes != "FLOWING PROSE" {
		t.Errorf("notes = %q, want FLOWING PROSE", notes)
	}
}

// TestGenerateNotesFilterEmptyLog verifies that when all commits in the range
// are filtered out, the pipeline returns "No changes documented." without
// calling the LLM.
func TestGenerateNotesFilterEmptyLog(t *testing.T) {
	pip, stub, f, store := fixture(t, nil, nil)

	dir := t.TempDir()
	origin := filepath.Join(dir, "origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	gitNoAsk(t, origin, "init", "-q")
	gitNoAsk(t, origin, "commit", "-q", "--allow-empty", "-m", "chore: only chore")
	gitNoAsk(t, origin, "tag", "v1.0.0")
	gitNoAsk(t, origin, "commit", "-q", "--allow-empty", "-m", "chore: another chore")
	gitNoAsk(t, origin, "tag", "v1.1.0")
	f.origin = origin

	if err := store.UpsertSettings(db.Settings{CommitTypes: "feat"}); err != nil {
		t.Fatal(err)
	}

	notes, err := pip.GenerateNotes(context.Background(), genSpec("v1.1.0", "rel-filtered"), Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	if notes != "No changes documented." {
		t.Errorf("notes = %q, want %q", notes, "No changes documented.")
	}
	if stub.calls != 0 {
		t.Errorf("LLM called %d times when all commits filtered, want 0", stub.calls)
	}
}

// TestGenerateNotesIdempotentAndPublishGuard verifies (a) a human-edited body
// (no marker) is left alone, (b) notes get published + recorded when empty, and
// (c) a second non-forced run returns the stored note without re-generating.
func TestGenerateNotesIdempotentAndPublishGuard(t *testing.T) {
	t.Run("human-edited body is not clobbered", func(t *testing.T) {
		pip, stub, f, _ := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1, Body: "hand written notes"}})
		notes, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-human"), Options{Publish: true})
		if err != nil {
			t.Fatalf("GenerateNotes: %v", err)
		}
		if f.edited != "" {
			t.Errorf("human-edited body was overwritten with %q", f.edited)
		}
		if stub.calls != 1 {
			t.Errorf("LLM calls = %d, want 1", stub.calls)
		}
		_ = notes
	})

	t.Run("publish writes and marks generated, repeat is idempotent", func(t *testing.T) {
		pip, stub, f, store := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1, Body: ""}})

		notes, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-1"), Options{Publish: true})
		if err != nil {
			t.Fatalf("first GenerateNotes: %v", err)
		}
		if !strings.HasSuffix(notes, "FLOWING PROSE") {
			t.Errorf("notes = %q", notes)
		}
		wantBody := "FLOWING PROSE\n<!-- generated by annalist -->"
		if f.edited != wantBody {
			t.Errorf("published body = %q, want %q", f.edited, wantBody)
		}
		gn, err := store.GetGenerated("github", "rel-1")
		if err != nil || gn == nil {
			t.Fatalf("expected generated record, got %v (err=%v)", gn, err)
		}
		if gn.Tag != "v0.2.0" || gn.Owner != "djdembeck" || gn.Repo != "annalist" {
			t.Errorf("generated record = %+v", gn)
		}
		if stub.calls != 1 {
			t.Fatalf("LLM calls = %d after first run, want 1", stub.calls)
		}

		// Second, non-forced run: idempotency returns the stored note, no LLM.
		again, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-1"), Options{Publish: true})
		if err != nil {
			t.Fatalf("second GenerateNotes: %v", err)
		}
		if again != notes {
			t.Errorf("idempotent run returned %q, want %q", again, notes)
		}
		if stub.calls != 1 {
			t.Errorf("LLM called %d times after idempotent run, want 1 (stored note returned)", stub.calls)
		}
	})
}

// pipForPlatformErrors builds a Pipeline wired to a real store but with no
// platform clients, so GenerateNotes exercises platformFor before any cloning.
func pipForPlatformErrors(t *testing.T) *Pipeline {
	t.Helper()
	dir := t.TempDir()
	store, err := db.New(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	cfg := &config.Config{
		Data: config.DataConfig{Dir: filepath.Join(dir, "data")},
		LLM:  config.LLMConfig{Model: "m", Temperature: 0.1},
	}
	return New(cfg, store, nil, nil, nil)
}

// TestGenerateNotesPlatformErrors covers the platformFor error branches, all
// of which short-circuit before any clone, so no git origin is needed.
func TestGenerateNotesPlatformErrors(t *testing.T) {
	pip := pipForPlatformErrors(t)

	t.Run("unknown platform", func(t *testing.T) {
		_, err := pip.GenerateNotes(context.Background(), Spec{Platform: "gitlab", Owner: "o", Repo: "r", ToTag: "v1", ReleaseID: "1"}, Options{})
		if err == nil || !strings.Contains(err.Error(), `unknown platform "gitlab"`) {
			t.Fatalf("err = %v, want unknown-platform error", err)
		}
	})
	t.Run("github not configured", func(t *testing.T) {
		_, err := pip.GenerateNotes(context.Background(), Spec{Platform: "github", Owner: "o", Repo: "r", ToTag: "v0.2.0", ReleaseID: "1"}, Options{})
		if err == nil || !strings.Contains(err.Error(), "platform github is not configured") {
			t.Fatalf("err = %v, want github not-configured error", err)
		}
	})
	t.Run("forgejo not configured", func(t *testing.T) {
		_, err := pip.GenerateNotes(context.Background(), Spec{Platform: "forgejo", Owner: "o", Repo: "r", ToTag: "v1", ReleaseID: "1"}, Options{})
		if err == nil || !strings.Contains(err.Error(), "platform forgejo is not configured") {
			t.Fatalf("err = %v, want forgejo not-configured error", err)
		}
	})
}

// TestGenerateNotesCloneInfoError verifies a CloneInfo failure propagates
// before any clone work is attempted.
func TestGenerateNotesCloneInfoError(t *testing.T) {
	pip, stub, f, _ := fixture(t, nil, nil)
	f.cloneErr = errors.New("boom")

	notes, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-clone"), Options{})
	if err == nil || !strings.Contains(err.Error(), "pipeline: clone info") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want wrapped clone-info error", err)
	}
	if notes != "" {
		t.Errorf("notes = %q, want empty on CloneInfo error", notes)
	}
	if stub.calls != 0 {
		t.Errorf("LLM called %d times, want 0 (clone aborted before LLM)", stub.calls)
	}
}

// TestGenerateNotesReadRepoFileErrorDrop verifies that a ReadRepoFile failure
// does not abort generation: the source treats the in-repo file as optional
// and falls back to the resolved instructions.
func TestGenerateNotesReadRepoFileErrorDrop(t *testing.T) {
	pip, _, f, _ := fixture(t, nil, nil)
	f.readErr = errors.New("file api down")

	notes, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-read"), Options{})
	if err != nil {
		t.Fatalf("GenerateNotes: expected in-repo file error to be dropped, got %v", err)
	}
	if notes != "FLOWING PROSE" {
		t.Errorf("notes = %q, want %q", notes, "FLOWING PROSE")
	}
}

// TestGenerateNotesLLMError verifies an LLM failure propagates with no
// fallback text and nothing is published or recorded.
func TestGenerateNotesLLMError(t *testing.T) {
	stub := newStub("FLOWING PROSE")
	pip, stub2, f, store := fixtureWithStub(t, stub, nil, map[string]*Release{"v0.2.0": {ID: 1, Body: ""}})
	stub2.errStatus = http.StatusInternalServerError

	notes, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "rel-llmfail"), Options{Publish: true})
	if err == nil || !strings.Contains(err.Error(), "llm: unexpected status") {
		t.Fatalf("err = %v, want llm status error", err)
	}
	if notes != "" {
		t.Errorf("notes = %q, want empty (no fallback text)", notes)
	}
	if f.edited != "" {
		t.Errorf("release body was edited despite LLM failure: %q", f.edited)
	}
	if gn, _ := store.GetGenerated("github", "rel-llmfail"); gn != nil {
		t.Errorf("generated record written despite LLM failure: %+v", gn)
	}
}

// TestPublishErrorsAndGuard covers Publish error branches and the
// don't-clobber guard directly.
func TestPublishErrorsAndGuard(t *testing.T) {
	t.Run("fetch release error propagates", func(t *testing.T) {
		pip, _, f, _ := fixture(t, nil, nil) // no releases map -> tag missing
		err := pip.Publish(context.Background(), genSpec("v0.2.0", "rel-fetch"), f, "notes", false)
		if err == nil || !strings.Contains(err.Error(), "pipeline: fetch release") {
			t.Fatalf("err = %v, want fetch-release error", err)
		}
	})
	t.Run("human-edited body not clobbered without force", func(t *testing.T) {
		pip, _, f, _ := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1, Body: "hand written"}})
		err := pip.Publish(context.Background(), genSpec("v0.2.0", "rel-x"), f, "NEW NOTES", false)
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if f.edited != "" {
			t.Errorf("clobbered human-edited body: %q", f.edited)
		}
	})
	t.Run("force overrides don't-clobber guard", func(t *testing.T) {
		pip, _, f, _ := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1, Body: "hand written"}})
		err := pip.Publish(context.Background(), genSpec("v0.2.0", "rel-x"), f, "NEW NOTES", true)
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}
		want := "NEW NOTES\n<!-- generated by annalist -->"
		if f.edited != want {
			t.Errorf("edited body = %q, want %q", f.edited, want)
		}
	})
	t.Run("edit release body error propagates", func(t *testing.T) {
		pip, _, f, _ := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1, Body: ""}})
		f.editErr = errors.New("api down")
		err := pip.Publish(context.Background(), genSpec("v0.2.0", "rel-x"), f, "NEW NOTES", false)
		if err == nil || !strings.Contains(err.Error(), "pipeline: edit release body") {
			t.Fatalf("err = %v, want edit-release-body error", err)
		}
	})
}
