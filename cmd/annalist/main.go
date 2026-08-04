// Command annalist is the self-hosted AI release-notes service. It listens
// for release webhooks (GitHub App + Forgejo), generates flowing prose release
// notes via an OpenAI-compatible LLM endpoint, and writes them into the release
// body. It also exposes a small admin CLI for manual regeneration.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/httpapi"
	"github.com/djdembeck/annalist/internal/llm"
	"github.com/djdembeck/annalist/internal/pipeline"
	"github.com/djdembeck/annalist/internal/platform/forgejo"
	"github.com/djdembeck/annalist/internal/platform/github"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading config: %v\n", err)
		os.Exit(1)
	}

	cmd := "serve"
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve", "generate", "version":
			cmd = os.Args[1]
		}
	}

	switch cmd {
	case "version":
		fmt.Println("0.1.0")
	case "generate":
		cmdGenerate(cfg, os.Args[2:])
	default:
		cmdServe(cfg)
	}
}

// cmdServe runs the web server: webhooks + dashboard + admin API.
func cmdServe(cfg *config.Config) {
	if cfg.Admin.Token == "" {
		fmt.Fprintf(os.Stderr, "ADMIN_TOKEN is required; set it before starting\n")
		os.Exit(1)
	}
	if (cfg.GitHubEnabled() || cfg.ForgejoEnabled()) && cfg.LLM.BaseURL == "" {
		fmt.Fprintln(os.Stderr, "LLM_BASE_URL is required when a platform is configured")
		os.Exit(1)
	}

	store, err := db.New(cfg.Data.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	llmClient := llm.New(cfg.LLM)
	gh := github.New(cfg.GitHub)
	fj := forgejo.New(cfg.Forgejo)
	pip := pipeline.New(cfg, store, llmClient, gh, fj)
	handler := httpapi.New(cfg, store, llmClient, pip, gh, fj)

	addr := net.JoinHostPort(cfg.Server.Host, fmt.Sprintf("%d", cfg.Server.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// ReadTimeout caps slow-body clients (slowloris); WriteTimeout is
		// generous because /api generate runs clone + LLM inline (LLM default
		// timeout is 120s). IdleTimeout reaps keep-alive connections.
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "listening on http://%s\n", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "shutdown error: %v\n", err)
			os.Exit(1)
		}
	}
}

// cmdGenerate generates release notes for a single release on the CLI. It
// always forces regeneration (bypasses idempotency); --publish additionally
// writes the notes into the release body.
func cmdGenerate(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	platform := fs.String("platform", "", "platform to use (github|forgejo)")
	owner := fs.String("owner", "", "repository owner")
	repo := fs.String("repo", "", "repository name")
	toTag := fs.String("to-tag", "", "tag to generate notes for")
	fromTag := fs.String("from-tag", "", "previous tag (optional; auto-resolved if empty)")
	publish := fs.Bool("publish", false, "publish the generated notes to the release body")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: annalist generate --platform <github|forgejo> --owner <owner> --repo <repo> --to-tag <tag> [--from-tag <tag>] [--publish]")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *platform == "" || *owner == "" || *repo == "" || *toTag == "" {
		fs.Usage()
		os.Exit(1)
	}
	if cfg.LLM.BaseURL == "" {
		fmt.Fprintln(os.Stderr, "LLM_BASE_URL is required to generate notes")
		os.Exit(1)
	}

	store, err := db.New(cfg.Data.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	llmClient := llm.New(cfg.LLM)

	var pip *pipeline.Pipeline
	switch *platform {
	case "github":
		if !cfg.GitHubEnabled() {
			fmt.Fprintf(os.Stderr, "platform %s is not configured\n", *platform)
			os.Exit(1)
		}
		pip = pipeline.New(cfg, store, llmClient, github.New(cfg.GitHub), nil)
	case "forgejo":
		if !cfg.ForgejoEnabled() {
			fmt.Fprintf(os.Stderr, "platform %s is not configured\n", *platform)
			os.Exit(1)
		}
		pip = pipeline.New(cfg, store, llmClient, nil, forgejo.New(cfg.Forgejo))
	default:
		fmt.Fprintf(os.Stderr, "unknown platform %q (want github|forgejo)\n", *platform)
		os.Exit(1)
	}

	releaseID := "cli:" + *platform + "/" + *owner + "/" + *repo + "@" + *toTag
	notes, err := pip.GenerateNotes(context.Background(), pipeline.Spec{
		Platform:  *platform,
		Owner:     *owner,
		Repo:      *repo,
		ToTag:     *toTag,
		FromTag:   *fromTag,
		ReleaseID: releaseID,
	}, pipeline.Options{Force: true, Publish: *publish})
	if err != nil {
		fmt.Fprintf(os.Stderr, "generating notes: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(notes)
}
