package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/engine"
	"github.com/djdembeck/annalist/internal/llm"
	"github.com/djdembeck/annalist/internal/pipeline"
	"github.com/djdembeck/annalist/internal/platform/forgejo"
	"github.com/djdembeck/annalist/internal/platform/github"
	"github.com/djdembeck/annalist/web"
)

// ghClient is the subset of the GitHub platform client the HTTP API needs.
type ghClient interface {
	WebhookHandler(p *pipeline.Pipeline) http.Handler
	ListRepos(ctx context.Context) ([]pipeline.OwnerRepo, error)
	ReadRepoFile(ctx context.Context, owner, repo, path, ref string) (string, error)
}

// fjClient is the subset of the Forgejo platform client the HTTP API needs.
type fjClient interface {
	WebhookHandler(p *pipeline.Pipeline) http.Handler
	ListRepos(ctx context.Context) ([]pipeline.OwnerRepo, error)
	ReadRepoFile(ctx context.Context, owner, repo, path, ref string) (string, error)
}

// pipService is the subset of the pipeline the HTTP API needs, so handlers can
// be unit-tested with a fake. *pipeline.Pipeline satisfies it.
type pipService interface {
	Resolve(ctx context.Context, platform, owner, repo string) (bool, pipeline.Effective, engine.Resolved, error)
	GenerateNotes(ctx context.Context, spec pipeline.Spec, opts pipeline.Options) (pipeline.Result, error)
}

// api carries the dependencies shared by every HTTP handler.
type api struct {
	cfg   *config.Config
	store *db.Store
	llm   *llm.Client
	pip   pipService
	gh    ghClient
	fj    fjClient
}

// New builds the full HTTP handler: the JSON API, the platform webhooks
// (mounted only when their platform is enabled), and the embedded SPA.
// It performs no network calls at construction time.
func New(cfg *config.Config, store *db.Store, llmClient *llm.Client, pip *pipeline.Pipeline, gh *github.Client, fj *forgejo.Client) http.Handler {
	a := &api{cfg: cfg, store: store, llm: llmClient, pip: pip, gh: gh, fj: fj}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	r.Use(limitBody)

	// GET /api/health is open; everything else under /api/* requires admin auth.
	r.Get("/api/health", a.handleHealth)
	r.Route("/api", func(r chi.Router) {
		r.Use(a.adminAuth)
		r.Get("/status", a.handleStatus)
		r.Get("/repos", a.handleListRepos)
		r.Get("/repos/available", a.handleListAvailableRepos)
		r.Post("/repos", a.handleAddRepo)
		r.Put("/repos/{platform}/{owner}/{repo}/settings", a.handlePutRepoSettings)
		r.Post("/repos/{platform}/{owner}/{repo}/generate", a.handleGenerate)
		r.Get("/repos/{platform}/{owner}/{repo}/in-repo-instructions", a.handleInRepoInstructions)
		r.Get("/settings", a.handleGetSettings)
		r.Put("/settings", a.handlePutSettings)
		r.Get("/models", a.handleGetModels)
	})

	// Webhooks bypass admin auth; each platform handler verifies its own
	// signature. Mounted only when the platform is configured.
	if cfg.GitHubEnabled() {
		r.Mount("/webhooks/github", a.gh.WebhookHandler(pip))
	}
	if cfg.ForgejoEnabled() {
		r.Mount("/webhooks/forgejo", a.fj.WebhookHandler(pip))
	}

	// SPA fallback for everything that isn't a route or webhook. web.Handler
	// serves the embedded build when compiled with `-tags webui`, else a 404
	// stub (embed_stub.go) that lets non-UI builds/tests compile without the
	// frontend build present.
	r.NotFound(web.Handler().ServeHTTP)
	return r
}

// limitBody caps every request body at 1 MiB. Webhook and settings payloads
// are small; LLM responses are read separately in llm/client.go with their own
// 4096-byte limit.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
}

// adminAuth rejects requests without a constant-time-matching
// `Authorization: Bearer <ADMIN_TOKEN>`.
func (a *api) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := a.cfg.Admin.Token
		if token == "" {
			writeErr(w, http.StatusUnauthorized, "admin token not configured")
			return
		}
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, prefix) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		provided := strings.TrimSpace(auth[len(prefix):])
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
