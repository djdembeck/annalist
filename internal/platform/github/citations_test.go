package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/djdembeck/annalist/internal/config"
)

const sourceSHA = "1111111111111111111111111111111111111111"
const secondSourceSHA = "2222222222222222222222222222222222222222"

type sourceGitHubRT struct {
	handler               func(*http.Request) (*http.Response, error)
	installations, tokens int
}

func sourceGitHubResponse(req *http.Request, status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("Content-Type", "application/json")
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: headers, Request: req, Body: io.NopCloser(strings.NewReader(body))}
}

func (rt *sourceGitHubRT) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/repos/o/r/installation":
		rt.installations++
		return sourceGitHubResponse(req, 200, `{"id":99,"app_id":1}`, nil), nil
	case "/app/installations/99/access_tokens":
		rt.tokens++
		return sourceGitHubResponse(req, 200, `{"token":"source-token","expires_at":"2030-01-01T00:00:00Z"}`, nil), nil
	default:
		return rt.handler(req)
	}
}

func sourceGitHubClient(t *testing.T, handler func(*http.Request) (*http.Response, error)) (*Client, *sourceGitHubRT) {
	t.Helper()
	rt := &sourceGitHubRT{handler: handler}
	original := http.DefaultTransport
	http.DefaultTransport = rt
	t.Cleanup(func() { http.DefaultTransport = original })
	return New(config.GitHubConfig{AppID: 1, AppPrivateKeyFile: testAppKeyPEM(t)}), rt
}

func mergedSourcePull(number int, ownerRepo, htmlURL string) string {
	return fmt.Sprintf(`{"number":%d,"html_url":%q,"state":"closed","merged_at":"2026-11-01T12:00:00Z","merge_commit_sha":%q,"base":{"repo":{"full_name":%q}}}`, number, htmlURL, sourceSHA, ownerRepo)
}

func TestCommitAssociationsUseMergedPaginatedResultsAndOneToken(t *testing.T) {
	lookups := 0
	client, rt := sourceGitHubClient(t, func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "token source-token" && req.Header.Get("Authorization") != "Bearer source-token" {
			t.Errorf("association lookup was not installation-authenticated: %q", req.Header.Get("Authorization"))
		}
		if !strings.HasSuffix(req.URL.Path, "/pulls") || req.URL.Query().Get("per_page") != "100" {
			t.Fatalf("unexpected source lookup: %s", req.URL)
		}
		lookups++
		if req.URL.Query().Get("page") == "1" {
			headers := http.Header{"Link": []string{"<https://api.github.com" + req.URL.Path + "?page=2>; rel=\"next\""}}
			return sourceGitHubResponse(req, 200, `[{"number":9999,"state":"open","html_url":"https://github.com/o/r/pull/9999","base":{"repo":{"full_name":"o/r"}}}]`, headers), nil
		}
		body := "[" + mergedSourcePull(1779, "o/r", "https://github.com/o/r/pull/1779") + "," + mergedSourcePull(1754, "o/r", "https://github.com/o/r/pull/1754") + "]"
		return sourceGitHubResponse(req, 200, body, nil), nil
	})
	refs, err := client.ResolveCommitReferences(context.Background(), "o", "r", []string{sourceSHA, secondSourceSHA}, nil)
	if err != nil || len(refs) != 2 || lookups != 4 || rt.installations != 1 || rt.tokens != 1 {
		t.Fatalf("refs=%+v err=%v lookups=%d installations=%d tokens=%d", refs, err, lookups, rt.installations, rt.tokens)
	}
	for _, sha := range []string{sourceSHA, secondSourceSHA} {
		ref := refs[sha]
		if len(ref.PullRequests) != 2 || ref.CommitURL != "" || ref.PullRequests[0].Number != 1779 || ref.PullRequests[1].Number != 1754 {
			t.Fatalf("unmerged association selected or merged association lost: %+v", ref)
		}
	}
}

func TestGitHubEmptyAssociationsUseReturnedCommitURL(t *testing.T) {
	commitRequests := 0
	client, _ := sourceGitHubClient(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/o/r/commits/" + sourceSHA + "/pulls":
			return sourceGitHubResponse(req, 200, "[]", nil), nil
		case "/repos/o/r/commits/" + sourceSHA:
			commitRequests++
			return sourceGitHubResponse(req, 200, `{"sha":"`+sourceSHA+`","html_url":"https://github.com/o/r/commit/`+sourceSHA+`"}`, nil), nil
		default:
			t.Fatalf("unexpected lookup: %s", req.URL)
			return nil, errors.New("unexpected lookup")
		}
	})
	refs, err := client.ResolveCommitReferences(context.Background(), "o", "r", []string{sourceSHA}, nil)
	if err != nil || commitRequests != 1 || len(refs[sourceSHA].PullRequests) != 0 || refs[sourceSHA].CommitURL != "https://github.com/o/r/commit/"+sourceSHA {
		t.Fatalf("empty associations did not retain an authentic commit URL: refs=%+v requests=%d err=%v", refs, commitRequests, err)
	}
}

func TestGitHubUnresolvedAssociationsNeverBecomeCommitFallback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{name: "App lacks PR permission", status: 403, body: `{"message":"Resource not accessible by integration"}`},
		{name: "unauthorized", status: 401, body: `{"message":"Bad credentials"}`},
		{name: "not found", status: 404, body: `{"message":"Not Found"}`},
		{name: "provider failure", status: 502, body: `{"message":"unavailable"}`},
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "null is not empty array", status: 200, body: "null"},
		{name: "only open PR", status: 200, body: `[{"number":1779,"state":"open","html_url":"https://github.com/o/r/pull/1779","base":{"repo":{"full_name":"o/r"}}}]`},
		{name: "different base repo", status: 200, body: "[" + mergedSourcePull(1779, "other/repo", "https://github.com/o/r/pull/1779") + "]"},
		{name: "different URL host", status: 200, body: "[" + mergedSourcePull(1779, "o/r", "https://evil.invalid/o/r/pull/1779") + "]"},
		{name: "different URL number", status: 200, body: "[" + mergedSourcePull(1779, "o/r", "https://github.com/o/r/pull/1754") + "]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commitFallbacks := 0
			client, _ := sourceGitHubClient(t, func(req *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(req.URL.Path, "/pulls") {
					commitFallbacks++
				}
				if tc.err != nil {
					return nil, tc.err
				}
				return sourceGitHubResponse(req, tc.status, tc.body, nil), nil
			})
			refs, err := client.ResolveCommitReferences(context.Background(), "o", "r", []string{sourceSHA}, nil)
			if err == nil || refs != nil || commitFallbacks != 0 {
				t.Fatalf("unresolved lookup fell back: refs=%+v err=%v commit_requests=%d", refs, err, commitFallbacks)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("deadline cause lost: %v", err)
			}
		})
	}
}
