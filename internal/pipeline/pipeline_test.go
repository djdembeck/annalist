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
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func (f *fakePlatform) ListReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	out := make([]Release, 0, len(f.releases))
	for tag, r := range f.releases {
		out = append(out, Release{ID: r.ID, Tag: tag, Body: r.Body, Draft: r.Draft, CreatedAt: r.CreatedAt, PublishedAt: r.PublishedAt})
	}
	return out, nil
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
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
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

// buildOriginThree creates a three-commit origin tagged v0.1.0, v0.2.0, v0.3.0
// so two distinct tags (v0.1.0 and v0.2.0) each have a non-empty commit log —
// required to drive both runs into the LLM call in the concurrency test.
func buildOriginThree(t *testing.T, dir string) string {
	t.Helper()
	origin := filepath.Join(dir, "origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	gitNoAsk(t, origin, "init", "-q")
	commit := func(subject, tag string) {
		if err := os.WriteFile(filepath.Join(origin, "f.txt"), []byte(subject), 0o644); err != nil {
			t.Fatal(err)
		}
		gitNoAsk(t, origin, "add", "-A")
		gitNoAsk(t, origin, "commit", "-q", "-m", subject)
		gitNoAsk(t, origin, "tag", tag)
	}
	commit("first: scaffolding", "v0.1.0")
	commit("feat: feature x", "v0.2.0")
	commit("feat: feature y", "v0.3.0")
	return origin
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

// TestGenerateNotesResolvesReleaseID verifies that a Spec with an empty
// ReleaseID (manual generation) resolves the platform's release ID up front,
// so the generated note is stored under the same "platform:<id>" key the
// webhooks use: a later non-forced run with that explicit ReleaseID returns
// the stored note without re-generating.
func TestGenerateNotesResolvesReleaseID(t *testing.T) {
	pip, stub, _, store := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 77, Body: ""}})

	notes, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", ""), Options{Force: true, Publish: true})
	if err != nil {
		t.Fatalf("GenerateNotes: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("LLM calls = %d, want 1", stub.calls)
	}

	gn, err := store.GetGenerated("github", "github:77")
	if err != nil {
		t.Fatalf("GetGenerated: %v", err)
	}
	if gn == nil {
		t.Fatal("no generated record under github:77; ReleaseID was not auto-resolved")
	}

	// A non-forced run carrying the explicit webhook-style ID hits the store.
	again, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "github:77"), Options{})
	if err != nil {
		t.Fatalf("second GenerateNotes: %v", err)
	}
	if again != notes {
		t.Errorf("idempotent run returned %q, want %q", again, notes)
	}
	if stub.calls != 1 {
		t.Errorf("LLM called %d times after idempotent run, want 1", stub.calls)
	}
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

// TestSortReleases pins the display ordering shared by the github and forgejo
// clients: published releases first (descending publication time), drafts last
// (descending creation time). A nil PublishedAt (draft) falls back to the
// creation time; equal keys keep stable input order.
func TestSortReleases(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	at := func(daysAgo int) *time.Time {
		tt := now.AddDate(0, 0, -daysAgo)
		return &tt
	}
	created := func(daysAgo int) time.Time { return now.AddDate(0, 0, -daysAgo) }

	// Input order deliberately scrambled relative to the wanted order.
	got := []Release{
		{ID: 1, Tag: "draft-old", Draft: true, CreatedAt: created(30)},
		{ID: 2, Tag: "pub-old", PublishedAt: at(20)},
		{ID: 3, Tag: "draft-new", Draft: true, CreatedAt: created(2)},
		{ID: 4, Tag: "pub-new", PublishedAt: at(1)},
		{ID: 5, Tag: "pub-mid", PublishedAt: at(10)},
	}
	SortReleases(got)

	var gotTags []string
	for _, r := range got {
		gotTags = append(gotTags, r.Tag)
	}
	// Published descending by published_at, then drafts descending by created_at.
	want := []string{"pub-new", "pub-mid", "pub-old", "draft-new", "draft-old"}
	if strings.Join(gotTags, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", gotTags, want)
	}
	// The draft must carry a nil PublishedAt and fall back to its creation time.
	if got[3].PublishedAt != nil || !got[3].CreatedAt.Equal(created(2)) {
		t.Errorf("draft-new = %+v, want PublishedAt nil, CreatedAt %v", got[3], created(2))
	}
}

// TestGenerateNotesAutoResolveMissingTag verifies that a manual generation
// (empty ReleaseID) whose tag resolves to no release fails before any clone or
// LLM work: the missing release must be reported as ErrNotFound and nothing
// may be spent or recorded.
func TestGenerateNotesAutoResolveMissingTag(t *testing.T) {
	// Empty releases map -> GetReleaseByTag returns ErrNotFound.
	pip, stub, f, _ := fixture(t, nil, map[string]*Release{})
	// A sentinel clone error proves the clone was never attempted: if the
	// pipeline had proceeded past the missing release, CloneInfo would surface
	// this error instead of the fetch-release error.
	f.cloneErr = errors.New("clone sentinel (must not be reached)")

	_, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", ""), Options{})
	if err == nil || !strings.Contains(err.Error(), "pipeline: fetch release") {
		t.Fatalf("err = %v, want wrapped fetch-release error", err)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want errors.Is(err, ErrNotFound)", err)
	}
	if strings.Contains(err.Error(), "clone sentinel") {
		t.Fatalf("clone was attempted despite a missing release: %v", err)
	}
	if stub.calls != 0 {
		t.Errorf("LLM called %d times, want 0 (missing release must fail before spend)", stub.calls)
	}
	if f.edited != "" {
		t.Errorf("release body was edited despite a missing release: %q", f.edited)
	}
}

// runOrigin is a fakePlatform whose CloneInfo points each run at a fresh copy
// of the shared origin. Concurrent runs of the same origin would otherwise
// collide on the identical derived clone workdir (git clone refuses to write
// into a non-empty dir), so a per-run destination is what lets genuinely
// concurrent generations proceed to the LLM. The copied origins land under
// dir, which the owning test registers as a t.TempDir cleanup.
type runOrigin struct {
	*fakePlatform
	base string
	dir  string
	mu   sync.Mutex
	next int
}

func newRunOrigin(f *fakePlatform, base, dir string) *runOrigin {
	return &runOrigin{fakePlatform: f, base: base, dir: dir}
}

func (r *runOrigin) CloneInfo(ctx context.Context, owner, repo string) (string, string, error) {
	r.mu.Lock()
	idx := r.next
	r.next++
	r.mu.Unlock()
	dst := filepath.Join(r.dir, fmt.Sprintf("origin-%d", idx))
	if err := copyDir(r.base, dst); err != nil {
		return "", "", fmt.Errorf("copy origin: %w", err)
	}
	return dst, "Bearer test-token", nil
}

// copyDir recursively copies a directory tree. git stores .git and some of its
// internals with restrictive modes (e.g. 0500 lock dirs) that a subsequent
// clone into the copy would refuse to write; the copy is a disposable test
// fixture, so dirs and files are normalized to writable modes.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// blockingStub is an LLM stub that counts concurrent in-flight chat requests
// and holds each one open (no response written) until the test releases it.
// The LLM call sits *behind* the per-(platform, releaseID) inflight lock, so
// the number of requests held open at once is exactly the number of distinct
// keys allowed to proceed in parallel. `peak` records the maximum concurrent
// in-flight count the server ever observed, which a test can assert on.
type blockingStub struct {
	srv      *httptest.Server
	inFlight atomic.Int32  // requests currently held at the gate
	peak     atomic.Int32  // max in-flight count ever observed
	total    atomic.Int32  // total requests served
	release  chan struct{} // closed by the test to let the held requests finish
}

func newBlockingStub() *blockingStub {
	b := &blockingStub{release: make(chan struct{})}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		b.total.Add(1)
		n := b.inFlight.Add(1)
		// Record the peak concurrent in-flight count.
		for {
			p := b.peak.Load()
			if n <= p || b.peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-b.release // hold the response open until the test releases this stub
		b.inFlight.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"FLOWING PROSE"}}]}`)
	}))
	return b
}

func (b *blockingStub) close() { b.srv.Close() }

func (b *blockingStub) inFlightNow() int { return int(b.inFlight.Load()) }

func (b *blockingStub) peakNow() int { return int(b.peak.Load()) }

func (b *blockingStub) totalNow() int { return int(b.total.Load()) }

// TestGenerateNotesInflightKeyIsResolved pins the resolve-before-lock reorder:
// a manual run (empty ReleaseID) must acquire the inflight lock on the
// *resolved* (platform:releaseID) key, not a platform-wide key.
//
// (a) Two runs for DIFFERENT tags must not block each other on the lock. If
// the lock were platform-wide, the first run would hold the whole platform and
// the second could not even proceed while the first is in flight, so at most
// one run would reach the LLM at a time. The gate proves both distinct runs
// reach the LLM together, which is only possible if each resolved a distinct
// key.
//
// (b) Two runs that resolve to the SAME release must serialize on the shared
// key: the second cannot enter the LLM call while the first is in it, so the
// peak concurrent LLM calls for the same key stays at 1 (no double spend).
func TestGenerateNotesInflightKeyIsResolved(t *testing.T) {
	t.Run("distinct releases proceed concurrently", func(t *testing.T) {
		b := newBlockingStub()
		t.Cleanup(b.close)
		pip, _, f, _ := fixture(t, nil, map[string]*Release{
			"v0.1.0": {ID: 1, Body: ""},
			"v0.2.0": {ID: 2, Body: ""},
		})
		// A 3-commit origin so both distinct tags yield a non-empty log and
		// drive their run into the LLM call.
		f.origin = buildOriginThree(t, t.TempDir())
		pip.GitHub = newRunOrigin(f, f.origin, t.TempDir())
		// Point the LLM client at the blocking stub (the fixture stub is unused).
		pip.Cfg.LLM.BaseURL = b.srv.URL

		var wg sync.WaitGroup
		for _, tag := range []string{"v0.1.0", "v0.2.0"} {
			wg.Add(1)
			go func(tag string) {
				defer wg.Done()
				_, _ = pip.GenerateNotes(context.Background(), genSpec(tag, ""), Options{Force: true})
			}(tag)
		}
		// Both distinct-release runs must reach the LLM together (peak == 2).
		// This is the reorder discriminator: if the inflight lock were acquired
		// BEFORE the ReleaseID was resolved (a platform-wide key), the second
		// run could not even pass the lock while the first holds the whole
		// platform, so at most one would be in flight.
		waitFor(t, func() bool { return b.peakNow() == 2 }, "both distinct-release runs in flight together (no platform-wide lock)")
		close(b.release)
		wg.Wait()
	})

	t.Run("same release serializes on the shared key", func(t *testing.T) {
		b := newBlockingStub()
		t.Cleanup(b.close)
		pip, _, f, _ := fixture(t, nil, map[string]*Release{
			"v0.2.0": {ID: 1, Body: ""},
		})
		pip.GitHub = newRunOrigin(f, f.origin, t.TempDir())
		pip.Cfg.LLM.BaseURL = b.srv.URL

		done := make(chan struct{})
		go func() {
			_, _ = pip.GenerateNotes(context.Background(), genSpec("v0.2.0", ""), Options{Force: true})
			close(done)
		}()
		// The first run holds its resolved key and is in the LLM call.
		waitFor(t, func() bool { return b.inFlightNow() == 1 }, "first same-key run in the LLM call")

		// A second run for the SAME release must block on the shared inflight
		// key, so the second cannot enter the LLM call while the first is in it
		// — at most one same-key run may be in flight at once (no double spend).
		done2 := make(chan struct{})
		go func() {
			_, _ = pip.GenerateNotes(context.Background(), genSpec("v0.2.0", ""), Options{Force: true})
			close(done2)
		}()
		// Release the held LLM calls; both runs finish in turn.
		close(b.release)
		<-done
		<-done2
		// Both same-key runs generated (each spent the LLM once), but never ran
		// concurrently: the peak in-flight count stayed at 1.
		if got := b.totalNow(); got != 2 {
			t.Errorf("total LLM calls = %d, want 2 (each same-key run must generate)", got)
		}
		if got := b.peakNow(); got != 1 {
			t.Errorf("peak concurrent LLM calls = %d, want 1 (same-key runs must serialize)", got)
		}
	})
}

// waitFor polls until f() is true or the test's generous deadline elapses,
// failing the test with msg if it never does.
func waitFor(t *testing.T, f func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + msg)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
