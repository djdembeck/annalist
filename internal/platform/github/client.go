// Package github implements the GitHub hosting platform: a GitHub App client
// (go-github + ghinstallation) and the release-webhook handler.
package github

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v89/github"

	"github.com/bradleyfalzon/ghinstallation/v2"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/pipeline"
)

// Client talks to GitHub as a GitHub App. Credentials are validated lazily on
// first use so an unconfigured (but disabled) platform never fails at startup.
type Client struct {
	cfg config.GitHubConfig
}

// New builds a GitHub client from the given configuration.
func New(cfg config.GitHubConfig) *Client {
	return &Client{cfg: cfg}
}

// appsClient builds a Client authenticated as the GitHub App itself (using its
// JWT), able to enumerate installations. It has no per-repo access.
func (c *Client) appsClient() (*gogithub.Client, error) {
	if err := c.validateCreds(); err != nil {
		return nil, err
	}
	atr, err := ghinstallation.NewAppsTransportKeyFromFile(
		http.DefaultTransport, c.cfg.AppID, c.cfg.AppPrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("github: apps transport: %w", err)
	}
	client, err := gogithub.NewClient(gogithub.WithHTTPClient(&http.Client{Transport: atr}))
	if err != nil {
		return nil, fmt.Errorf("github: new apps client: %w", err)
	}
	return client, nil
}

// installationClient builds a Client authenticated as a single installation,
// scoped to the installation's granted repositories.
func (c *Client) installationClient(ctx context.Context, installID int64) (*gogithub.Client, error) {
	if err := c.validateCreds(); err != nil {
		return nil, err
	}
	itr, err := ghinstallation.NewKeyFromFile(
		http.DefaultTransport, c.cfg.AppID, installID, c.cfg.AppPrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("github: installation transport: %w", err)
	}
	client, err := gogithub.NewClient(gogithub.WithHTTPClient(&http.Client{Transport: itr}))
	if err != nil {
		return nil, fmt.Errorf("github: new installation client: %w", err)
	}
	return client, nil
}

// repoClient resolves the installation that owns owner/repo and returns an
// installation-scoped client for it. Used by every per-repo operation.
func (c *Client) repoClient(ctx context.Context, owner, repo string) (*gogithub.Client, error) {
	appClient, err := c.appsClient()
	if err != nil {
		return nil, err
	}
	inst, _, err := appClient.Apps.GetRepositoryInstallation(ctx, owner, repo)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: github installation for %s/%s", pipeline.ErrNotFound, owner, repo)
		}
		return nil, err
	}
	return c.installationClient(ctx, inst.GetID())
}

// validateCreds reports whether the GitHub App credential pair is present.
func (c *Client) validateCreds() error {
	if c.cfg.AppID == 0 || c.cfg.AppPrivateKeyFile == "" {
		return errors.New("github app credentials not configured")
	}
	return nil
}

// isNotFound reports whether err is a GitHub 404 API error.
func isNotFound(err error) bool {
	var er *gogithub.ErrorResponse
	if errors.As(err, &er) {
		return er.Response != nil && er.Response.StatusCode == http.StatusNotFound
	}
	return false
}

// ListRepos walks every installation of the GitHub App and collects the owner
// and name of each accessible repository, de-duplicated across installations.
func (c *Client) ListRepos(ctx context.Context) ([]pipeline.OwnerRepo, error) {
	appClient, err := c.appsClient()
	if err != nil {
		return nil, err
	}

	var installs []*gogithub.Installation
	opts := &gogithub.ListOptions{PerPage: 100}
	for {
		batch, resp, err := appClient.Apps.ListInstallations(ctx, opts)
		if err != nil {
			return nil, err
		}
		installs = append(installs, batch...)
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	seen := make(map[string]struct{})
	var repos []pipeline.OwnerRepo
	for _, inst := range installs {
		ic, err := c.installationClient(ctx, inst.GetID())
		if err != nil {
			return nil, err
		}
		ropts := &gogithub.ListOptions{PerPage: 100}
		for {
			list, resp, err := ic.Apps.ListRepos(ctx, ropts)
			if err != nil {
				return nil, err
			}
			// Repos from a personal installation belong to the user's own namespace.
			// Repos from an organization installation belong to a shared namespace.
			ownNamespace := inst.GetAccount().GetType() == "User"
			for _, r := range list.Repositories {
				owner := r.GetOwner().GetLogin()
				name := r.GetName()
				if owner == "" || name == "" {
					continue
				}
				key := owner + "/" + name
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				repos = append(repos, pipeline.OwnerRepo{
					Owner:        owner,
					Repo:         name,
					Fork:         r.GetFork(),
					OwnNamespace: ownNamespace,
				})
			}
			if resp == nil || resp.NextPage == 0 {
				break
			}
			ropts.Page = resp.NextPage
		}
	}
	return repos, nil
}

// ReadRepoFile returns the UTF-8 content of a file in the repository. A missing
// file, a directory, or any other 404 is reported as pipeline.ErrNotFound so
// the pipeline treats it as "no instructions file".
func (c *Client) ReadRepoFile(ctx context.Context, owner, repo, path string) (string, error) {
	client, err := c.repoClient(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	file, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		if isNotFound(err) {
			return "", fmt.Errorf("%w: github file %s", pipeline.ErrNotFound, path)
		}
		return "", err
	}
	if file == nil || file.GetType() != "file" {
		return "", fmt.Errorf("%w: github path %s is not a file", pipeline.ErrNotFound, path)
	}
	content, err := file.GetContent()
	if err != nil {
		return "", fmt.Errorf("github: decode file %s: %w", path, err)
	}
	return content, nil
}

// GetReleaseByTag fetches a release by tag, wrapping a missing release in
// pipeline.ErrNotFound.
func (c *Client) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*pipeline.Release, error) {
	client, err := c.repoClient(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	rel, _, err := client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: github release %s/%s@%s", pipeline.ErrNotFound, owner, repo, tag)
		}
		return nil, err
	}
	return &pipeline.Release{ID: rel.GetID(), Body: rel.GetBody()}, nil
}

// EditReleaseBody replaces the body of the release identified by releaseID.
func (c *Client) EditReleaseBody(ctx context.Context, owner, repo string, releaseID int64, body string) error {
	client, err := c.repoClient(ctx, owner, repo)
	if err != nil {
		return err
	}
	_, _, err = client.Repositories.UpdateRelease(ctx, owner, repo, releaseID,
		gogithub.UpdateReleaseRequest{Body: gogithub.String(body)})
	return err
}

// CloneInfo returns the URL and Authorization header the clone step uses. The
// token is minted per-installation and carried only in the header, never in the
// URL.
func (c *Client) CloneInfo(ctx context.Context, owner, repo string) (cloneURL, header string, err error) {
	appClient, err := c.appsClient()
	if err != nil {
		return "", "", err
	}
	inst, _, err := appClient.Apps.GetRepositoryInstallation(ctx, owner, repo)
	if err != nil {
		if isNotFound(err) {
			return "", "", fmt.Errorf("%w: github installation for %s/%s", pipeline.ErrNotFound, owner, repo)
		}
		return "", "", err
	}
	itr, err := ghinstallation.NewKeyFromFile(
		http.DefaultTransport, c.cfg.AppID, inst.GetID(), c.cfg.AppPrivateKeyFile)
	if err != nil {
		return "", "", fmt.Errorf("github: installation transport: %w", err)
	}
	token, err := itr.Token(ctx)
	if err != nil {
		return "", "", fmt.Errorf("github: mint installation token: %w", err)
	}
	return "https://github.com/" + owner + "/" + repo + ".git", gitAuthHeader(token), nil
}

// gitAuthHeader builds the Authorization header for git clone over HTTPS.
// GitHub's smart-HTTP git endpoint rejects Bearer tokens; it requires Basic
// auth with the reserved "x-access-token" username and the installation token
// as the password (the documented GitHub App pattern). Forgejo is the reason
// the pipeline passes the header verbatim: it accepts the same Basic form.
func gitAuthHeader(token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))
}
