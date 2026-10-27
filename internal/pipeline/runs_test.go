package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djdembeck/annalist/internal/db"
)

func onlyRun(t *testing.T, store *db.Store) db.GenerationRun {
	t.Helper()
	runs, err := store.ListRuns("", "", "", 20)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	return runs[0]
}

func TestMonitoredGenerationOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		publish          bool
	}{
		{"preview", "", "generated", false},
		{"publish", "", "published", true},
		{"human body", "hand written release", "preserved", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pip, _, f, store := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1, Body: tc.body}})
			spec := genSpec("v0.2.0", "monitored")
			result, err := pip.GenerateNotes(context.Background(), spec, Options{Source: "manual", Publish: tc.publish})
			if err != nil {
				t.Fatal(err)
			}
			run := onlyRun(t, store)
			if result.Outcome != tc.want || run.Status != tc.want || run.FinishedAt == nil || run.ErrorCode != nil || run.WarningCode != nil || run.FromTag != result.FromTag {
				t.Fatalf("result=%+v run=%+v", result, run)
			}
			if tc.want == "preserved" && f.edited != "" {
				t.Fatal("human body overwritten")
			}
			if tc.want == "published" {
				cached, err := pip.GenerateNotes(context.Background(), spec, Options{Source: "webhook", Publish: true})
				if err != nil || cached.Outcome != "cached" || onlyRun(t, store).Status != "cached" {
					t.Fatalf("cached=%+v err=%v", cached, err)
				}
			}
		})
	}
}

func TestMonitoredFailureCodesDoNotLeak(t *testing.T) {
	pip, _, f, store := fixture(t, nil, nil)
	f.cloneErr = errors.New("secret token private-host prompt")
	_, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "failed"), Options{Source: "manual"})
	if err == nil {
		t.Fatal("expected clone failure")
	}
	run := onlyRun(t, store)
	if run.Status != "failed" || run.ErrorCode == nil || *run.ErrorCode != "repository" || run.FinishedAt == nil {
		t.Fatalf("run=%+v", run)
	}
	encoded, _ := json.Marshal(run)
	for _, secret := range []string{"secret token", "private-host", "prompt"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("history leaked %q: %s", secret, encoded)
		}
	}
}

func TestMonitoredCancellationAndValidation(t *testing.T) {
	pip, _, _, store := fixture(t, nil, nil)
	spec := genSpec("v0.2.0", "cancelled")
	spec.Profile = "bad profile"
	if _, err := pip.GenerateNotes(context.Background(), spec, Options{Source: "manual"}); err == nil {
		t.Fatal("expected validation failure")
	}
	runs, err := store.ListRuns("", "", "", 20)
	if err != nil || len(runs) != 0 {
		t.Fatalf("invalid request recorded: %+v %v", runs, err)
	}
	spec.Profile = ""
	for range cap(pip.sem()) {
		pip.sem() <- struct{}{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pip.GenerateNotes(ctx, spec, Options{Source: "manual"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	run := onlyRun(t, store)
	if run.Status != "failed" || run.ErrorCode == nil || *run.ErrorCode != "cancelled" || run.FinishedAt == nil {
		t.Fatalf("cancelled terminal run=%+v", run)
	}
}

func TestMonitoredCacheReadFailureIsConfiguration(t *testing.T) {
	pip, stub, f, store := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1}})
	conn, err := sql.Open("sqlite", "file:"+filepath.Join(pip.Cfg.Data.Dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`DROP TABLE generated_notes`); err != nil {
		t.Fatal(err)
	}

	_, err = pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "cache-read-failure"), Options{Source: "manual", Publish: true})
	if err == nil || !strings.Contains(err.Error(), "check generated") {
		t.Fatalf("err=%v, want generated-cache lookup failure", err)
	}
	run := onlyRun(t, store)
	if run.Status != "failed" || run.ErrorCode == nil || *run.ErrorCode != "configuration" {
		t.Fatalf("cache lookup failure classified incorrectly: %+v", run)
	}
	if stub.calls != 0 || f.edited != "" {
		t.Fatalf("cache lookup failure reached LLM/publication: llm calls=%d published body=%q", stub.calls, f.edited)
	}
}

func TestPublishedCacheFailureRemainsPublished(t *testing.T) {
	pip, _, f, store := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1}})
	conn, err := sql.Open("sqlite", "file:"+filepath.Join(pip.Cfg.Data.Dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`CREATE TRIGGER reject_generated BEFORE INSERT ON generated_notes BEGIN SELECT RAISE(FAIL, 'forced cache failure'); END`); err != nil {
		t.Fatal(err)
	}
	result, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "cache-failure"), Options{Source: "manual", Publish: true})
	if err != nil {
		t.Fatalf("successful remote edit reported failure: %v", err)
	}
	if result.Outcome != "published" || result.WarningCode != "cache_persistence" || result.Notes == "" || !strings.Contains(f.edited, result.Notes) || !strings.HasSuffix(f.edited, marker) {
		t.Fatalf("result=%+v remote=%q", result, f.edited)
	}
	run := onlyRun(t, store)
	if run.Status != "published" || run.ErrorCode != nil || run.WarningCode == nil || *run.WarningCode != "cache_persistence" {
		t.Fatalf("run=%+v", run)
	}
	written, err := pip.Publish(context.Background(), result, genSpec("v0.2.0", "direct-cache-failure"), f, true)
	if !written || err == nil {
		t.Fatalf("Publish must preserve remote success: written=%v err=%v", written, err)
	}
}

func TestPublishedTerminalWriteFailureRemainsPublished(t *testing.T) {
	pip, _, f, store := fixture(t, nil, map[string]*Release{"v0.2.0": {ID: 1}})
	conn, err := sql.Open("sqlite", "file:"+filepath.Join(pip.Cfg.Data.Dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`CREATE TRIGGER reject_finish BEFORE UPDATE ON generation_runs BEGIN SELECT RAISE(FAIL, 'forced terminal failure'); END`); err != nil {
		t.Fatal(err)
	}
	result, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "terminal-failure"), Options{Source: "manual", Publish: true})
	if err != nil || result.Outcome != "published" || !strings.Contains(f.edited, result.Notes) {
		t.Fatalf("remote success lost: result=%+v err=%v", result, err)
	}
	if onlyRun(t, store).Status != "running" {
		t.Fatal("terminal persistence failure falsely recorded success")
	}
	if _, err := conn.Exec(`DROP TRIGGER reject_finish`); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileRuns(); err != nil {
		t.Fatal(err)
	}
	if onlyRun(t, store).Status != "interrupted" {
		t.Fatal("unfinished publication not reconciled")
	}
}

func TestMonitoringCreationFailurePreventsRemoteWork(t *testing.T) {
	pip, stub, f, store := fixture(t, nil, nil)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "db-closed"), Options{Source: "manual", Publish: true})
	if err == nil || !strings.Contains(err.Error(), "start generation run") {
		t.Fatalf("err=%v", err)
	}
	if stub.calls != 0 || f.edited != "" || len(f.reads) != 0 {
		t.Fatal("remote work performed without history")
	}
}
