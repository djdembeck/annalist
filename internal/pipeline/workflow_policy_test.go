package pipeline

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
