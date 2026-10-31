package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Run the real decision script against real refs: the selected version is the
// consumer-visible boundary, not the spelling of the workflow or its wiring.
func workflowDecision(t *testing.T, template, origin, source, requested string) (string, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "workflows", template))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				ID  string `yaml:"id"`
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var script string
	for _, step := range workflow.Jobs["preflight"].Steps {
		if step.ID == "decide" {
			script = step.Run
			break
		}
	}
	if script == "" {
		t.Fatal("workflow has no executable version decision")
	}
	script = strings.NewReplacer(
		"${{ steps.freeze.outputs.sha }}", source,
		"${{ inputs.version }}", requested,
		"${{ inputs.bump }}", "auto",
	).Replace(script)
	output := filepath.Join(t.TempDir(), "decision")
	cmd := exec.Command("bash", "-euo", "pipefail", "-c", script)
	cmd.Dir = origin
	cmd.Env = append(os.Environ(), "GITHUB_OUTPUT="+output)
	log, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Logf("version decision: %s", log)
	}
	result, err := os.ReadFile(output)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(result)), runErr
}

// The real 'Build and publish' step, executed against local annalist and
// release-API boundary stubs. The consumer contract under test: ONE metadata
// read (body plus draft) decides every path — a published nonempty release
// no-ops WITHOUT any generation, drafts are explicitly published, and a
// metadata read failure that is not an unambiguous absence (403/500/network)
// blocks the step instead of overwriting existing notes.
func TestWorkflowPublishStepStateTransitions(t *testing.T) {
	for _, template := range []string{"github-release.yml", "forgejo-release.yml"} {
		t.Run(template, func(t *testing.T) { runPublishScenarios(t, template) })
	}
}

func runPublishScenarios(t *testing.T, template string) {
	t.Helper()
	fx := &pubFixture{}
	srv := httptest.NewServer(http.HandlerFunc(fx.handle))
	defer srv.Close()

	var ghDir string
	if template == "github-release.yml" {
		ghDir = writeGHStub(t)
	}
	script, env, err := publishStepScript(template, srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	tt := []struct {
		name        string
		state       map[string]any // initial release state; nil = release does not exist
		failLookup  bool           // release metadata read returns 500 (not an absence)
		failGen     bool           // annalist generate returns 500
		ghView      string         // GH stub view mode: "", "missing", "failure"
		ghOnly      bool           // drives only the GitHub stub; skipped on Forgejo
		wantExit    int
		wantBody    string
		wantDraft   bool
		wantCreated bool
		wantID      int
		wantGen     int
	}{
		{name: "published complete is a no-op with no generation",
			state:    map[string]any{"body": "HUMAN NOTES", "draft": false},
			wantExit: 0, wantBody: "HUMAN NOTES", wantDraft: false, wantCreated: true, wantID: 1, wantGen: 0},
		// Resume on an EXISTING published release: the notes fill via edit,
		// the release is never re-created — the boundary's duplicate-create
		// refusal (gh: pre-existing state file / API: 409) would fail the
		// step if the template ever regressed to create.
		{name: "published empty notes are refilled by edit, never re-created",
			state:    map[string]any{"body": "", "draft": false},
			wantExit: 0, wantBody: "PING GENERATED NOTES", wantDraft: false, wantCreated: true, wantID: 1, wantGen: 1},
		{name: "empty draft is refilled and explicitly published",
			state:    map[string]any{"body": "", "draft": true},
			wantExit: 0, wantBody: "PING GENERATED NOTES", wantDraft: false, wantCreated: true, wantID: 1, wantGen: 1},
		{
			name:     "draft with human notes is published without generation",
			state:    map[string]any{"body": "HUMAN DRAFT NOTES", "draft": true},
			wantExit: 0, wantBody: "HUMAN DRAFT NOTES", wantDraft: false, wantCreated: true, wantID: 1, wantGen: 0,
		},
		{name: "missing release is created once with generated notes",
			state: nil, ghView: "missing",
			wantExit: 0, wantBody: "PING GENERATED NOTES", wantDraft: false, wantCreated: true, wantID: 1, wantGen: 1},
		{name: "metadata read failure blocks the step and leaves the state untouched",
			state:      map[string]any{"body": "HUMAN NOTES", "draft": false},
			failLookup: true, ghView: "failure",
			wantExit: 1, wantBody: "HUMAN NOTES", wantDraft: false, wantCreated: true, wantID: 1, wantGen: 0},
		// A repo-visibility/authorization blur looks NOTHING like the CLI's
		// exact absence line, so the step must refuse to MODIFY (and in
		// particular to CREATE) the release.
		{name: "repo visibility failure blocks the step and creates nothing",
			state: nil, ghView: "failure", ghOnly: true,
			wantExit: 1, wantBody: "", wantDraft: false, wantCreated: false, wantID: 0, wantGen: 0},
		{name: "completed release no-ops even when annalist is down",
			state: map[string]any{"body": "HUMAN NOTES", "draft": false}, failGen: true,
			wantExit: 0, wantBody: "HUMAN NOTES", wantDraft: false, wantCreated: true, wantID: 1, wantGen: 0},
		// One generation request was made (and it is the one that 500s) —
		// the step then fails without creating a release.
		{name: "missing release with annalist down fails without creating a release",
			state: nil, failGen: true, ghView: "missing",
			wantExit: 22, wantBody: "", wantDraft: false, wantCreated: false, wantID: 0, wantGen: 1},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ghOnly && ghDir == "" {
				t.Skip("GitHub-stub-only scenario; no gh boundary on this platform")
			}
			ghState := ""
			if ghDir != "" {
				ghState = filepath.Join(t.TempDir(), "gh-state.json")
			}
			fx.reset(tc.state, tc.failLookup, tc.failGen, ghState)

			var extra []string
			if ghDir != "" {
				extra = []string{
					"PATH=" + ghDir + string(os.PathListSeparator) + os.Getenv("PATH"),
					"GH_STUB_STATE=" + ghState,
					"GH_STUB_VIEW=" + tc.ghView,
				}
			}
			out, runErr := runPublishScript(t, script, env, extra)
			if tc.wantExit == 0 {
				if runErr != nil {
					t.Fatalf("publish step failed: %v\n%s", runErr, out)
				}
			} else if runErr == nil {
				t.Fatalf("publish step succeeded, want exit %d\n%s", tc.wantExit, out)
			} else if ee, ok := runErr.(*exec.ExitError); !ok || ee.ExitCode() != tc.wantExit {
				t.Fatalf("publish step exited with %v, want %d\n%s", runErr, tc.wantExit, out)
			}

			body, draft, created, id := fx.postState()
			if body != tc.wantBody || draft != tc.wantDraft || created != tc.wantCreated || id != tc.wantID {
				t.Errorf("release state = (body %q, draft %v, created %v, id %d), want (%q, %v, %v, %d)",
					body, draft, created, id, tc.wantBody, tc.wantDraft, tc.wantCreated, tc.wantID)
			}
			if fx.gens() != tc.wantGen {
				t.Errorf("annalist generate calls = %d, want %d", fx.gens(), tc.wantGen)
			}
		})
	}
}

// publishStepScript extracts the real 'Build and publish' script and the
// workflow/step env it runs under, with the external endpoints pointed at the
// local fixture server.
func publishStepScript(template, serverURL string) (string, []string, error) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "workflows", template))
	if err != nil {
		return "", nil, err
	}
	var workflow struct {
		Env  map[string]string `yaml:"env"`
		Jobs map[string]struct {
			Steps []struct {
				Name string            `yaml:"name"`
				Env  map[string]string `yaml:"env"`
				Run  string            `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return "", nil, err
	}
	script, stepEnv := "", map[string]string{}
	for _, s := range workflow.Jobs["publish"].Steps {
		if s.Name == "Build and publish" {
			script, stepEnv = s.Run, s.Env
			break
		}
	}
	if script == "" {
		return "", nil, fmt.Errorf("workflow has no Build and publish step")
	}
	repl := strings.NewReplacer(
		"https://annalist.example.com", serverURL,
		"${{ github.server_url }}", serverURL,
		"${{ github.repository_owner }}", "stubowner",
		"${{ github.event.repository.name }}", "stubrepo",
		"${{ github.repository }}", "stubowner/stubrepo",
		"${{ github.token }}", "stub-gh-token",
		"${{ secrets.FORGEJO_TOKEN }}", "stub-fj-token",
		"${{ secrets.ANNALIST_ADMIN_TOKEN }}", "stub-annalist-token",
		"${{ needs.preflight.outputs.version }}", "v1.0.0",
	)
	env := make([]string, 0, len(workflow.Env)+len(stepEnv))
	for _, kv := range []map[string]string{workflow.Env, stepEnv} {
		for k, v := range kv {
			env = append(env, k+"="+repl.Replace(v))
		}
	}
	return repl.Replace(script), env, nil
}

func runPublishScript(t *testing.T, script string, env, extra []string) (string, error) {
	t.Helper()
	stepPath := filepath.Join(t.TempDir(), "step.sh")
	if err := os.WriteFile(stepPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", stepPath)
	cmd.Env = append(append(os.Environ(), env...), extra...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// pubFixture is the stateful boundary the extracted step talks to: the
// annalist generate endpoint for both templates, and the Forgejo release API
// (the GitHub template's release state lives in the gh stub's state file).
type pubFixture struct {
	mu         sync.Mutex
	exists     bool
	id         int
	body       string
	draft      bool
	genCalls   int
	failLookup bool
	failGen    bool
	ghState    string
}

func (f *pubFixture) reset(state map[string]any, failLookup, failGen bool, ghState string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exists = state != nil
	f.id = 1
	f.genCalls = 0
	f.failLookup = failLookup
	f.failGen = failGen
	f.ghState = ghState
	f.body, f.draft = "", false
	if state == nil {
		if ghState != "" {
			os.Remove(ghState)
		}
		return
	}
	f.body, _ = state["body"].(string)
	f.draft, _ = state["draft"].(bool)
	if ghState != "" {
		// The GitHub template's release state lives in the gh stub's file.
		raw, _ := json.Marshal(map[string]any{"id": 1, "body": f.body, "isDraft": f.draft})
		if err := os.WriteFile(ghState, raw, 0o600); err != nil {
			panic(err)
		}
	}
}

func (f *pubFixture) gens() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.genCalls
}

// postState returns (body, draft, releaseExists, releaseID) from the release
// store the template actually writes to: the gh stub state file for GitHub,
// the fixture state for Forgejo.
func (f *pubFixture) postState() (string, bool, bool, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ghState != "" {
		raw, err := os.ReadFile(f.ghState)
		if err != nil {
			return "", false, false, 0
		}
		var st struct {
			ID      int
			Body    string
			IsDraft bool
		}
		if err := json.Unmarshal(raw, &st); err != nil {
			return "", false, false, 0
		}
		return st.Body, st.IsDraft, true, st.ID
	}
	if !f.exists {
		return "", false, false, 0
	}
	return f.body, f.draft, true, f.id
}

func (f *pubFixture) handle(w http.ResponseWriter, r *http.Request) {
	repoAPI := "/api/v1/repos/stubowner/stubrepo"
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, repoAPI+"/releases/tags/"):
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failLookup {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"internal fixture failure"}`)
			return
		}
		if !f.exists {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"release not found"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%d,"tag_name":"%s","body":%q,"draft":%t,"prerelease":false}`,
			f.id, strings.TrimPrefix(r.URL.Path, repoAPI+"/releases/tags/"), f.body, f.draft)

	case r.Method == http.MethodPost && r.URL.Path == repoAPI+"/releases":
		f.mu.Lock()
		defer f.mu.Unlock()
		var created struct {
			TagName string
			Body    string
			Draft   bool
		}
		if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if f.exists {
			// The real API refuses to replace an existing release — the
			// resume path must EDIT, never re-create.
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"message":"a release with this tag already exists"}`)
			return
		}
		f.exists, f.id, f.body, f.draft = true, 1, created.Body, false
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":1,"tag_name":%q,"body":%q,"draft":false}`, created.TagName, f.body)

	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, repoAPI+"/releases/"):
		f.mu.Lock()
		defer f.mu.Unlock()
		var patch struct {
			Body  *string
			Draft *bool
		}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if patch.Body != nil {
			f.body = *patch.Body
		}
		if patch.Draft != nil {
			f.draft = *patch.Draft
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%d,"body":%q,"draft":%t}`, f.id, f.body, f.draft)

	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/generate"):
		f.mu.Lock()
		f.genCalls++
		down := f.failGen
		f.mu.Unlock()
		if down {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"annalist down"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"notes":"PING GENERATED NOTES","range_policy":"previous_published"}`)
	}
}

// writeGHStub installs the `gh` binary the GitHub template talks to: a
// release state file with view/edit/create, and failures shaped like the real
// CLI. Absence is the CLI's exact stderr line (verified against the
// installed gh: `release not found`, exit 1); a repo-visibility/authorization
// failure emits its own message and must NOT read as absence. `create`
// refuses to replace an existing release (id stays the existing one), exactly
// like the real CLI.
func writeGHStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stub := `#!/usr/bin/env bash
set -euo pipefail
# Production invokes "gh release <view|edit|create> TAG ...".
if [ "${1:-}" != "release" ]; then
  echo "missing required argument: release" >&2
  exit 2
fi
shift
cmd="${1:-}"; shift
tag=""
if [ "$#" -gt 0 ] && [[ "${1}" != --* ]]; then
  tag="${1}"; shift
fi
case "${cmd}" in
view)
  case "${GH_STUB_VIEW:-}" in
  missing)
    echo "release not found" >&2
    exit 1
    ;;
  failure)
    echo "GraphQL: Forbidden - authorization failed (repositoryOwner[stubowner].release[viewReleaseByTag])." >&2
    exit 1
    ;;
  esac
  if [ ! -f "${GH_STUB_STATE}" ]; then
    echo "release not found" >&2
    exit 1
  fi
  jq '{id, body, isDraft}' "${GH_STUB_STATE}"
  ;;
edit)
  body=""
  draft=""
  while [ $# -gt 0 ]; do
    case "${1}" in
    --notes) body="${2}"; shift 2 ;;
    --draft=false) draft=false; shift ;;
    --draft=true) draft=true; shift ;;
    *) shift ;;
    esac
  done
  tmp="$(mktemp)"
  jq --arg b "${body}" --argjson d "${draft:-null}" '
    if $d == null then (if $b == "" then . else .body = $b end)
    else (if $b == "" then .isDraft = $d else .body = $b | .isDraft = $d end)
    end' "${GH_STUB_STATE}" > "${tmp}"
  mv "${tmp}" "${GH_STUB_STATE}"
  ;;
create)
  notes=""
  while [ $# -gt 0 ]; do
    case "${1}" in
    --notes) notes="${2}"; shift 2 ;;
    *) shift ;;
    esac
  done
  if [ -f "${GH_STUB_STATE}" ]; then
    echo "A release with the tag '${tag}' already exists" >&2
    exit 1
  fi
  jq -n --arg b "${notes}" '{"id":1,"body":$b,"isDraft":false}' > "${GH_STUB_STATE}"
  ;;
*)
  echo "unknown gh subcommand: ${cmd}" >&2
  exit 2
  ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWorkflowDecisionSourceIdentity(t *testing.T) {
	for _, template := range []string{"github-release.yml", "forgejo-release.yml"} {
		t.Run(template, func(t *testing.T) {
			for _, test := range []struct {
				name      string
				requested string
				ambiguous bool
				wantError bool
			}{
				{name: "reuse lower source reservation after higher ancestor"},
				{name: "refuse a second manual version", requested: "v1.0.1", wantError: true},
				{name: "refuse ambiguous source reservations", ambiguous: true, wantError: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					origin := buildReservationOrigin(t, t.TempDir())
					git := func(args ...string) string {
						t.Helper()
						out, err := gitRun(origin, args...)
						if err != nil {
							t.Fatalf("git %v: %v\n%s", args, err, out)
						}
						return strings.TrimSpace(string(out))
					}
					git("tag", "-a", "v2.0.0", "v0.1.0", "-m", "higher ancestor reservation")
					git("tag", "-a", "v1.0.0", "HEAD", "-m", "existing source reservation")
					if test.ambiguous {
						git("tag", "-a", "v1.0.1", "HEAD", "-m", "conflicting source reservation")
					}
					version, err := workflowDecision(t, template, origin, git("rev-parse", "HEAD"), test.requested)
					if test.wantError {
						var exit *exec.ExitError
						if !errors.As(err, &exit) || exit.ExitCode() != 1 || version != "" {
							t.Fatalf("conflicting identity must fail without a version: output=%q error=%v", version, err)
						}
					} else if err != nil || version != "version=v1.0.0" {
						t.Fatalf("source must retain v1.0.0 despite higher ancestor: output=%q error=%v", version, err)
					}
				})
			}
		})
	}
}
