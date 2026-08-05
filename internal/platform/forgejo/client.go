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
		return pipeline.ErrNotFound
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
		}
		path := "/user/repos?limit=50&page=" + strconv.Itoa(page)
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
			})
		}
		if len(batch) < 50 {
			break
		}
		page++
	}
	return out, nil
}

// ReadRepoFile reads a file's content from a repository via the contents API.
// A 404 is surfaced as pipeline.ErrNotFound.
func (c *Client) ReadRepoFile(ctx context.Context, owner, repo, path string) (string, error) {
	var resp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = apiPath(s)
	}
	p := "/repos/" + apiPath(owner) + "/" + apiPath(repo) + "/contents/" + strings.Join(segments, "/")
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
