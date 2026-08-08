package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/engine"
	"github.com/djdembeck/annalist/internal/pipeline"
	"github.com/djdembeck/annalist/internal/version"
)

// fakePip is a controllable pipService double: it returns a fixed Resolved and
// either a recorded notes string or a forced error, and records every
// GenerateNotes call for assertions.
type fakePip struct {
	notes      string
	err        error
	resolveErr error
	eff        engine.Resolved
	calls      []pipeline.Spec
	opts       []pipeline.Options
}

func newFakePip() *fakePip {
	return &fakePip{
		notes: "generated notes",
		eff:   engine.Resolved{Tone: "t0", Instructions: "i0", Model: "m0", Temperature: 0.5},
	}
}

func (f *fakePip) Resolve(ctx context.Context, platform, owner, repo string) (bool, engine.Resolved, error) {
	if f.resolveErr != nil {
		return false, engine.Resolved{}, f.resolveErr
	}
	return true, f.eff, nil
}

func (f *fakePip) GenerateNotes(ctx context.Context, spec pipeline.Spec, opts pipeline.Options) (string, error) {
	f.calls = append(f.calls, spec)
	f.opts = append(f.opts, opts)
	if f.err != nil {
		return "", f.err
	}
	return f.notes, nil
}

// fakeClient is a controllable ghClient/fjClient double.
type fakeClient struct {
	repos          []pipeline.OwnerRepo
	err            error
	fileContents   map[string]string
}

func (f *fakeClient) WebhookHandler(p *pipeline.Pipeline) http.Handler {
	return http.NotFoundHandler()
}

func (f *fakeClient) ListRepos(ctx context.Context) ([]pipeline.OwnerRepo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.repos, nil
}

func (f *fakeClient) ReadRepoFile(ctx context.Context, owner, repo, path string) (string, error) {
	content, ok := f.fileContents[path]
	if ok {
		return content, nil
	}
	return "", fmt.Errorf("file not found: %s", path)
}

// testAPI builds an api with a real temp-db store and a fake pip.
func testAPI(t *testing.T, cfg *config.Config) (*api, *fakePip, *db.Store) {
	t.Helper()
	store, err := db.New(t.TempDir())
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	pip := newFakePip()
	return &api{cfg: cfg, store: store, pip: pip}, pip, store
}

// newReq builds a request, optionally injecting chi URL params.
func newReq(method, target string, body string, params map[string]string) *http.Request {
	r := httptest.NewRequest(method, "http://test"+target, strings.NewReader(body))
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}
	return r
}

func do(h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func TestValidPlatform(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"github", true},
		{"forgejo", true},
		{"gitlab", false},
		{"", false},
		{"GITHUB", false},
	}
	for _, tc := range cases {
		if got := validPlatform(tc.in); got != tc.want {
			t.Errorf("validPlatform(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestDecodePresenceMap(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    map[string]json.RawMessage
		wantErr bool
	}{
		{
			name: "object preserves keys",
			body: `{"tone": "a", "temperature": null}`,
			want: map[string]json.RawMessage{
				"tone":        json.RawMessage(`"a"`),
				"temperature": json.RawMessage(`null`),
			},
		},
		{
			name: "empty body is an empty object",
			body: "",
			want: map[string]json.RawMessage{},
		},
		{
			name:    "non-object body errors",
			body:    `[1,2,3]`,
			wantErr: true,
		},
		{
			name:    "invalid json errors",
			body:    `{`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://test/", strings.NewReader(tc.body))
			got, err := decodePresenceMap(r)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d keys %v, want %d", len(got), got, len(tc.want))
			}
			for k, v := range tc.want {
				if string(got[k]) != string(v) {
					t.Errorf("key %q = %s, want %s", k, got[k], v)
				}
			}
		})
	}
}

func TestHandleHealth(t *testing.T) {
	a, _, _ := testAPI(t, &config.Config{})
	w := do(a.handleHealth, httptest.NewRequest(http.MethodGet, "http://test/api/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %q, want ok", body["status"])
	}
	if body["version"] != version.Version {
		t.Errorf("version field = %q, want %q", body["version"], version.Version)
	}
}

func TestAdminAuth(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	run := func(t *testing.T, a *api, header string) int {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "http://test/api/status", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		return do(a.adminAuth(ok).ServeHTTP, r).Code
	}

	t.Run("token not configured", func(t *testing.T) {
		a, _, _ := testAPI(t, &config.Config{})
		if got := run(t, a, "Bearer anything"); got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	t.Run("missing header", func(t *testing.T) {
		a, _, _ := testAPI(t, &config.Config{Admin: config.AdminConfig{Token: "secret"}})
		if got := run(t, a, ""); got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	t.Run("missing bearer prefix", func(t *testing.T) {
		a, _, _ := testAPI(t, &config.Config{Admin: config.AdminConfig{Token: "secret"}})
		if got := run(t, a, "secret"); got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		a, _, _ := testAPI(t, &config.Config{Admin: config.AdminConfig{Token: "secret"}})
		if got := run(t, a, "Bearer wrong"); got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})

	t.Run("correct token", func(t *testing.T) {
		a, _, _ := testAPI(t, &config.Config{Admin: config.AdminConfig{Token: "secret"}})
		if got := run(t, a, "Bearer secret"); got != http.StatusOK {
			t.Errorf("status = %d, want 200", got)
		}
	})
}

func TestHandleStatus(t *testing.T) {
	cases := []struct {
		name       string
		cfg        *config.Config
		wantGithub bool
		wantForgej bool
	}{
		{
			name:       "both enabled",
			cfg:        &config.Config{GitHub: config.GitHubConfig{WebhookSecret: "s"}, Forgejo: config.ForgejoConfig{Token: "t"}},
			wantGithub: true,
			wantForgej: true,
		},
		{
			name:       "none enabled",
			cfg:        &config.Config{},
			wantGithub: false,
			wantForgej: false,
		},
		{
			name:       "github app enabled",
			cfg:        &config.Config{GitHub: config.GitHubConfig{AppID: 1, AppPrivateKeyFile: "k"}},
			wantGithub: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := testAPI(t, tc.cfg)
			w := do(a.handleStatus, httptest.NewRequest(http.MethodGet, "http://test/api/status", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("bad json: %v", err)
			}
			if body["github"] != tc.wantGithub {
				t.Errorf("github = %v, want %v", body["github"], tc.wantGithub)
			}
			if body["forgejo"] != tc.wantForgej {
				t.Errorf("forgejo = %v, want %v", body["forgejo"], tc.wantForgej)
			}
			if body["admin"] != true {
				t.Errorf("admin = %v, want true", body["admin"])
			}
		})
	}
}

func TestHandleListReposManagedOnly(t *testing.T) {
	cfg := &config.Config{GitHub: config.GitHubConfig{WebhookSecret: "s"}}
	a, _, store := testAPI(t, cfg)
	// The gh client reports two repos, but only annalist is managed.
	a.gh = &fakeClient{repos: []pipeline.OwnerRepo{
		{Owner: "djdembeck", Repo: "annalist"},
		{Owner: "someone", Repo: "other"},
	}}
	if err := store.UpsertRepoSettings(db.RepoSetting{
		Platform: "github", Owner: "djdembeck", Repo: "annalist", Enabled: true, Trigger: "auto",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(a.handleListRepos, httptest.NewRequest(http.MethodGet, "http://test/api/repos", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var items []repoItemResp
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (managed only): %+v", len(items), items)
	}
	it := items[0]
	if it.Platform != "github" || it.Owner != "djdembeck" || it.Repo != "annalist" {
		t.Errorf("item mismatch: %+v", it)
	}
	if !it.Enabled {
		t.Errorf("item.Enabled = false, want true")
	}
	if it.Effective.Model != "m0" {
		t.Errorf("effective model = %q, want fake resolve model m0", it.Effective.Model)
	}
	if it.Effective.Source != "global" {
		t.Errorf("effective source = %q, want global (no repo instructions set)", it.Effective.Source)
	}
}

func TestHandleListReposResolveError(t *testing.T) {
	a, pip, store := testAPI(t, &config.Config{})
	pip.resolveErr = errors.New("boom")
	if err := store.UpsertRepoSettings(db.RepoSetting{
		Platform: "github", Owner: "o", Repo: "r", Enabled: true, Trigger: "auto",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := do(a.handleListRepos, httptest.NewRequest(http.MethodGet, "http://test/api/repos", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestHandleListReposMultipleReposParallel(t *testing.T) {
	cfg := &config.Config{GitHub: config.GitHubConfig{WebhookSecret: "s"}}
	a, pip, store := testAPI(t, cfg)
	pip.eff.Tone = "t0"
	pip.eff.Instructions = "resolved-instructions"
	pip.eff.Model = "m0"
	a.gh = &fakeClient{
		repos: []pipeline.OwnerRepo{
			{Owner: "djdembeck", Repo: "annalist"},
			{Owner: "djdembeck", Repo: "other-repo"},
			{Owner: "djdembeck", Repo: "third-repo"},
		},
	}
	if err := store.UpsertRepoSettings(db.RepoSetting{
		Platform: "github", Owner: "djdembeck", Repo: "annalist", Enabled: true, Trigger: "auto",
	}); err != nil {
		t.Fatalf("seed annalist: %v", err)
	}
	if err := store.UpsertRepoSettings(db.RepoSetting{
		Platform: "github", Owner: "djdembeck", Repo: "other-repo", Enabled: true, Trigger: "manual",
	}); err != nil {
		t.Fatalf("seed other-repo: %v", err)
	}
	if err := store.UpsertRepoSettings(db.RepoSetting{
		Platform: "github", Owner: "djdembeck", Repo: "third-repo", Enabled: false, Trigger: "auto", Instructions: "CUSTOM ROW",
	}); err != nil {
		t.Fatalf("seed third-repo: %v", err)
	}

	w := do(a.handleListRepos, httptest.NewRequest(http.MethodGet, "http://test/api/repos", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var items []repoItemResp
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}

	// Build a map keyed by repo name for stable assertions regardless of parallel ordering.
	itemsByRepo := make(map[string]repoItemResp, len(items))
	for _, it := range items {
		itemsByRepo[it.Repo] = it
	}

	// Annalist — instructions from Resolve().
	annalist := itemsByRepo["annalist"]
	if annalist.Effective != (effective{
		Tone: "t0", Model: "m0", Temperature: 0.5,
		Instructions: "resolved-instructions", Source: "global",
	}) {
		t.Errorf("annalist effective = %+v, want resolved source", annalist.Effective)
	}

	// Other-repo — same resolved instructions.
	other := itemsByRepo["other-repo"]
	if other.Effective != (effective{
		Tone: "t0", Model: "m0", Temperature: 0.5,
		Instructions: "resolved-instructions", Source: "global",
	}) {
		t.Errorf("other-repo effective = %+v, want resolved source", other.Effective)
	}

	// Third-repo has repo-level instructions — source is "repo".
	third := itemsByRepo["third-repo"]
	if third.Effective != (effective{
		Tone: "t0", Model: "m0", Temperature: 0.5,
		Instructions: "resolved-instructions", Source: "repo",
	}) {
		t.Errorf("third-repo effective = %+v, want repo source", third.Effective)
	}
	if third.Enabled {
		t.Errorf("third-repo.Enabled = true, want false")
	}
	if third.Trigger != "auto" {
		t.Errorf("third-repo.Trigger = %q, want auto", third.Trigger)
	}
}

func TestHandleListAvailableRepos(t *testing.T) {
	cfg := &config.Config{GitHub: config.GitHubConfig{WebhookSecret: "s"}, Forgejo: config.ForgejoConfig{Token: "t"}}
	a, _, store := testAPI(t, cfg)
	a.gh = &fakeClient{repos: []pipeline.OwnerRepo{
		{Owner: "djdembeck", Repo: "annalist", OwnNamespace: true, UpdatedAt: time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC), PushedAt: time.Date(2024, 3, 2, 10, 0, 0, 0, time.UTC)},
		{Owner: "someone", Repo: "other", Fork: true, OwnNamespace: false, UpdatedAt: time.Date(2023, 5, 15, 20, 30, 0, 0, time.UTC), PushedAt: time.Date(2023, 5, 16, 20, 30, 0, 0, time.UTC)},
	}}
	a.fj = &fakeClient{repos: []pipeline.OwnerRepo{
		{Owner: "fjuser", Repo: "released", OwnNamespace: true, UpdatedAt: time.Date(2024, 7, 4, 12, 45, 0, 0, time.UTC), PushedAt: time.Date(2024, 7, 4, 10, 0, 0, 0, time.UTC)},
	}}
	// annalist is already managed, so it must be excluded from available.
	if err := store.UpsertRepoSettings(db.RepoSetting{
		Platform: "github", Owner: "djdembeck", Repo: "annalist", Enabled: true, Trigger: "auto",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(a.handleListAvailableRepos, httptest.NewRequest(http.MethodGet, "http://test/api/repos/available", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var avail []availableRepo
	if err := json.Unmarshal(w.Body.Bytes(), &avail); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	want := map[string]bool{
		"github/someone/other":    true,
		"forgejo/fjuser/released": true,
	}
	if len(avail) != len(want) {
		t.Fatalf("got %d repos %+v, want %d", len(avail), avail, len(want))
	}
	for _, ar := range avail {
		key := ar.Platform + "/" + ar.Owner + "/" + ar.Repo
		if !want[key] {
			t.Errorf("unexpected available repo: %+v", ar)
		}
		if ar.Platform == "github" && ar.Repo == "annalist" {
			t.Errorf("managed repo leaked into available: %+v", ar)
		}
		// The fork flag must be present in the JSON for every repo.
		if !strings.Contains(w.Body.String(), `"fork":`) {
			t.Fatalf("fork field missing from JSON: %s", w.Body.String())
		}
		// The ownNamespace flag must be present in the JSON for every repo.
		if !strings.Contains(w.Body.String(), `"ownNamespace":`) {
			t.Fatalf("ownNamespace field missing from JSON: %s", w.Body.String())
		}
		// The updatedAt field must be present in the JSON for every repo.
		if !strings.Contains(w.Body.String(), `"updatedAt":`) {
			t.Fatalf("updatedAt field missing from JSON: %s", w.Body.String())
		}
		// The pushedAt field must be present in the JSON for every repo.
		if !strings.Contains(w.Body.String(), `"pushedAt":`) {
			t.Fatalf("pushedAt field missing from JSON: %s", w.Body.String())
		}
	}
	// Each available repo must carry its last-updated timestamp.
	for _, ar := range avail {
		if ar.UpdatedAt.IsZero() {
			t.Errorf("available repo %s has zero updatedAt: %+v", ar.Platform+"/"+ar.Owner+"/"+ar.Repo, ar)
		}
		if ar.PushedAt.IsZero() {
			t.Errorf("available repo %s has zero pushedAt: %+v", ar.Platform+"/"+ar.Owner+"/"+ar.Repo, ar)
		}
	}
	// forgejo/fjuser/released is owned by the authenticated user's namespace, so
	// it must carry ownNamespace=true; the external fork must carry false.
	var ownNs bool
	var nonOwn bool
	for _, ar := range avail {
		if ar.Platform == "forgejo" && ar.Repo == "released" && ar.OwnNamespace {
			ownNs = true
		}
		if ar.Platform == "github" && ar.Repo == "other" && !ar.OwnNamespace {
			nonOwn = true
		}
	}
	if !ownNs {
		t.Errorf("forgejo/fjuser/released should be reported as own namespace, got %+v", avail)
	}
	if !nonOwn {
		t.Errorf("github/someone/other should not be reported as own namespace, got %+v", avail)
	}
	// The one repo that is a fork must carry fork=true.
	var forked bool
	for _, ar := range avail {
		if ar.Platform == "github" && ar.Repo == "other" && ar.Fork {
			forked = true
		}
	}
	if !forked {
		t.Errorf("github/someone/other should be reported as a fork, got %+v", avail)
	}
}

func TestHandleListAvailableReposError(t *testing.T) {
	cfg := &config.Config{GitHub: config.GitHubConfig{WebhookSecret: "s"}}
	a, _, _ := testAPI(t, cfg)
	a.gh = &fakeClient{err: context.DeadlineExceeded}
	w := do(a.handleListAvailableRepos, httptest.NewRequest(http.MethodGet, "http://test/api/repos/available", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "github:") {
		t.Errorf("error body missing platform prefix: %s", w.Body.String())
	}
}

func TestHandleListAvailableReposDisabledPlatformSkipped(t *testing.T) {
	// Only GitHub is enabled; Forgejo's client must never be consulted.
	a, _, _ := testAPI(t, &config.Config{GitHub: config.GitHubConfig{WebhookSecret: "s"}})
	a.gh = &fakeClient{repos: []pipeline.OwnerRepo{{Owner: "o", Repo: "r"}}}
	a.fj = &fakeClient{repos: []pipeline.OwnerRepo{{Owner: "should", Repo: "not-appear"}}}

	w := do(a.handleListAvailableRepos, httptest.NewRequest(http.MethodGet, "http://test/api/repos/available", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var avail []availableRepo
	if err := json.Unmarshal(w.Body.Bytes(), &avail); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(avail) != 1 || avail[0].Repo != "r" {
		t.Fatalf("avail = %+v, want only the github repo", avail)
	}
}

func TestHandleAddRepo(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Token: "secret"}}
	a, _, store := testAPI(t, cfg)

	r := newReq(http.MethodPost, "/api/repos", `{"platform":"github","owner":"djdembeck","repo":"annalist"}`, nil)
	w := do(a.handleAddRepo, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var item repoItemResp
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if item.Platform != "github" || item.Owner != "djdembeck" || item.Repo != "annalist" {
		t.Errorf("item mismatch: %+v", item)
	}
	if !item.Enabled {
		t.Errorf("item.Enabled = false, want true")
	}
	if item.Trigger != "auto" {
		t.Errorf("item.Trigger = %q, want auto", item.Trigger)
	}
	if item.Effective.Model != "m0" {
		t.Errorf("effective model = %q, want fake resolve model m0", item.Effective.Model)
	}

	// Verify persisted in the store.
	row, err := store.GetRepoSettings("github", "djdembeck", "annalist")
	if err != nil {
		t.Fatalf("GetRepoSettings: %v", err)
	}
	if row == nil {
		t.Fatal("expected stored row, got nil")
	}
	if !row.Enabled || row.Trigger != "auto" {
		t.Errorf("stored row = %+v, want enabled+auto", row)
	}
}

func TestHandleAddRepoInvalidPlatform(t *testing.T) {
	a, _, _ := testAPI(t, &config.Config{})
	r := newReq(http.MethodPost, "/api/repos", `{"platform":"gitlab","owner":"o","repo":"r"}`, nil)
	w := do(a.handleAddRepo, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleAddRepoMissingOwnerOrRepo(t *testing.T) {
	a, _, _ := testAPI(t, &config.Config{})
	r := newReq(http.MethodPost, "/api/repos", `{"platform":"github","owner":"","repo":"r"}`, nil)
	w := do(a.handleAddRepo, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleAddRepoBadJSON(t *testing.T) {
	a, _, _ := testAPI(t, &config.Config{})
	r := newReq(http.MethodPost, "/api/repos", `not json`, nil)
	w := do(a.handleAddRepo, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandlePutRepoSettingsRoundTrip(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Token: "secret"}}
	a, pip, store := testAPI(t, cfg)
	_ = pip

	r := newReq(http.MethodPut, "/api/repos/github/djdembeck/annalist/settings",
		`{"tone":"terse","model":"m9","temperature":0.25,"enabled":false,"trigger":"release","instructions":null}`,
		map[string]string{"platform": "github", "owner": "djdembeck", "repo": "annalist"})
	w := do(a.handlePutRepoSettings, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var item repoItemResp
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if item.Tone != "terse" || item.Model != "m9" || item.Trigger != "release" || item.Enabled != false {
		t.Errorf("response item mismatch: %+v", item)
	}
	if item.Temperature == nil || *item.Temperature != 0.25 {
		t.Errorf("temperature = %v, want 0.25", item.Temperature)
	}
	if item.Instructions != "" {
		t.Errorf("instructions = %q, want cleared by explicit null", item.Instructions)
	}

	// Verify it persisted in the store.
	row, err := store.GetRepoSettings("github", "djdembeck", "annalist")
	if err != nil {
		t.Fatalf("GetRepoSettings: %v", err)
	}
	if row == nil {
		t.Fatal("expected stored row, got nil")
	}
	if row.Tone != "terse" || row.Model != "m9" || row.Enabled != false || row.Trigger != "release" {
		t.Errorf("stored row mismatch: %+v", row)
	}
	if row.Temperature == nil || *row.Temperature != 0.25 {
		t.Errorf("stored temperature = %v, want 0.25", row.Temperature)
	}
}

func TestHandlePutRepoSettingsInvalidPlatform(t *testing.T) {
	a, _, _ := testAPI(t, &config.Config{})
	r := newReq(http.MethodPut, "/api/repos/gitlab/o/r/settings", `{}`,
		map[string]string{"platform": "gitlab", "owner": "o", "repo": "r"})
	w := do(a.handlePutRepoSettings, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandlePutRepoSettingsBadJSON(t *testing.T) {
	a, _, _ := testAPI(t, &config.Config{})
	r := newReq(http.MethodPut, "/api/repos/github/o/r/settings", `not json`,
		map[string]string{"platform": "github", "owner": "o", "repo": "r"})
	w := do(a.handlePutRepoSettings, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleGetSettingsDefaults(t *testing.T) {
	a, _, _ := testAPI(t, &config.Config{Admin: config.AdminConfig{Token: "secret"}, LLM: config.LLMConfig{BaseURL: "http://llm", Model: "m99"}})
	w := do(a.handleGetSettings, httptest.NewRequest(http.MethodGet, "http://test/api/settings", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if body["llm"] == nil {
		t.Fatal("expected llm block")
	}
	llm := body["llm"].(map[string]any)
	if llm["base_url"] != "http://llm" || llm["model"] != "m99" {
		t.Errorf("llm block = %v", llm)
	}
}

func TestHandlePutSettingsRoundTrip(t *testing.T) {
	a, _, store := testAPI(t, &config.Config{})
	r := newReq(http.MethodPut, "/api/settings",
		`{"tone":"cheerful","temperature":0.9,"model":null}`,
		nil)
	w := do(a.handlePutSettings, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if body["tone"] != "cheerful" || body["temperature"] != 0.9 {
		t.Errorf("response body = %v", body)
	}
	if body["model"] != "" {
		t.Errorf("model = %v, want cleared (empty)", body["model"])
	}

	s, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.Tone != "cheerful" || s.Model != "" {
		t.Errorf("stored settings = %+v", s)
	}
	if s.Temperature == nil || *s.Temperature != 0.9 {
		t.Errorf("stored temperature = %v, want 0.9", s.Temperature)
	}
}

func TestHandlePutSettingsBadJSON(t *testing.T) {
	a, _, _ := testAPI(t, &config.Config{})
	r := newReq(http.MethodPut, "/api/settings", `{`, nil)
	w := do(a.handlePutSettings, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleGenerate(t *testing.T) {
	a, pip, _ := testAPI(t, &config.Config{})

	t.Run("success", func(t *testing.T) {
		r := newReq(http.MethodPost, "/api/repos/github/o/r/generate", `{"to_tag":"v1.0.0","from_tag":"v0.9.0","force":true,"publish":true}`,
			map[string]string{"platform": "github", "owner": "o", "repo": "r"})
		w := do(a.handleGenerate, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		if body["notes"] != "generated notes" {
			t.Errorf("notes = %v", body["notes"])
		}
		if body["published"] != true {
			t.Errorf("published = %v, want true", body["published"])
		}
		if len(pip.calls) != 1 {
			t.Fatalf("GenerateNotes called %d times, want 1", len(pip.calls))
		}
		spec := pip.calls[0]
		if spec.ToTag != "v1.0.0" || spec.FromTag != "v0.9.0" || spec.Platform != "github" {
			t.Errorf("spec = %+v", spec)
		}
		if want := "manual:github/o/r@v1.0.0"; spec.ReleaseID != want {
			t.Errorf("release_id = %q, want %q", spec.ReleaseID, want)
		}
		if !pip.opts[0].Force || !pip.opts[0].Publish {
			t.Errorf("opts = %+v, want force+publish", pip.opts[0])
		}
	})

	t.Run("publish omitted defaults false", func(t *testing.T) {
		pip.calls = nil
		pip.opts = nil
		r := newReq(http.MethodPost, "/api/repos/github/o/r/generate", `{"to_tag":"v1.0.0"}`,
			map[string]string{"platform": "github", "owner": "o", "repo": "r"})
		w := do(a.handleGenerate, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		if body["published"] != false {
			t.Errorf("published = %v, want false", body["published"])
		}
		if len(pip.opts) != 1 || pip.opts[0].Publish || pip.opts[0].Force {
			t.Errorf("opts = %+v, want preview-only", pip.opts)
		}
	})

	t.Run("publish false", func(t *testing.T) {
		pip.calls = nil
		pip.opts = nil
		r := newReq(http.MethodPost, "/api/repos/github/o/r/generate", `{"to_tag":"v1.0.0","publish":false}`,
			map[string]string{"platform": "github", "owner": "o", "repo": "r"})
		w := do(a.handleGenerate, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		if body["notes"] != "generated notes" {
			t.Errorf("notes = %v", body["notes"])
		}
		if body["published"] != false {
			t.Errorf("published = %v, want false", body["published"])
		}
		if len(pip.calls) != 1 || pip.opts[0].Publish || pip.opts[0].Force {
			t.Errorf("opts = %+v, want publish=false, force=false", pip.opts[0])
		}
	})

	t.Run("pipeline error", func(t *testing.T) {
		pip.err = context.DeadlineExceeded
		r := newReq(http.MethodPost, "/api/repos/github/o/r/generate", `{"to_tag":"v1.0.0"}`,
			map[string]string{"platform": "github", "owner": "o", "repo": "r"})
		w := do(a.handleGenerate, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("unknown platform", func(t *testing.T) {
		pip.err = nil
		r := newReq(http.MethodPost, "/api/repos/gitlab/o/r/generate", `{"to_tag":"v1.0.0"}`,
			map[string]string{"platform": "gitlab", "owner": "o", "repo": "r"})
		w := do(a.handleGenerate, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing to_tag", func(t *testing.T) {
		r := newReq(http.MethodPost, "/api/repos/github/o/r/generate", `{"from_tag":"v1.0.0"}`,
			map[string]string{"platform": "github", "owner": "o", "repo": "r"})
		w := do(a.handleGenerate, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("bad body json", func(t *testing.T) {
		r := newReq(http.MethodPost, "/api/repos/github/o/r/generate", `{`,
			map[string]string{"platform": "github", "owner": "o", "repo": "r"})
		w := do(a.handleGenerate, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

// TestSpaThroughRouter exercises the mounted New() handler: unknown /api/* and
// /webhooks/* paths fall through to a hard 404, and the open health route is
// still served as JSON. (The SPA index-fallback behavior is webui-tagged and
// is covered by web/embed_test.go.)
func TestSpaThroughRouter(t *testing.T) {
	store, err := db.New(t.TempDir())
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	// No platforms enabled, so none of gh/fj/pip are exercised by New().
	cfg := &config.Config{Admin: config.AdminConfig{Token: "secret"}}
	h := New(cfg, store, nil, nil, nil, nil)

	t.Run("unknown api path is not spa fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://test/api/does-not-exist", nil)
		req.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("unknown webhooks path is not spa fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://test/webhooks/nope", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("open health route returns json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://test/api/health", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Errorf("content-type = %q, want json", w.Header().Get("Content-Type"))
		}
	})
}

func TestHandleInRepoInstructionsPresent(t *testing.T) {
	cfg := &config.Config{GitHub: config.GitHubConfig{WebhookSecret: "s"}}
	a, _, _ := testAPI(t, cfg)
	a.gh = &fakeClient{
		fileContents: map[string]string{
			".github/release-notes-instructions.md": "IN-REPO PROMPT",
		},
	}

	req := newReq(http.MethodGet, "/api/repos/github/djdembeck/annalist/in-repo-instructions", "", map[string]string{
		"platform": "github",
		"owner":    "djdembeck",
		"repo":     "annalist",
	})
	w := do(a.handleInRepoInstructions, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if body["instructions"] != "IN-REPO PROMPT" {
		t.Errorf("instructions = %q, want IN-REPO PROMPT", body["instructions"])
	}
}

func TestHandleInRepoInstructionsMissing(t *testing.T) {
	cfg := &config.Config{GitHub: config.GitHubConfig{WebhookSecret: "s"}}
	a, _, _ := testAPI(t, cfg)
	a.gh = &fakeClient{
		fileContents: map[string]string{}, // empty — file not found
	}

	req := newReq(http.MethodGet, "/api/repos/github/djdembeck/annalist/in-repo-instructions", "", map[string]string{
		"platform": "github",
		"owner":    "djdembeck",
		"repo":     "annalist",
	})
	w := do(a.handleInRepoInstructions, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("body = %v, want empty object {}", body)
	}
}

// Compile-time check that the seam stays behavior-preserving: the concrete
// pipeline must still satisfy pipService.
var _ pipService = (*pipeline.Pipeline)(nil)
