package db

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRunPersistenceAndExplicitReconciliation(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.StartRun(GenerationRun{Platform: "github", Owner: "o", Repo: "r", Source: "manual", ToTag: "v2", DisplayVersion: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	other, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	store = other
	runs, err := other.ListRuns("", "", "", 20)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	if runs[0].Status != "running" || runs[0].FinishedAt != nil || runs[0].ErrorCode != nil || runs[0].WarningCode != nil {
		t.Fatalf("opening DB modified run: %+v", runs[0])
	}
	if _, err := time.Parse(time.RFC3339, runs[0].StartedAt); err != nil {
		t.Fatal(err)
	}
	if err := other.ReconcileRuns(); err != nil {
		t.Fatal(err)
	}
	runs, err = store.ListRuns("github", "o", "r", 20)
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].ID != id || runs[0].Status != "interrupted" || runs[0].FinishedAt == nil || runs[0].ErrorCode == nil || *runs[0].ErrorCode != "interrupted" {
		t.Fatalf("run=%+v", runs[0])
	}
	if err := store.FinishRun(id, "v1", "published", "", ""); err != nil {
		t.Fatal(err)
	}
	runs, _ = store.ListRuns("", "", "", 20)
	if runs[0].Status != "interrupted" {
		t.Fatal("terminal run was overwritten")
	}
}

func TestLatestRunBeyondRecentHistory(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, repo := range []string{"old", "busy", "empty"} {
		if err := store.UpsertRepoSettings(RepoSetting{Platform: "github", Owner: "o", Repo: repo}); err != nil {
			t.Fatal(err)
		}
	}
	old, err := store.StartRun(GenerationRun{Platform: "github", Owner: "o", Repo: "old", Source: "manual", ToTag: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishRun(old, "", "published", "", "cache_persistence"); err != nil {
		t.Fatal(err)
	}
	for range 110 {
		if _, err := store.StartRun(GenerationRun{Platform: "github", Owner: "o", Repo: "busy", Source: "webhook", ToTag: "v2"}); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := store.ListRuns("", "", "", 100)
	if err != nil || len(recent) != 100 {
		t.Fatalf("recent=%d err=%v", len(recent), err)
	}
	latest, err := store.LatestRuns()
	if err != nil || len(latest) != 2 {
		t.Fatalf("latest=%+v err=%v", latest, err)
	}
	found := false
	for _, run := range latest {
		if run.ID == old {
			found = true
			if run.WarningCode == nil || *run.WarningCode != "cache_persistence" {
				t.Fatalf("warning lost: %+v", run)
			}
		}
	}
	if !found {
		t.Fatal("old repository omitted from latest")
	}
	filtered, err := store.ListRuns("github", "o", "old' OR 1=1 --", 20)
	if err != nil || len(filtered) != 0 {
		t.Fatalf("filter not literal: %+v %v", filtered, err)
	}
	encoded, err := json.Marshal(latest)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"notes", "instructions", "api_key", "config_digest", "prompt"} {
		if strings.Contains(string(encoded), `"`+forbidden+`"`) {
			t.Fatalf("sensitive history field %s", forbidden)
		}
	}
}
