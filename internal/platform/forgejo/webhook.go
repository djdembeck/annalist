package forgejo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/djdembeck/annalist/internal/pipeline"
)

// signatureHeaders are the header names Forgejo/Gitea (and GitHub, across
// versions) use to convey the webhook HMAC. Any one matching is accepted.
var signatureHeaders = []string{
	"X-Gitea-Signature",
	"X-Gogs-Signature",
	"X-Forgejo-Signature",
	"X-Hub-Signature-256",
}

// eventHeaders are the header names that may carry the webhook event type.
var eventHeaders = []string{
	"X-Gitea-Event",
	"X-Forgejo-Event",
	"X-GitHub-Event",
}

// releaseWebhookPayload mirrors the Forgejo/Gitea release webhook payload.
type releaseWebhookPayload struct {
	Action  string `json:"action"`
	Release struct {
		ID      int64  `json:"id"`
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
	} `json:"release"`
	Repository struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
		Owner    struct {
			Login    string `json:"login"`
			Username string `json:"username"`
		} `json:"owner"`
	} `json:"repository"`
}

// WebhookHandler returns the HTTP handler that consumes Forgejo release
// webhooks, verifies the signature, and triggers note generation.
func (c *Client) WebhookHandler(p *pipeline.Pipeline) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.cfg.WebhookSecret == "" {
			http.Error(w, "forgejo webhook secret not configured", http.StatusServiceUnavailable)
			return
		}

		body, err := readBody(r)
		if err != nil {
			http.Error(w, "could not read body", http.StatusBadRequest)
			return
		}

		if !validSignature(c.cfg.WebhookSecret, body, r) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		event := webhookEvent(r)
		if event != "release" {
			w.WriteHeader(http.StatusOK)
			return
		}

		var payload releaseWebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}

		if payload.Action != "created" && payload.Action != "updated" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if payload.Release.Draft || payload.Release.TagName == "" {
			w.WriteHeader(http.StatusOK)
			return
		}

		owner := ownerFromRepository(&payload)
		repo := payload.Repository.Name
		if owner == "" || repo == "" || payload.Release.ID == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}

		enabled, _, err := p.Resolve(context.Background(), "forgejo", owner, repo)
		if err != nil {
			http.Error(w, fmt.Sprintf("resolve: %v", err), http.StatusInternalServerError)
			return
		}
		if !enabled {
			w.WriteHeader(http.StatusOK)
			return
		}

		spec := pipeline.Spec{
			Platform:  "forgejo",
			Owner:     owner,
			Repo:      repo,
			ToTag:     payload.Release.TagName,
			ReleaseID: "forgejo:" + fmt.Sprintf("%d", payload.Release.ID),
		}

		// Generate asynchronously. Forgejo's webhook client aborts delivery
		// after ~5 seconds ([webhook] DELIVERY_TIMEOUT), which cancels
		// r.Context() and kills long-running LLM generations (observed with
		// full-history releases). Acknowledge the webhook immediately and run
		// the pipeline on a detached context.
		w.WriteHeader(http.StatusOK)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if _, err := p.GenerateNotes(ctx, spec, pipeline.Options{Publish: true}); err != nil {
				log.Printf("annalist: generate %s/%s %s: %v", owner, repo, payload.Release.TagName, err)
			} else {
				log.Printf("annalist: published notes for %s/%s %s", owner, repo, payload.Release.TagName)
			}
		}()
	})
}

// readBody drains the full raw request body.
func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// validSignature returns true when the raw body's HMAC-SHA256 hex, keyed by the
// webhook secret, matches any of the known signature headers (constant-time).
func validSignature(secret string, body []byte, r *http.Request) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	for _, name := range signatureHeaders {
		got := r.Header.Get(name)
		got = strings.TrimPrefix(got, "sha256=")
		if got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1 {
			return true
		}
	}
	return false
}

// webhookEvent returns the release-relevant event type from the headers.
func webhookEvent(r *http.Request) string {
	for _, name := range eventHeaders {
		if v := r.Header.Get(name); v != "" {
			return v
		}
	}
	return ""
}

// ownerFromRepository extracts the owner from repository.full_name (owner/repo
// form), falling back to repository.owner fields when full_name is empty.
func ownerFromRepository(p *releaseWebhookPayload) string {
	if p.Repository.FullName != "" {
		if i := strings.IndexByte(p.Repository.FullName, '/'); i > 0 {
			return p.Repository.FullName[:i]
		}
	}
	if p.Repository.Owner.Login != "" {
		return p.Repository.Owner.Login
	}
	return p.Repository.Owner.Username
}
