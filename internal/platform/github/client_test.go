package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	_, err := c.ReadRepoFile(context.Background(), "owner", "repo", "path", "")
	assertCredsError(t, err)
}

func TestGetReleaseByTagNoCreds(t *testing.T) {
	c := New(config.GitHubConfig{})
	_, err := c.GetReleaseByTag(context.Background(), "owner", "repo", "v1.0.0")
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

// testAppKeyPEM generates a throwaway RSA private key in the "PRIVATE KEY"
// (PKCS#1) form GitHub App credentials use, written to a temp file.
func testAppKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeGitHubRT is a full HTTP.RoundTripper that serves canned GitHub API
// responses and records the RequestURI of every request it handles. The
// ghinstallation transport routes every request (installation lookup, JWT
// access-token exchange, and the proxied contents call) through the base
// transport, so injecting this as http.DefaultTransport captures the exact
// wire URL the go-github client builds.
type fakeGitHubRT struct {
	mu   sync.Mutex
	recs []string
}

func (f *fakeGitHubRT) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.recs = append(f.recs, req.URL.RequestURI())
	f.mu.Unlock()

	var body string
	status := http.StatusOK
	switch req.URL.Path {
	case "/repos/o/r/installation":
		body = `{"id": 99, "app_id": 1}`
	case "/app/installations/99/access_tokens":
		body = `{"token": "inst-token", "expires_at": "2030-01-01T00:00:00Z"}`
	case "/repos/o/r/contents/f.md":
		body = `{"type":"file","encoding":"base64","content":"aGVsbG8="}`
	default:
		body = `{"message":"Not Found"}`
		status = http.StatusNotFound
	}
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Proto: "HTTP/1.1", Request: req,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (f *fakeGitHubRT) lastContents() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.recs) - 1; i >= 0; i-- {
		if strings.Contains(f.recs[i], "contents/f.md") {
			return f.recs[i]
		}
	}
	return ""
}

// TestReadRepoFilePinsRef proves the contents request carries the exact
// requested ref in the ?ref= query and omits it for an empty ref (default
// branch).
func TestReadRepoFilePinsRef(t *testing.T) {
	fake := &fakeGitHubRT{}
	orig := http.DefaultTransport
	http.DefaultTransport = fake
	t.Cleanup(func() { http.DefaultTransport = orig })

	c := New(config.GitHubConfig{AppID: 1, AppPrivateKeyFile: testAppKeyPEM(t)})

	t.Run("non-empty ref is sent", func(t *testing.T) {
		if _, err := c.ReadRepoFile(context.Background(), "o", "r", "f.md", "v1.2.3"); err != nil {
			t.Fatalf("ReadRepoFile: %v", err)
		}
		if got := fake.lastContents(); !strings.HasSuffix(got, "/repos/o/r/contents/f.md?ref=v1.2.3") {
			t.Errorf("wire URI = %q, want .../contents/f.md?ref=v1.2.3", got)
		}
	})

	t.Run("empty ref omits the query", func(t *testing.T) {
		fake.mu.Lock()
		fake.recs = nil
		fake.mu.Unlock()
		if _, err := c.ReadRepoFile(context.Background(), "o", "r", "f.md", ""); err != nil {
			t.Fatalf("ReadRepoFile: %v", err)
		}
		if got := fake.lastContents(); strings.Contains(got, "?") {
			t.Errorf("wire URI = %q, want no query for empty ref", got)
		}
	})
}
