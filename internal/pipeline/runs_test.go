package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/djdembeck/annalist/internal/db"
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
	conn := openTestDB(t, pip.Cfg.Data.Dir)
	if _, err := conn.Exec(`DROP TABLE generated_notes`); err != nil {
		t.Fatal(err)
	}

	_, err := pip.GenerateNotes(context.Background(), genSpec("v0.2.0", "cache-read-failure"), Options{Source: "manual", Publish: true})
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
	conn := openTestDB(t, pip.Cfg.Data.Dir)
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
	conn := openTestDB(t, pip.Cfg.Data.Dir)
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

// TestMonitoredFailureCodeTaxonomy maps each monitored failure site to its
// recorded error code: LLM transport → generation, profile/instruction
// reads → instructions, release-body writes → publication, and a deadline
// that expires while the generation semaphore is full → timeout.
func TestMonitoredFailureCodeTaxonomy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		releases map[string]*Release
		profile  string
		opts     Options
		wantCode string
		setup    func(t *testing.T, pip *Pipeline, stub *llmStub, f *fakePlatform)
	}{
		{
			name:     "llm 500 is generation",
			opts:     Options{Source: "manual"},
			wantCode: "generation",
			setup: func(t *testing.T, pip *Pipeline, stub *llmStub, f *fakePlatform) {
				stub.errStatus = 500
			},
		},
		{
			name:     "profile read failure is instructions",
			profile:  "technical",
			opts:     Options{Source: "manual"},
			wantCode: "instructions",
			setup: func(t *testing.T, pip *Pipeline, stub *llmStub, f *fakePlatform) {
				f.readErr = errors.New("profile api down")
			},
		},
		{
			name:     "release edit failure is publication",
			releases: map[string]*Release{"v0.2.0": {ID: 1}},
			opts:     Options{Source: "manual", Publish: true},
			wantCode: "publication",
			setup: func(t *testing.T, pip *Pipeline, stub *llmStub, f *fakePlatform) {
				f.editErr = errors.New("publish api down")
			},
		},
		{
			name:     "expired deadline under a full semaphore is timeout",
			opts:     Options{Source: "manual"},
			wantCode: "timeout",
			setup: func(t *testing.T, pip *Pipeline, stub *llmStub, f *fakePlatform) {
				for range cap(pip.sem()) {
					pip.sem() <- struct{}{}
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pip, stub, f, store := fixtureWithStub(t, newStub("TAXONOMY PROSE"), nil, tc.releases)
			tc.setup(t, pip, stub, f)
			spec := genSpec("v0.2.0", "tax-"+tc.name)
			spec.Profile = tc.profile
			ctx := context.Background()
			if tc.wantCode == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
			}
			_, err := pip.GenerateNotes(ctx, spec, tc.opts)
			if tc.wantCode == "timeout" {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("err=%v, want deadline exceeded", err)
				}
			} else if err == nil {
				t.Fatal("expected a generation failure")
			}
			run := onlyRun(t, store)
			if run.Status != "failed" || run.ErrorCode == nil || *run.ErrorCode != tc.wantCode || run.FinishedAt == nil {
				t.Fatalf("run=%+v, want failed/%s with FinishedAt", run, tc.wantCode)
			}
			if tc.wantCode == "timeout" {
				for range cap(pip.sem()) {
					<-pip.sem()
				}
			}
		})
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
