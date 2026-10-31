package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/llm"
	"github.com/djdembeck/annalist/internal/pipeline"
)

func openTestDB(t *testing.T, dataDir string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestRunHistoryAuthFiltersAndLimits(t *testing.T) {
	cfg := &config.Config{}
	cfg.Admin.Token = "secret"
	_, _, store := testAPI(t, cfg, nil)
	for range 105 {
		if _, err := store.StartRun(db.GenerationRun{Platform: "github", Owner: "o", Repo: "busy", Source: "manual", ToTag: "v1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.StartRun(db.GenerationRun{Platform: "forgejo", Owner: "other", Repo: "one", Source: "webhook", ToTag: "v2"}); err != nil {
		t.Fatal(err)
	}
	h := New(cfg, store, nil, nil, nil, nil)
	for _, tc := range []struct {
		query, token  string
		status, count int
	}{
		{"", "", 401, 0},
		{"", "secret", 200, 20},
		{"?limit=100", "secret", 200, 100},
		{"?limit=1", "secret", 200, 1},
		{"?limit=0", "secret", 400, 0},
		{"?limit=-1", "secret", 400, 0},
		{"?limit=101", "secret", 400, 0},
		{"?limit=no", "secret", 400, 0},
		{"?limit=", "secret", 400, 0},
		{"?owner=o", "secret", 400, 0},
		{"?platform=&owner=&repo=", "secret", 400, 0},
		{"?platform=gitlab&owner=o&repo=r", "secret", 400, 0},
		{"?platform=forgejo&owner=other&repo=one", "secret", 200, 1},
		{"?platform=github&owner=o&repo=none", "secret", 200, 0},
	} {
		t.Run(tc.query+tc.token, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/runs"+tc.query, nil)
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tc.status == 200 {
				var runs []db.GenerationRun
				if err := json.Unmarshal(w.Body.Bytes(), &runs); err != nil {
					t.Fatal(err)
				}
				if len(runs) != tc.count {
					t.Fatalf("count=%d want=%d", len(runs), tc.count)
				}
				for i, run := range runs {
					if i > 0 && run.ID >= runs[i-1].ID {
						t.Fatal("history not newest first")
					}
				}
				for _, forbidden := range []string{"notes", "instructions", "prompt", "api_key", "config_digest"} {
					if strings.Contains(w.Body.String(), `"`+forbidden+`"`) {
						t.Fatalf("history leaked %s", forbidden)
					}
				}
			}
		})
	}
}

func TestRepoLatestRunOutsideHistoryLimit(t *testing.T) {
	a, _, store := testAPI(t, &config.Config{}, nil)
	for _, repo := range []string{"old", "busy", "empty"} {
		if err := store.UpsertRepoSettings(db.RepoSetting{Platform: "github", Owner: "o", Repo: repo}); err != nil {
			t.Fatal(err)
		}
	}
	id, err := store.StartRun(db.GenerationRun{Platform: "github", Owner: "o", Repo: "old", Source: "manual", ToTag: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishRun(id, "", "generated", "", ""); err != nil {
		t.Fatal(err)
	}
	for range 105 {
		if _, err := store.StartRun(db.GenerationRun{Platform: "github", Owner: "o", Repo: "busy", Source: "manual", ToTag: "v2"}); err != nil {
			t.Fatal(err)
		}
	}
	w := do(a.handleListRepos, newReq(http.MethodGet, "/api/repos", "", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var repos []repoItemResp
	if err := json.Unmarshal(w.Body.Bytes(), &repos); err != nil {
		t.Fatal(err)
	}
	for _, repo := range repos {
		if repo.Repo == "old" && (repo.LatestRun == nil || repo.LatestRun.ID != id) {
			t.Fatalf("old repo latest missing: %+v", repo)
		}
		if repo.Repo == "empty" && repo.LatestRun != nil {
			t.Fatal("invented history")
		}
	}
}

func TestGenerateUsesObservedOutcome(t *testing.T) {
	for _, outcome := range []string{"generated", "published", "cached", "preserved"} {
		t.Run(outcome, func(t *testing.T) {
			a, pip, _ := testAPI(t, &config.Config{}, nil)
			pip.outcome = outcome
			if outcome == "published" {
				pip.warningCode = "cache_persistence"
			}
			w := do(a.handleGenerate, newReq(http.MethodPost, "/generate", `{"to_tag":"v1","publish":true}`, map[string]string{"platform": "github", "owner": "o", "repo": "r"}))
			if w.Code != 200 {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["outcome"] != outcome || body["published"] != (outcome == "published") {
				t.Fatalf("body=%+v", body)
			}
			if outcome == "published" && body["warning_code"] != "cache_persistence" {
				t.Fatalf("warning lost: %+v", body)
			}
			if outcome != "published" && body["warning_code"] != nil {
				t.Fatalf("absent warning not null: %+v", body)
			}
		})
	}
}

type runTestPlatform struct{ origin, body string }

func (p *runTestPlatform) ReadRepoFile(context.Context, string, string, string, string) (string, error) {
	return "", pipeline.ErrNotFound
}
func (p *runTestPlatform) GetReleaseByTag(context.Context, string, string, string) (*pipeline.Release, error) {
	return &pipeline.Release{ID: 1}, nil
}
func (p *runTestPlatform) ListReleases(context.Context, string, string) ([]pipeline.ReleaseMeta, error) {
	return nil, nil
}
func (p *runTestPlatform) EditReleaseBody(_ context.Context, _, _ string, _ int64, body string) error {
	p.body = body
	return nil
}
func (p *runTestPlatform) CloneInfo(context.Context, string, string) (string, string, error) {
	return p.origin, "", nil
}

func TestGenerateRemoteSuccessCacheFailureAPI(t *testing.T) {
	dir, origin := t.TempDir(), t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "chore: initial"}, {"tag", "v1"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = origin
		cmd.Env = append(os.Environ(), "PRE_COMMIT_ALLOW_NO_CONFIG=1", "GIT_CEILING_DIRECTORIES="+origin)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	store, err := db.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	conn := openTestDB(t, dir)
	if _, err := conn.Exec(`CREATE TRIGGER reject_generated BEFORE INSERT ON generated_notes BEGIN SELECT RAISE(FAIL, 'forced cache failure'); END`); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Data: config.DataConfig{Dir: dir}}
	platform := &runTestPlatform{origin: origin}
	pip := pipeline.New(cfg, store, llm.New(cfg.LLM), platform, nil)
	a := &api{cfg: cfg, store: store, pip: pip}
	w := do(a.handleGenerate, newReq(http.MethodPost, "/generate", `{"to_tag":"v1","from_tag":"v1","publish":true}`, map[string]string{"platform": "github", "owner": "o", "repo": "r"}))
	if w.Code != 200 {
		t.Fatalf("remote success returned %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["published"] != true || body["outcome"] != "published" || body["warning_code"] != "cache_persistence" {
		t.Fatalf("body=%+v", body)
	}
	if !strings.Contains(platform.body, "No changes documented.") {
		t.Fatalf("remote body=%q", platform.body)
	}
	runs, err := store.ListRuns("github", "o", "r", 20)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	if runs[0].Status != "published" || runs[0].WarningCode == nil || *runs[0].WarningCode != "cache_persistence" || runs[0].ErrorCode != nil {
		t.Fatalf("run=%+v", runs[0])
	}
}

func TestHandleListReposHistoryLoadFailure(t *testing.T) {
	dir := t.TempDir()
	store, err := db.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	// Drop the history table after setup so LatestRuns fails; ListRepoSettings
	// still succeeds, so the handler reaches the history-load branch.
	conn := openTestDB(t, dir)
	if _, err := conn.Exec(`DROP TABLE generation_runs`); err != nil {
		t.Fatal(err)
	}
	a := &api{cfg: &config.Config{}, store: store, pip: newFakePip()}
	w := do(a.handleListRepos, newReq(http.MethodGet, "/api/repos", "", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "could not load generation history") {
		t.Fatalf("body=%s", w.Body.String())
	}
}
