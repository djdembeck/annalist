package forgejo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/engine"
	"github.com/djdembeck/annalist/internal/llm"
	"github.com/djdembeck/annalist/internal/pipeline"
)

const forgejoSourceSHA = "1111111111111111111111111111111111111111"
const forgejoSecondSHA = "2222222222222222222222222222222222222222"
const forgejoMergeSHA = "3333333333333333333333333333333333333333"

func forgejoSourcePull(root, sha string) map[string]any {
	return map[string]any{
		"number": 41, "html_url": root + "/o/r/pulls/41", "merged": true,
		"merge_commit_sha": sha,
		"base":             map[string]any{"repo": map[string]string{"full_name": "o/r"}},
	}
}

func missingForgejoPull(w http.ResponseWriter) {
	okJSON(w, http.StatusNotFound, map[string]string{"message": "pull request does not exist [id: 0, index: 0, head_repo_id: 0, base_repo_id: 7, head_branch: , base_branch: ]"})
}

func TestForgejoCommitAssociationUsesSingularEndpointAndConfiguredRoot(t *testing.T) {
	var root string
	_, client, log := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forge/api/v1/repos/o/r/commits/"+forgejoMergeSHA+"/pull" {
			t.Fatalf("unexpected route: %s", r.URL)
		}
		okJSON(w, 200, forgejoSourcePull(root, forgejoMergeSHA))
	})
	root = client.cfg.URL + "/forge"
	client.cfg.URL = root + "/"
	refs, err := client.ResolveCommitReferences(context.Background(), "o", "r", []string{forgejoMergeSHA}, nil)
	if err != nil || len(refs[forgejoMergeSHA].PullRequests) != 1 || refs[forgejoMergeSHA].PullRequests[0].URL != root+"/o/r/pulls/41" || refs[forgejoMergeSHA].CommitURL != "" {
		t.Fatalf("refs=%+v err=%v", refs, err)
	}
	if request := log.first(); request == nil || request.method != http.MethodGet || request.auth != "token secret-token" {
		t.Fatalf("association API was not authenticated: %+v", request)
	}
}

func TestForgejoBranchAndInteriorCommitsUsePaginatedVerifiedMembership(t *testing.T) {
	var root string
	_, client, log := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/commits/" + forgejoSourceSHA + "/pull", "/api/v1/repos/o/r/commits/" + forgejoSecondSHA + "/pull":
			missingForgejoPull(w)
		case "/api/v1/repos/o/r/commits/" + forgejoMergeSHA + "/pull":
			okJSON(w, 200, forgejoSourcePull(root, forgejoMergeSHA))
		case "/api/v1/repos/o/r/pulls/41/commits":
			if r.URL.Query().Get("limit") != "50" {
				t.Errorf("unbounded membership request: %s", r.URL)
			}
			if r.URL.Query().Get("page") == "1" {
				commits := make([]map[string]string, 50)
				for i := range commits {
					commits[i] = map[string]string{"sha": fmt.Sprintf("%040x", i+1000)}
				}
				okJSON(w, 200, commits)
			} else {
				okJSON(w, 200, []map[string]string{{"sha": forgejoSourceSHA}, {"sha": forgejoSecondSHA}})
			}
		default:
			t.Fatalf("unexpected fallback, guessed PR, or repo-wide scan: %s", r.URL)
		}
	})
	root = client.cfg.URL
	refs, err := client.ResolveCommitReferences(context.Background(), "o", "r", []string{forgejoSourceSHA, forgejoSecondSHA}, []string{forgejoMergeSHA})
	if err != nil || len(refs) != 2 || len(log.reqs) != 5 {
		t.Fatalf("refs=%+v requests=%d err=%v", refs, len(log.reqs), err)
	}
	for _, sha := range []string{forgejoSourceSHA, forgejoSecondSHA} {
		if ref := refs[sha]; len(ref.PullRequests) != 1 || ref.PullRequests[0].Number != 41 || ref.CommitURL != "" {
			t.Fatalf("branch source linked incorrectly: sha=%s ref=%+v", sha, ref)
		}
	}
}

func TestForgejo404PolicyRequiresExplicitMissingRecord(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			var root string
			commitLookups := 0
			_, client, _ := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/repos/o/r/commits/" + forgejoSourceSHA + "/pull":
					if explicit {
						missingForgejoPull(w)
					} else {
						okJSON(w, 404, map[string]string{"message": "Not Found"})
					}
				case "/api/v1/repos/o/r/git/commits/" + forgejoSourceSHA:
					commitLookups++
					okJSON(w, 200, map[string]string{"sha": forgejoSourceSHA, "html_url": root + "/o/r/commit/" + forgejoSourceSHA})
				default:
					t.Fatalf("unexpected request: %s", r.URL)
				}
			})
			root = client.cfg.URL
			refs, err := client.ResolveCommitReferences(context.Background(), "o", "r", []string{forgejoSourceSHA}, nil)
			if explicit {
				if err != nil || commitLookups != 1 || refs[forgejoSourceSHA].CommitURL != root+"/o/r/commit/"+forgejoSourceSHA {
					t.Fatalf("explicit recorded absence did not retain the real commit URL: refs=%+v err=%v", refs, err)
				}
			} else if err == nil || refs != nil || commitLookups != 0 {
				t.Fatalf("ambiguous 404 became no association: refs=%+v err=%v commit_lookups=%d", refs, err, commitLookups)
			}
		})
	}
}

func TestForgejoSourceErrorsNeverBecomeCommitFallback(t *testing.T) {
	for _, failure := range []string{"forbidden", "membership", "wrong host", "wrong repo", "wrong merge SHA"} {
		t.Run(failure, func(t *testing.T) {
			var root string
			commitLookups := 0
			_, client, _ := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/repos/o/r/commits/" + forgejoSourceSHA + "/pull":
					if failure == "forbidden" {
						okJSON(w, 403, map[string]string{"message": "access denied"})
					} else {
						missingForgejoPull(w)
					}
				case "/api/v1/repos/o/r/commits/" + forgejoMergeSHA + "/pull":
					pr := forgejoSourcePull(root, forgejoMergeSHA)
					switch failure {
					case "wrong host":
						pr["html_url"] = "https://evil.invalid/o/r/pulls/41"
					case "wrong repo":
						pr["base"] = map[string]any{"repo": map[string]string{"full_name": "other/project"}}
					case "wrong merge SHA":
						pr["merge_commit_sha"] = forgejoSecondSHA
					}
					okJSON(w, 200, pr)
				case "/api/v1/repos/o/r/pulls/41/commits":
					okJSON(w, 502, map[string]string{"message": "membership unavailable"})
				default:
					commitLookups++
					okJSON(w, 200, map[string]string{"sha": forgejoSourceSHA, "html_url": root + "/o/r/commit/" + forgejoSourceSHA})
				}
			})
			root = client.cfg.URL
			refs, err := client.ResolveCommitReferences(context.Background(), "o", "r", []string{forgejoSourceSHA}, []string{forgejoMergeSHA})
			if err == nil || refs != nil || commitLookups != 0 {
				t.Fatalf("unresolved source fell back: refs=%+v err=%v commit_lookups=%d", refs, err, commitLookups)
			}
		})
	}
}

type localCitationPlatform struct {
	*Client
	origin string
}

func (p *localCitationPlatform) CloneInfo(context.Context, string, string) (string, string, error) {
	return p.origin, "", nil
}

func TestForgejoNormalMergeReleaseLinksEverySelectedBranchCommit(t *testing.T) {
	origin := t.TempDir()
	gitNoAsk(t, origin, "init", "-q", "--initial-branch=main")
	gitNoAsk(t, origin, "commit", "--allow-empty", "-m", "chore: baseline")
	gitNoAsk(t, origin, "tag", "v1.0.0")
	gitNoAsk(t, origin, "checkout", "-b", "archive")
	var shas []string
	for _, subject := range []string{"feat: export verified archives", "perf: parse audio headers once"} {
		if err := os.WriteFile(filepath.Join(origin, "source.txt"), []byte(subject), 0o600); err != nil {
			t.Fatal(err)
		}
		gitNoAsk(t, origin, "add", ".")
		gitNoAsk(t, origin, "commit", "-m", subject)
		shas = append(shas, engine.TargetCommitSHA(context.Background(), origin, "HEAD"))
	}
	gitNoAsk(t, origin, "checkout", "main")
	gitNoAsk(t, origin, "merge", "--no-ff", "archive", "-m", "Merge archive work")
	gitNoAsk(t, origin, "tag", "v1.1.0")
	mergeSHA := engine.TargetCommitSHA(context.Background(), origin, "HEAD")
	var root, published string
	associationLookups := 0
	_, client, _ := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/contents/.forgejo/release-notes.md":
			okJSON(w, 404, map[string]string{"message": "Not Found"})
		case "/api/v1/repos/o/r/commits/" + shas[0] + "/pull", "/api/v1/repos/o/r/commits/" + shas[1] + "/pull":
			associationLookups++
			missingForgejoPull(w)
		case "/api/v1/repos/o/r/commits/" + mergeSHA + "/pull":
			associationLookups++
			okJSON(w, 200, forgejoSourcePull(root, mergeSHA))
		case "/api/v1/repos/o/r/pulls/41/commits":
			associationLookups++
			okJSON(w, 200, []map[string]string{{"sha": shas[0]}, {"sha": shas[1]}})
		case "/api/v1/repos/o/r/releases/tags/v1.1.0":
			okJSON(w, 200, map[string]any{"id": 7, "body": ""})
		case "/api/v1/repos/o/r/releases/7":
			var body struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			published = body.Body
			okJSON(w, 200, map[string]any{})
		default:
			t.Errorf("selected branch commit lost its PR provenance: %s", r.URL)
			okJSON(w, 500, map[string]string{"message": "unexpected route"})
		}
	})
	root = client.cfg.URL
	raw := "- Parse audio headers once. " + engine.SourceToken(shas[1]) + "\n- Export verified archives. " + engine.SourceToken(shas[0])
	modelCalls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls++
		okJSON(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": raw}}}})
	}))
	defer model.Close()
	dir := t.TempDir()
	store, err := db.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := &config.Config{Data: config.DataConfig{Dir: dir}, Forgejo: config.ForgejoConfig{URL: root}, LLM: config.LLMConfig{BaseURL: model.URL, Model: "controlled-model"}}
	platform := &localCitationPlatform{Client: client, origin: origin}
	pip := pipeline.New(cfg, store, llm.New(cfg.LLM), nil, platform)
	spec := pipeline.Spec{Platform: "forgejo", Owner: "o", Repo: "r", ToTag: "v1.1.0", FromTag: "v1.0.0", ReleaseID: "merge-citations"}
	result, err := pip.GenerateNotes(context.Background(), spec, pipeline.Options{Publish: true})
	want := "- Parse audio headers once. ([#41](" + root + "/o/r/pulls/41))\n- Export verified archives. ([#41](" + root + "/o/r/pulls/41))"
	if err != nil || result.Outcome != "published" || result.Notes != want || !strings.HasPrefix(published, want) || associationLookups != 4 || modelCalls != 1 {
		t.Fatalf("normal merge failed: result=%+v body=%q lookups=%d model_calls=%d err=%v", result, published, associationLookups, modelCalls, err)
	}
	cached, err := pip.GenerateNotes(context.Background(), spec, pipeline.Options{})
	if err != nil || cached.Outcome != "cached" || cached.Notes != want || associationLookups != 4 || modelCalls != 1 {
		t.Fatalf("cached merge provenance changed: result=%+v lookups=%d err=%v", cached, associationLookups, err)
	}
}
