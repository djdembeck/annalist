package forgejo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/pipeline"
)

// forgejoServer starts an httptest server, wires a Client to it, and records
// every request so tests can assert method/path/headers/body.
func forgejoServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client, *requestLog) {
	t.Helper()
	log := &requestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	client := New(config.ForgejoConfig{URL: srv.URL, Token: "secret-token"})
	return srv, client, log
}

type requestLog struct {
	reqs []*recordedRequest
}

type recordedRequest struct {
	method string
	path   string
	auth   string
	body   string
}

func (l *requestLog) record(r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(raw)))
	l.reqs = append(l.reqs, &recordedRequest{
		method: r.Method,
		path:   r.URL.RequestURI(),
		auth:   r.Header.Get("Authorization"),
		body:   string(raw),
	})
}

func (l *requestLog) first() *recordedRequest {
	if len(l.reqs) == 0 {
		return nil
	}
	return l.reqs[0]
}

func okJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// repoPageJSON builds a page of ListRepos response objects numbered 0..n-1.
func repoPageJSON(n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range n {
		out[i] = map[string]any{
			"full_name": fmt.Sprintf("owner%d/repo%d", i, i),
			"name":      fmt.Sprintf("repo%d", i),
			"owner": map[string]any{
				"login":    fmt.Sprintf("owner%d", i),
				"username": "",
			},
		}
	}
	return out
}

func TestNew(t *testing.T) {
	c := New(config.ForgejoConfig{URL: "https://example.com/"})
	if c == nil {
		t.Fatal("New returned nil client")
	}
	if got, want := c.baseURL(), "https://example.com/api/v1"; got != want {
		t.Errorf("baseURL() = %q, want %q", got, want)
	}
}

func TestListReposPartialPageStopsPagination(t *testing.T) {
	_, client, log := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user":
			okJSON(w, 200, map[string]any{"login": "owner0"})
		case "/api/v1/user/repos":
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Errorf("page = %q, want 1", got)
			}
			okJSON(w, 200, repoPageJSON(2))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})

	repos, err := client.ListRepos(context.Background())
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	want := []pipeline.OwnerRepo{
		{Owner: "owner0", Repo: "repo0", OwnNamespace: true},
		{Owner: "owner1", Repo: "repo1"},
	}
	if len(log.reqs) != 2 {
		t.Errorf("expected 2 requests (current user + partial page), got %d", len(log.reqs))
	}
	if !reflect.DeepEqual(repos, want) {
		t.Errorf("ListRepos = %+v, want %+v", repos, want)
	}
}

func TestListReposPaginatesUntilPartialPage(t *testing.T) {
	// Page 1 returns exactly 50 (a full page) forcing a second request; page 2
	// returns an empty (partial) page, stopping the loop.
	_, client, log := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			okJSON(w, 200, map[string]any{"login": "someone"})
			return
		}
		switch r.URL.Query().Get("page") {
		case "1":
			okJSON(w, 200, repoPageJSON(50))
		case "2":
			okJSON(w, 200, []map[string]any{})
		default:
			t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
			okJSON(w, 200, []map[string]any{})
		}
	})

	repos, err := client.ListRepos(context.Background())
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 50 {
		t.Errorf("got %d repos, want 50", len(repos))
	}
	if len(log.reqs) != 3 {
		t.Errorf("expected 3 requests (current user + full page + empty page), got %d", len(log.reqs))
	}
	if got := log.reqs[1].path; !strings.Contains(got, "page=1") {
		t.Errorf("first page request = %q, want page=1", got)
	}
	if got := log.reqs[2].path; !strings.Contains(got, "page=2") {
		t.Errorf("second page request = %q, want page=2", got)
	}
	if got := log.reqs[1].auth; got != "token secret-token" {
		t.Errorf("Authorization = %q, want %q", got, "token secret-token")
	}
}

func TestReadRepoFileDecodesBase64(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("hello world"))
	_, client, log := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		want := "/api/v1/repos/owner/my-repo/contents/dir/file.txt"
		if got := r.URL.Path; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		okJSON(w, 200, map[string]any{"content": content, "encoding": "base64"})
	})

	got, err := client.ReadRepoFile(context.Background(), "owner", "my-repo", "dir/file.txt")
	if err != nil {
		t.Fatalf("ReadRepoFile: %v", err)
	}
	if got != "hello world" {
		t.Errorf("ReadRepoFile = %q, want %q", got, "hello world")
	}
	if req := log.first(); req == nil || req.method != http.MethodGet {
		t.Errorf("expected GET request, got %v", req)
	}
}

func TestReadRepoFileEscapesPathSegments(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("x"))
	_, client, _ := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		want := "/api/v1/repos/own%20er/my-repo/contents/my%20file.txt"
		if got := r.URL.EscapedPath(); got != want {
			t.Errorf("escaped path = %q, want %q", got, want)
		}
		okJSON(w, 200, map[string]any{"content": content, "encoding": "base64"})
	})
	if _, err := client.ReadRepoFile(context.Background(), "own er", "my-repo", "my file.txt"); err != nil {
		t.Fatalf("ReadRepoFile: %v", err)
	}
}

func TestReadRepoFileNotFound(t *testing.T) {
	_, client, _ := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	_, err := client.ReadRepoFile(context.Background(), "owner", "repo", "missing.txt")
	if !errors.Is(err, pipeline.ErrNotFound) {
		t.Errorf("ReadRepoFile error = %v, want pipeline.ErrNotFound", err)
	}
}

func TestReadRepoFileBadBase64(t *testing.T) {
	_, client, _ := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		okJSON(w, 200, map[string]any{"content": "!!!not-base64!!!", "encoding": "base64"})
	})
	_, err := client.ReadRepoFile(context.Background(), "owner", "repo", "file.txt")
	if err == nil {
		t.Fatal("expected base64 decode error, got nil")
	}
}

func TestGetReleaseByTag(t *testing.T) {
	_, client, log := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		want := "/api/v1/repos/owner/repo/releases/tags/v1.2.3"
		if got := r.URL.Path; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		okJSON(w, 200, map[string]any{"id": 42, "body": "release body"})
	})

	rel, err := client.GetReleaseByTag(context.Background(), "owner", "repo", "v1.2.3")
	if err != nil {
		t.Fatalf("GetReleaseByTag: %v", err)
	}
	want := &pipeline.Release{ID: 42, Body: "release body"}
	if !reflect.DeepEqual(rel, want) {
		t.Errorf("GetReleaseByTag = %+v, want %+v", rel, want)
	}
	if req := log.first(); req == nil || req.method != http.MethodGet {
		t.Errorf("expected GET request, got %v", req)
	}
}

func TestGetReleaseByTagNotFound(t *testing.T) {
	_, client, _ := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	_, err := client.GetReleaseByTag(context.Background(), "owner", "repo", "v0.0.0")
	if !errors.Is(err, pipeline.ErrNotFound) {
		t.Errorf("GetReleaseByTag error = %v, want pipeline.ErrNotFound", err)
	}
}

func TestEditReleaseBodyIssuesPATCH(t *testing.T) {
	_, client, log := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		want := "/api/v1/repos/owner/repo/releases/123"
		if got := r.URL.Path; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q, want PATCH", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["body"] != "updated" {
			t.Errorf("request body = %v, want body=updated", body)
		}
		w.WriteHeader(http.StatusOK)
	})

	err := client.EditReleaseBody(context.Background(), "owner", "repo", 123, "updated")
	if err != nil {
		t.Fatalf("EditReleaseBody: %v", err)
	}
	if req := log.first(); req == nil || req.method != http.MethodPatch {
		t.Errorf("expected PATCH request, got %v", req)
	}
}

func TestEditReleaseBodyNotFound(t *testing.T) {
	_, client, _ := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	err := client.EditReleaseBody(context.Background(), "owner", "repo", 55, "x")
	if !errors.Is(err, pipeline.ErrNotFound) {
		t.Errorf("EditReleaseBody error = %v, want pipeline.ErrNotFound", err)
	}
}

func TestEditReleaseBodyUnexpectedStatus(t *testing.T) {
	_, client, _ := forgejoServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	err := client.EditReleaseBody(context.Background(), "owner", "repo", 7, "x")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	if errors.Is(err, pipeline.ErrNotFound) {
		t.Errorf("500 should not map to ErrNotFound, got %v", err)
	}
}

func TestCloneInfo(t *testing.T) {
	client := New(config.ForgejoConfig{URL: "https://forgejo.example.com/", Token: "tok"})
	url, header, err := client.CloneInfo(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("CloneInfo: %v", err)
	}
	if url != "https://forgejo.example.com/owner/repo.git" {
		t.Errorf("clone URL = %q, want %q", url, "https://forgejo.example.com/owner/repo.git")
	}
	if header != "token tok" {
		t.Errorf("header = %q, want %q", header, "token tok")
	}
}

func TestCloneInfoNoToken(t *testing.T) {
	client := New(config.ForgejoConfig{URL: "https://forgejo.example.com"})
	_, _, err := client.CloneInfo(context.Background(), "owner", "repo")
	if err == nil {
		t.Fatal("expected error without token, got nil")
	}
}
