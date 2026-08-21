package github

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"

	gogithub "github.com/google/go-github/v89/github"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/pipeline"
)

// credsConfig builds a GitHubConfig with the given app id and key file.
func credsConfig(appID int64, keyFile string) config.GitHubConfig {
	return config.GitHubConfig{AppID: appID, AppPrivateKeyFile: keyFile}
}

func TestNew(t *testing.T) {
	c := New(credsConfig(123, "/path/key.pem"))
	if c == nil {
		t.Fatal("New returned nil client")
	}
	// The client should validate the credentials it was constructed with.
	if err := c.validateCreds(); err != nil {
		t.Errorf("validateCreds after New with full creds = %v, want nil", err)
	}
}

func TestValidateCreds(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.GitHubConfig
		wantErr bool
	}{
		{"both present valid", credsConfig(123, "/path/key.pem"), false},
		{"missing app id", credsConfig(0, "/path/key.pem"), true},
		{"missing key file", credsConfig(123, ""), true},
		{"both missing", credsConfig(0, ""), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.cfg)
			err := c.validateCreds()
			if tt.wantErr && err == nil {
				t.Error("validateCreds() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateCreds() = %v, want nil", err)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			"404 error response",
			&gogithub.ErrorResponse{Response: &http.Response{StatusCode: http.StatusNotFound}},
			true,
		},
		{
			"500 error response",
			&gogithub.ErrorResponse{Response: &http.Response{StatusCode: http.StatusInternalServerError}},
			false,
		},
		{
			"plain error",
			errors.New("boom"),
			false,
		},
		{
			"nil",
			nil,
			false,
		},
		{
			"wrapped 404",
			&wrappedErr{inner: &gogithub.ErrorResponse{Response: &http.Response{StatusCode: http.StatusNotFound}}},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotFound(tt.err); got != tt.want {
				t.Errorf("isNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// wrappedErr lets us exercise errors.As inside isNotFound.
type wrappedErr struct{ inner error }

func (w *wrappedErr) Error() string { return "wrapped" + w.inner.Error() }
func (w *wrappedErr) Unwrap() error { return w.inner }

func TestListReposNoCreds(t *testing.T) {
	c := New(config.GitHubConfig{})
	_, err := c.ListRepos(context.Background())
	assertCredsError(t, err)
}

func TestReadRepoFileNoCreds(t *testing.T) {
	c := New(config.GitHubConfig{})
	_, err := c.ReadRepoFile(context.Background(), "owner", "repo", "path")
	assertCredsError(t, err)
}

func TestGetReleaseByTagNoCreds(t *testing.T) {
	c := New(config.GitHubConfig{})
	_, err := c.GetReleaseByTag(context.Background(), "owner", "repo", "v1.0.0")
	assertCredsError(t, err)
}

func TestListReleasesNoCreds(t *testing.T) {
	c := New(config.GitHubConfig{})
	_, err := c.ListReleases(context.Background(), "owner", "repo")
	assertCredsError(t, err)
}

func TestEditReleaseBodyNoCreds(t *testing.T) {
	c := New(config.GitHubConfig{})
	err := c.EditReleaseBody(context.Background(), "owner", "repo", 1, "body")
	assertCredsError(t, err)
}

func TestCloneInfoNoCreds(t *testing.T) {
	c := New(config.GitHubConfig{})
	_, _, err := c.CloneInfo(context.Background(), "owner", "repo")
	assertCredsError(t, err)
}

func TestGitAuthHeader(t *testing.T) {
	// GitHub App git over HTTPS requires Basic auth with the reserved
	// x-access-token username and the installation token as the password;
	// GitHub's smart-HTTP endpoint rejects Bearer tokens. The header must
	// decode back to exactly that.
	got := gitAuthHeader("secrettoken")
	const wantPrefix = "Basic "
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("gitAuthHeader() = %q, want %q prefix", got, wantPrefix)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, wantPrefix))
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	if want := "x-access-token:secrettoken"; string(raw) != want {
		t.Errorf("header payload = %q, want %q", raw, want)
	}
}

func assertCredsError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected credentials error, got nil")
	}
	if !strings.Contains(err.Error(), "app credentials") {
		t.Errorf("error = %q, want a credentials-configuration error", err)
	}
}

// TestNoCredsGuardMapsToNotFound ensures that, even with a partial credential
// pair, the per-repo methods return the creds error and not ErrNotFound (they
// must fail before any network call).
func TestPartialCredsStillGuards(t *testing.T) {
	c := New(credsConfig(123, "")) // app id set, key missing
	_, _, err := c.CloneInfo(context.Background(), "owner", "repo")
	if errors.Is(err, pipeline.ErrNotFound) {
		t.Errorf("missing key should not surface ErrNotFound, got %v", err)
	}
	assertCredsError(t, err)
}
