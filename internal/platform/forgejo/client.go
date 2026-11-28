// Package forgejo implements the Forgejo (and Gitea-compatible) hosting
// platform client and webhook handler, talking plain REST over net/http.
package forgejo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/pipeline"
)

// defaultReposPerPage is the pagination limit for listing repositories.
const defaultReposPerPage = 50

// Client talks to a Forgejo/Gitea instance over its REST API.
type Client struct {
	cfg  config.ForgejoConfig
	http *http.Client
}

// New builds a Forgejo client from the given configuration.
func New(cfg config.ForgejoConfig) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// baseURL returns the trimmed instance root plus /api/v1.
func (c *Client) baseURL() string {
	return strings.TrimSuffix(c.cfg.URL, "/") + "/api/v1"
}

// do performs an authenticated API request: builds the URL from path (which
// may contain raw segments like /repos/{owner}/{repo}/...), enforces 2xx,
// wraps 404 in pipeline.ErrNotFound, and decodes the JSON response into out
// (which may be nil when no body is expected).
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	base := strings.TrimSuffix(c.cfg.URL, "/")
	u := base + "/api/v1" + path

	var reqBody io.Reader
	if body != nil {
		enc, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("forgejo: encode request body: %w", err)
		}
		reqBody = bytes.NewReader(enc)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return fmt.Errorf("forgejo: build request: %w", err)
	}
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "token "+c.cfg.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("forgejo: %s %s: %w", method, u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&payload)
		return &notFoundError{message: payload.Message}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("forgejo: %s %s: unexpected status %s: %s", method, u, resp.Status, strings.TrimSpace(string(msg)))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("forgejo: decode %s %s response: %w", method, u, err)
		}
	}
	return nil
}

type notFoundError struct{ message string }

func (e *notFoundError) Error() string { return "forgejo: resource not found" }
func (e *notFoundError) Unwrap() error { return pipeline.ErrNotFound }

// Forgejo's GetCommitPullRequest handler returns this model error only after
// an authenticated repository lookup succeeds. A generic route/auth 404 does
// not establish the absence of a recorded merged-commit association.
func missingPullRecord(err error) bool {
	var missing *notFoundError
	return errors.As(err, &missing) && strings.HasPrefix(missing.message, "pull request does not exist [")
}

// apiPath escapes a path segment for use inside a URL path.
func apiPath(seg string) string {
	return url.PathEscape(seg)
}

// ListRepos returns the authenticated user's repositories, paginating over all
// pages until a partial page is returned.
// currentUser returns the login of the authenticated user, resolving the
// /user endpoint's username fallback for Gitea-compatible instances.
func (c *Client) currentUser(ctx context.Context) (string, error) {
	var resp struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	}
	if err := c.do(ctx, http.MethodGet, "/user", nil, &resp); err != nil {
		return "", err
	}
	if resp.Login != "" {
		return resp.Login, nil
	}
	return resp.Username, nil
}

// ListRepos returns the authenticated user's repositories, paginating over all
// pages until a partial page is returned.
func (c *Client) ListRepos(ctx context.Context) ([]pipeline.OwnerRepo, error) {
	user, err := c.currentUser(ctx)
	if err != nil {
		return nil, err
	}
	var out []pipeline.OwnerRepo
	page := 1
	for {
		var batch []struct {
			FullName string `json:"full_name"`
			Name     string `json:"name"`
			Fork     bool   `json:"fork"`
			Owner    struct {
				Login    string `json:"login"`
				Username string `json:"username"`
			} `json:"owner"`
			UpdatedAt time.Time `json:"updated_at"`
			PushedAt  time.Time `json:"pushed_at,omitempty"`
		}
		path := "/user/repos?limit=" + strconv.Itoa(defaultReposPerPage) + "&page=" + strconv.Itoa(page)
		if err := c.do(ctx, http.MethodGet, path, nil, &batch); err != nil {
			return nil, err
		}
		for _, r := range batch {
			owner := r.Owner.Login
			if owner == "" {
				owner = r.Owner.Username
			}
			out = append(out, pipeline.OwnerRepo{
				Owner:        owner,
				Repo:         r.Name,
				Fork:         r.Fork,
				OwnNamespace: owner == user,
				UpdatedAt:    r.UpdatedAt,
				PushedAt:     r.PushedAt,
			})
			if out[len(out)-1].PushedAt.IsZero() {
				out[len(out)-1].PushedAt = r.UpdatedAt
			}
		}
		if len(batch) < defaultReposPerPage {
			break
		}
		page++
	}
	return out, nil
}

// ReadRepoFile reads a file's content from a repository via the contents API,
// pinned to ref when it is non-empty (ref == "" reads the default branch).
// A 404 is surfaced as pipeline.ErrNotFound.
func (c *Client) ReadRepoFile(ctx context.Context, owner, repo, path, ref string) (string, error) {
	var resp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = apiPath(s)
	}
	p := "/repos/" + apiPath(owner) + "/" + apiPath(repo) + "/contents/" + strings.Join(segments, "/")
	if ref != "" {
		p += "?ref=" + url.QueryEscape(ref)
	}
	if err := c.do(ctx, http.MethodGet, p, nil, &resp); err != nil {
		return "", err
	}
	if resp.Encoding == "" || resp.Encoding == "base64" {
		raw, err := base64.StdEncoding.DecodeString(resp.Content)
		if err != nil {
			return "", fmt.Errorf("forgejo: decode base64 content: %w", err)
		}
		return string(raw), nil
	}
	return resp.Content, nil
}

// ListReleases returns every release listed for the repository (drafts and
// prereleases included), paginated over all pages.
func (c *Client) ListReleases(ctx context.Context, owner, repo string) ([]pipeline.ReleaseMeta, error) {
	prefix := "/repos/" + apiPath(owner) + "/" + apiPath(repo) + "/releases?limit=" +
		strconv.Itoa(defaultReposPerPage)
	var out []pipeline.ReleaseMeta
	page := 1
	for {
		var batch []struct {
			TagName    string `json:"tag_name"`
			Draft      bool   `json:"draft"`
			Prerelease bool   `json:"prerelease"`
		}
		if err := c.do(ctx, http.MethodGet, prefix+"&page="+strconv.Itoa(page), nil, &batch); err != nil {
			return nil, err
		}
		for _, r := range batch {
			out = append(out, pipeline.ReleaseMeta{Tag: r.TagName, Draft: r.Draft, Prerelease: r.Prerelease})
		}
		if len(batch) < defaultReposPerPage {
			return out, nil
		}
		page++
	}
}

// GetReleaseByTag fetches a release by its tag. A 404 is surfaced as
// pipeline.ErrNotFound.
func (c *Client) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*pipeline.Release, error) {
	var resp struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	p := "/repos/" + apiPath(owner) + "/" + apiPath(repo) + "/releases/tags/" + apiPath(tag)
	if err := c.do(ctx, http.MethodGet, p, nil, &resp); err != nil {
		return nil, err
	}
	return &pipeline.Release{ID: resp.ID, Body: resp.Body}, nil
}

// EditReleaseBody replaces a release's body.
func (c *Client) EditReleaseBody(ctx context.Context, owner, repo string, releaseID int64, body string) error {
	p := "/repos/" + apiPath(owner) + "/" + apiPath(repo) + "/releases/" + strconv.FormatInt(releaseID, 10)
	return c.do(ctx, http.MethodPatch, p, map[string]string{"body": body}, nil)
}

type associatedPull struct {
	Number         int    `json:"number"`
	HTMLURL        string `json:"html_url"`
	Merged         bool   `json:"merged"`
	MergeCommitSHA string `json:"merge_commit_sha"`
	Base           struct {
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

func (c *Client) pullReference(owner, repo string, pr associatedPull) (pipeline.PullRequestReference, error) {
	if !pr.Merged || pr.MergeCommitSHA == "" || pr.Number <= 0 || !strings.EqualFold(pr.Base.Repo.FullName, owner+"/"+repo) {
		return pipeline.PullRequestReference{}, errors.New("forgejo: response does not establish a merged association in the requested repository")
	}
	if err := pipeline.ValidateSourceURL(c.cfg.URL, owner, repo, "pulls", strconv.Itoa(pr.Number), pr.HTMLURL); err != nil {
		return pipeline.PullRequestReference{}, fmt.Errorf("forgejo: %w", err)
	}
	return pipeline.PullRequestReference{Number: pr.Number, URL: pr.HTMLURL}, nil
}

// ResolveCommitReferences uses the exact merged-commit route first. Branch
// commits are checked against the verified PRs of merges from the same Git log.
// Only Forgejo's explicit missing-record response permits a commit fallback;
// generic 404s, unavailable membership, and other API errors remain unresolved.
func (c *Client) ResolveCommitReferences(ctx context.Context, owner, repo string, shas, mergeSHAs []string) (map[string]pipeline.CommitReference, error) {
	refs := make(map[string]pipeline.CommitReference, len(shas))
	if len(shas) == 0 {
		return refs, nil
	}
	if c.cfg.Token == "" {
		return nil, errors.New("forgejo: no token configured for source lookup")
	}
	prefix := "/repos/" + apiPath(owner) + "/" + apiPath(repo)
	lookup := func(sha string) (*associatedPull, error) {
		var pr associatedPull
		path := prefix + "/commits/" + apiPath(sha) + "/pull"
		if err := c.do(ctx, http.MethodGet, path, nil, &pr); err != nil {
			if missingPullRecord(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("forgejo: commit pull-request lookup is unresolved (a generic 404 is not a missing association): %w", err)
		}
		if pr.MergeCommitSHA != sha {
			return nil, errors.New("forgejo: merged association has a different commit SHA")
		}
		return &pr, nil
	}
	pending := make(map[string]bool)
	for _, sha := range shas {
		pr, err := lookup(sha)
		if err != nil {
			return nil, err
		}
		if pr == nil {
			pending[sha] = true
			continue
		}
		ref, err := c.pullReference(owner, repo, *pr)
		if err != nil {
			return nil, err
		}
		refs[sha] = pipeline.CommitReference{PullRequests: []pipeline.PullRequestReference{ref}}
	}
	if len(pending) == 0 {
		return refs, nil
	}
	seen := make(map[int]string)
	for _, mergeSHA := range mergeSHAs {
		pr, err := lookup(mergeSHA)
		if err != nil {
			return nil, err
		}
		if pr == nil {
			continue
		}
		ref, err := c.pullReference(owner, repo, *pr)
		if err != nil {
			return nil, err
		}
		if previous, ok := seen[pr.Number]; ok {
			if previous != ref.URL {
				return nil, errors.New("forgejo: inconsistent merged pull-request association")
			}
			continue
		}
		seen[pr.Number] = ref.URL
		for page := 1; ; page++ {
			var commits []struct {
				SHA string `json:"sha"`
			}
			path := prefix + "/pulls/" + strconv.Itoa(pr.Number) + "/commits?limit=" + strconv.Itoa(defaultReposPerPage) + "&page=" + strconv.Itoa(page)
			if err := c.do(ctx, http.MethodGet, path, nil, &commits); err != nil {
				return nil, fmt.Errorf("forgejo: cannot establish merged PR commit membership: %w", err)
			}
			if commits == nil || (page == 1 && len(commits) == 0) {
				return nil, errors.New("forgejo: merged PR commit membership is unavailable")
			}
			for _, commit := range commits {
				if !pending[commit.SHA] {
					continue
				}
				current := refs[commit.SHA]
				duplicate := false
				for _, existing := range current.PullRequests {
					if existing.Number == ref.Number {
						duplicate = true
						break
					}
				}
				if !duplicate {
					current.PullRequests = append(current.PullRequests, ref)
				}
				refs[commit.SHA] = current
			}
			if len(commits) < defaultReposPerPage {
				break
			}
		}
	}
	for _, sha := range shas {
		if !pending[sha] || len(refs[sha].PullRequests) != 0 {
			continue
		}
		var commit struct {
			SHA     string `json:"sha"`
			HTMLURL string `json:"html_url"`
		}
		path := prefix + "/git/commits/" + apiPath(sha) + "?stat=false&verification=false&files=false"
		if err := c.do(ctx, http.MethodGet, path, nil, &commit); err != nil {
			return nil, fmt.Errorf("forgejo: commit URL lookup: %w", err)
		}
		if commit.SHA != sha {
			return nil, errors.New("forgejo: commit response has a different SHA")
		}
		if err := pipeline.ValidateSourceURL(c.cfg.URL, owner, repo, "commit", sha, commit.HTMLURL); err != nil {
			return nil, fmt.Errorf("forgejo: %w", err)
		}
		refs[sha] = pipeline.CommitReference{CommitURL: commit.HTMLURL}
	}
	return refs, nil
}

// CloneInfo returns the clone URL and the Authorization header value needed to
// clone the repo. It returns an error when no token is configured.
func (c *Client) CloneInfo(ctx context.Context, owner, repo string) (cloneURL, header string, err error) {
	if c.cfg.Token == "" {
		return "", "", errors.New("forgejo: no token configured for cloning")
	}
	cloneURL = strings.TrimSuffix(c.cfg.URL, "/") + "/" + owner + "/" + repo + ".git"
	header = "token " + c.cfg.Token
	return cloneURL, header, nil
}
