# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Developers and operators who publish software releases on GitHub or Forgejo/Gitea and want consistent, human-sounding release notes without hand-writing them. The owner intends annalist to grow as a general-purpose, self-hostable release-notes product rather than a personal tool. In practice the first users are self-hosters who deploy the daemon to their own infrastructure and log into the admin dashboard to manage repositories and tune the writing voice.

## Product Purpose

Generate release notes automatically: annalist listens for release webhooks, clones the repository, summarizes commit history through an OpenAI-compatible LLM, and writes the notes back into the release body. It exists so a release can ship with notes that read like a person wrote them, with no human transcription step. Success is a release event arriving and a vetted-sounding note appearing in the release body unattended.

## Positioning

A self-hosted, vendor-neutral release-notes engine: first-class support for both GitHub and Forgejo/Gitea under one pipeline, where the released writing voice is a configurable product feature rather than a fixed template. The combination a neighboring tool cannot truthfully copy is cross-platform coverage plus per-repo voice control plus zero-touch automation in one small self-hosted binary.

## Operating Context

Self-hosted service run as a container or Go binary. Configured through `config.yaml` or environment variables (defaults < config.yaml < env). SQLite database and git clones live in a data directory. Three surfaces:

- Webhook ingestion (GitHub App, Forgejo) that triggers generation on release events.
- CLI: `serve` (daemon), `generate` (on-demand, `--publish` writes to the release body), `version`.
- Admin dashboard served at `/`, protected by a single bearer `ADMIN_TOKEN`.

Operators typically run it on their own server next to the CI that ships their releases.

## Capabilities and Constraints

- Generation from release webhooks (GitHub App + Forgejo), deduplicated by event hash; regenerated notes carry a marker so repeated deliveries are idempotent.
- On-demand generation from the CLI on any platform, with optional publish-back.
- Voice control: a global or per-repo `tone`, three built-in presets (`chronicler`, `engineer`, `launch`), any arbitrary string treated as a custom system prompt, plus per-repo markdown instructions.
- Configurable LLM endpoint, model, temperature, token limit, and timeout against any OpenAI-compatible API (default model `qwen3.5-397b-a17b`).
- Single admin bearer token, single-operator model: one account protects all `/api/*` and dashboard actions.
- Data model: SQLite for settings/state, working clones for commit history.
- Backend: Go; dashboard: SvelteKit + Tailwind, embedded into the binary via `go:embed`. Docker image published to the GitHub Container Registry from CI on `main` and `v*` tags.
- An internal `platform` package abstracts GitHub vs Forgejo/Gitea differences so release handling stays shared — a load-bearing constraint, not an implementation detail.

## Brand Commitments

- Name: "Annalist" (displayed as "Annalist" in the dashboard nav).
- License: UNLICENSED. Distributed from `github.com/djdembeck/annalist`.
- Voice promise, stated in the README: "consistent, human-sounding release notes."
- No formal brand system, logo, or marketing commitments exist.

## Evidence on Hand

- README.md documenting install, usage, dashboard, and voice configuration.
- config.example.yaml documenting every configuration key and environment variable.
- Working Go backend (webhooks, pipeline, CLI) and a SvelteKit dashboard (setup, repos, settings pages).
- Docker image published via CI to the Forgejo Container Registry.
- No testimonials, customers, case studies, press, or marketing assets exist — future work must not fabricate them.

## Product Principles

1. **Zero-touch by default.** A release event should produce notes with no human in the loop; manual regeneration and publish-back are always available as an override, never the primary path.
2. **Vendor-neutral.** GitHub and Forgejo/Gitea are equals behind one shared pipeline; nothing should lock notes to either platform's format or workflow.
3. **The voice is the product.** Notes must read like a person wrote them; tone and per-repo instructions are first-class settings, not post-processing.
4. **Safe to operate unattended.** Idempotent regeneration and event dedup guarantee retries and re-delivered webhooks cannot corrupt or duplicate a release body.
5. **Boring to run.** A small self-hosted binary: Go, SQLite, an embedded static dashboard, config-file/env-driven, no external SaaS dependency except the user's own LLM endpoint.
