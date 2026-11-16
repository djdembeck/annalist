package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/llm"
	"github.com/djdembeck/annalist/internal/pipeline"
)

func TestQualityAPIResponseContract(t *testing.T) {
	params := map[string]string{"platform": "github", "owner": "o", "repo": "r"}
	t.Run("captured preview", func(t *testing.T) {
		a, pip, _ := testAPI(t, &config.Config{}, nil)
		pip.notes = "Captured notes; unchanged."
		pip.warningCode = "quality_review_required"
		pip.qualityIssues = []string{"strict_prose_semicolon", "source_context_incomplete"}
		w := do(a.handleGenerate, newReq(http.MethodPost, "/generate", `{"to_tag":"v2","publish":false}`, params))
		var body struct {
			Notes     string   `json:"notes"`
			Outcome   string   `json:"outcome"`
			Published bool     `json:"published"`
			Warning   string   `json:"warning_code"`
			Issues    []string `json:"quality_issues"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || body.Notes != pip.notes || body.Outcome != "generated" || body.Published || body.Warning != "quality_review_required" || !slices.Equal(body.Issues, pip.qualityIssues) {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
	t.Run("safe typed publication error", func(t *testing.T) {
		a, pip, _ := testAPI(t, &config.Config{}, nil)
		pip.err = fmt.Errorf("raw model and source must stay private: %w", &pipeline.QualityError{Issues: []string{"source_context_incomplete"}})
		w := do(a.handleGenerate, newReq(http.MethodPost, "/generate", `{"to_tag":"v2","publish":true}`, params))
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusUnprocessableEntity || body["published"] != false || body["warning_code"] != "quality_review_required" || body["error"] != "pipeline: notes need quality review before publication" {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if _, exists := body["notes"]; exists || strings.Contains(w.Body.String(), "raw model and source") {
			t.Fatalf("quality error exposed notes/source: %s", w.Body.String())
		}
	})
}

func TestSourceLinkAPIErrorIsSafe(t *testing.T) {
	a, pip, _ := testAPI(t, &config.Config{}, nil)
	pip.err = fmt.Errorf("private provider wrapper must stay private: %w", &pipeline.SourceReferenceError{Platform: "github", Err: fmt.Errorf("private token provider response")})
	params := map[string]string{"platform": "github", "owner": "o", "repo": "r"}
	w := do(a.handleGenerate, newReq(http.MethodPost, "/generate", `{"to_tag":"v2","publish":true,"force":true}`, params))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusBadGateway || body["published"] != false || body["error_code"] != "source_links" || !strings.Contains(w.Body.String(), "Pull requests: Read-only") || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("unsafe or unhelpful provider failure: status=%d body=%s", w.Code, w.Body.String())
	}
	if _, exists := body["notes"]; exists {
		t.Fatalf("unresolved source exposed note text: %s", w.Body.String())
	}
}

func TestQualityAPIControlledGenerationAndHistory(t *testing.T) {
	origin := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"commit", "--allow-empty", "-m", "chore: initial"},
		{"tag", "v1"},
		{"commit", "--allow-empty", "-m", "feat: add Fern archive export"},
		{"tag", "v2"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = origin
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid", "PRE_COMMIT_ALLOW_NO_CONFIG=1", "GIT_CEILING_DIRECTORIES="+origin)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	const notes = "Fern export is available; archived records remain readable."
	const linkedNotes = "- " + notes + " ([#17](https://github.com/o/r/pull/17))"
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		var answer string
		for _, message := range request.Messages {
			if message.Role != "user" {
				continue
			}
			for line := range strings.SplitSeq(message.Content, "\n") {
				if token, ok := strings.CutPrefix(line, "Source: "); ok {
					answer = "- " + notes + " " + token
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}}})
	}))
	defer model.Close()
	dir := t.TempDir()
	store, err := db.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertSettings(db.Settings{Tone: "engineer"}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Data: config.DataConfig{Dir: dir}, LLM: config.LLMConfig{BaseURL: model.URL, Model: "controlled-model"}}
	platform := &runTestPlatform{origin: origin, body: "original release body"}
	pip := pipeline.New(cfg, store, llm.New(cfg.LLM), platform, nil)
	a := &api{cfg: cfg, store: store, pip: pip}
	params := map[string]string{"platform": "github", "owner": "o", "repo": "r"}
	preview := do(a.handleGenerate, newReq(http.MethodPost, "/generate", `{"to_tag":"v2","from_tag":"v1","publish":false}`, params))
	var body struct {
		Notes   string   `json:"notes"`
		Warning string   `json:"warning_code"`
		Issues  []string `json:"quality_issues"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if preview.Code != http.StatusOK || body.Notes != linkedNotes || body.Warning != "quality_review_required" || !slices.Equal(body.Issues, []string{"strict_prose_semicolon"}) {
		t.Fatalf("preview=%d %s", preview.Code, preview.Body.String())
	}
	rejected := do(a.handleGenerate, newReq(http.MethodPost, "/generate", `{"to_tag":"v2","from_tag":"v1","publish":true,"force":true}`, params))
	if rejected.Code != http.StatusUnprocessableEntity || strings.Contains(rejected.Body.String(), notes) || !strings.Contains(rejected.Body.String(), "strict_prose_semicolon") {
		t.Fatalf("publication=%d %s", rejected.Code, rejected.Body.String())
	}
	var cached int
	if err := openTestDB(t, dir).QueryRow(`SELECT COUNT(*) FROM generated_notes`).Scan(&cached); err != nil {
		t.Fatal(err)
	}
	if platform.gets != 0 || platform.edits != 0 || platform.body != "original release body" || cached != 0 || calls != 2 {
		t.Fatalf("quality failure had side effects: gets=%d edits=%d cached=%d model_calls=%d", platform.gets, platform.edits, cached, calls)
	}
	history := do(a.handleListRuns, newReq(http.MethodGet, "/api/runs?platform=github&owner=o&repo=r", "", nil))
	var runs []db.GenerationRun
	if err := json.Unmarshal(history.Body.Bytes(), &runs); err != nil {
		t.Fatal(err)
	}
	if history.Code != http.StatusOK || len(runs) != 2 || runs[0].Status != "failed" || runs[0].ErrorCode == nil || *runs[0].ErrorCode != "quality" || runs[1].Status != "generated" {
		t.Fatalf("history=%d %s", history.Code, history.Body.String())
	}
	for _, run := range runs {
		if run.WarningCode == nil || *run.WarningCode != "quality_review_required" {
			t.Fatalf("quality warning missing from history: %+v", run)
		}
	}
	if strings.Contains(history.Body.String(), notes) || strings.Contains(history.Body.String(), "Fern archive export") {
		t.Fatalf("history exposed output/source: %s", history.Body.String())
	}
}
