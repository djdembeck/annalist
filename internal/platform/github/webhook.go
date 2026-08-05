package github

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

// webhookPayload is the subset of the GitHub release webhook payload this app
// consumes.
type webhookPayload struct {
	Action  string `json:"action"`
	Release struct {
		ID      int64  `json:"id"`
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
	} `json:"release"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

// WebhookHandler returns an http.Handler that verifies the GitHub webhook
// signature and dispatches release events into the pipeline. It reads the full
// raw body before verifying, so the HMAC is computed over the exact bytes
// GitHub signed.
func (c *Client) WebhookHandler(p *pipeline.Pipeline) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.cfg.WebhookSecret == "" {
			http.Error(w, "github webhook secret not configured", http.StatusServiceUnavailable)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "github: read body", http.StatusBadRequest)
			return
		}

		// Verify the HMAC-SHA256 hex signature over the raw body.
		sig := strings.TrimPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
		mac := hmac.New(sha256.New, []byte(c.cfg.WebhookSecret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
			http.Error(w, "github: invalid signature", http.StatusUnauthorized)
			return
		}

		// Ignore non-release events (this is a ping/probe or an unrelated event).
		if r.Header.Get("X-GitHub-Event") != "release" {
			w.WriteHeader(http.StatusOK)
			return
		}

		var pl webhookPayload
		if err := json.Unmarshal(body, &pl); err != nil {
			http.Error(w, "github: bad payload", http.StatusBadRequest)
			return
		}
		if pl.Action != "published" && pl.Action != "created" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if pl.Release.Draft {
			w.WriteHeader(http.StatusOK)
			return
		}

		spec := pipeline.Spec{
			Platform:  "github",
			Owner:     pl.Repository.Owner.Login,
			Repo:      pl.Repository.Name,
			ToTag:     pl.Release.TagName,
			ReleaseID: "github:" + fmt.Sprintf("%d", pl.Release.ID),
		}

		enabled, _, err := p.Resolve(context.Background(), spec.Platform, spec.Owner, spec.Repo)
		if err != nil {
			http.Error(w, "github: resolve settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !enabled {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Generate asynchronously. GitHub's webhook delivery aborts the
		// request quickly, which cancels r.Context() and kills long-running
		// LLM generations. Acknowledge the webhook immediately and run the
		// pipeline on a detached context (mirrors the Forgejo handler).
		w.WriteHeader(http.StatusOK)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if _, err := p.GenerateNotes(ctx, spec, pipeline.Options{Publish: true}); err != nil {
				log.Printf("annalist: generate %s/%s %s: %v", spec.Owner, spec.Repo, spec.ToTag, err)
			} else {
				log.Printf("annalist: published notes for %s/%s %s", spec.Owner, spec.Repo, spec.ToTag)
			}
		}()
	})
}
