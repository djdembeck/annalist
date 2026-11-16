// Command annalist is the self-hosted AI release-notes service. It listens
// for release webhooks (GitHub App + Forgejo), generates flowing prose release
// notes via an OpenAI-compatible LLM endpoint, and writes them into the release
// body. It also exposes a small admin CLI for manual regeneration.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/djdembeck/annalist/internal/config"
	"github.com/djdembeck/annalist/internal/db"
	"github.com/djdembeck/annalist/internal/engine"
	"github.com/djdembeck/annalist/internal/httpapi"
	"github.com/djdembeck/annalist/internal/llm"
	"github.com/djdembeck/annalist/internal/pipeline"
	"github.com/djdembeck/annalist/internal/platform/forgejo"
	"github.com/djdembeck/annalist/internal/platform/github"
	"github.com/djdembeck/annalist/internal/version"
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
		fmt.Println(version.Version)
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
	if err := store.ReconcileRuns(); err != nil {
		fmt.Fprintf(os.Stderr, "reconciling generation runs: %v\n", err)
		os.Exit(1)
	}

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
	rangePolicy := fs.String("range-policy", "", "auto-resolution policy when --from-tag is empty: previous_published (previous published non-draft ancestor release, full history when none); empty = legacy previous tag")
	publish := fs.Bool("publish", false, "publish the generated notes to the release body")
	mode := fs.String("mode", "", "generation mode: lite (default) or deep (commit log + diff)")
	profile := fs.String("profile", "", "named release-note profile from .annalist/release-notes.yaml (default: legacy provider-specific instructions)")
	displayVersion := fs.String("display-version", "", "presentation version shown in the notes (default: the source tag)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: annalist generate --platform <github|forgejo> --owner <owner> --repo <repo> --to-tag <tag> [--from-tag <tag> | --range-policy previous_published] [--mode <lite|deep>] [--profile <name>] [--display-version <text>] [--publish]")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *platform == "" || *owner == "" || *repo == "" || *toTag == "" {
		fs.Usage()
		os.Exit(1)
	}
	if *mode != "" && *mode != engine.ModeLite && *mode != engine.ModeDeep {
		fmt.Fprintf(os.Stderr, "unknown mode %q (want lite|deep)\n", *mode)
		os.Exit(1)
	}
	if *rangePolicy != "" && *rangePolicy != pipeline.RangePreviousPublished {
		fmt.Fprintf(os.Stderr, "unknown range policy %q (want previous_published)\n", *rangePolicy)
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
	result, err := pip.GenerateNotes(context.Background(), pipeline.Spec{
		Platform:       *platform,
		Owner:          *owner,
		Repo:           *repo,
		ToTag:          *toTag,
		FromTag:        *fromTag,
		RangePolicy:    *rangePolicy,
		ReleaseID:      releaseID,
		Profile:        *profile,
		DisplayVersion: *displayVersion,
	}, pipeline.Options{Force: true, Publish: *publish, Mode: *mode})
	if code := printGenerationResult(os.Stdout, os.Stderr, result, err); code != 0 {
		os.Exit(code)
	}
}

// printGenerationResult keeps review-needed output visible but unsuccessful,
// so shell chains cannot treat it as approved for unattended publication.
func printGenerationResult(stdout, stderr io.Writer, result pipeline.Result, err error) int {
	if err != nil {
		var qualityErr *pipeline.QualityError
		if errors.As(err, &qualityErr) {
			fmt.Fprintln(stderr, "Publication blocked: notes need quality review. Nothing was published.")
			printQualityIssues(stderr, qualityErr.Issues)
		} else {
			fmt.Fprintf(stderr, "generating notes: %v\n", err)
		}
		return 1
	}

	fmt.Fprintln(stdout, result.Notes)
	if result.WarningCode == "quality_review_required" || len(result.QualityIssues) > 0 {
		fmt.Fprintln(stderr, "Warning: notes need quality review. Nothing was published. Do not publish this preview automatically.")
		printQualityIssues(stderr, result.QualityIssues)
		return 1
	}
	if result.WarningCode == "cache_persistence" {
		fmt.Fprintln(stderr, "Warning: notes were published, but the local note cache could not be saved. Check server storage before generating again.")
	} else if result.WarningCode != "" {
		fmt.Fprintln(stderr, "Warning: generation completed with a warning. Check server configuration before generating again.")
	}
	return 0
}

func printQualityIssues(w io.Writer, issues []string) {
	for _, issue := range issues {
		switch issue {
		case "strict_prose_semicolon":
			fmt.Fprintln(w, "- Strict engineer prose contains semicolons. Review the wording without changing technical text.")
		case "source_context_incomplete":
			fmt.Fprintln(w, "- Some diff hunks did not fit the source context. Review the full release range for missing actions and conditions.")
		case "source_attribution_invalid":
			fmt.Fprintln(w, "- Source attribution is invalid, so the preview was withheld. Check for missing, duplicate, or unknown source IDs and unverified links. Follow the citation contract before generating again.")
		case "quality_check_required":
			fmt.Fprintln(w, "- The notes do not have a current quality check. Generate a new preview before publishing.")
		default:
			fmt.Fprintln(w, "- A required quality check did not pass. Review the notes and source before publishing.")
		}
	}
}
