# Annalist — example configuration pack

Copy-paste templates for adopting annalist in any repository: deploy the
service, configure it, set the release-notes prompt, and wire an automatic,
generate-first release on either Forgejo/Gitea or GitHub.

All examples are **language- and framework-neutral** — the workflows only need
`git`, `curl`, `jq`, and (on GitHub) `gh`, so they drop into a Go backend, a
Node/Deno/Bun web app, a Rust CLI, a static site, or anything else unchanged.

## Files

| File | What it is | Where it goes |
|---|---|---|
| `config.yaml` | Annotated annalist configuration (env var per key) | `config.yaml` next to the `annalist` binary (e.g. `/etc/annalist/config.yaml`), or as env vars |
| `docker-compose.yml` | Deploy annalist via Docker Compose (with healthcheck + data volume) | your infrastructure repo / `docker compose up -d` |
| `prompt.md` | Release-notes instructions consumed by annalist (the "system prompt") | `.forgejo/release-notes.md` (Forgejo) or `.github/release-notes-instructions.md` (GitHub) |
| `workflows/forgejo-release.yml` | Generate-first release workflow for Forgejo / Gitea | `.forgejo/workflows/release.yml` in the consuming repo |
| `workflows/github-release.yml` | Generate-first release workflow for GitHub | `.github/workflows/release.yml` in the consuming repo |

## Quick start

1. **Deploy annalist.** Copy `docker-compose.yml`, fill in `.env`
   (`ADMIN_TOKEN`, `LLM_BASE_URL`, `LLM_API_KEY`), and `docker compose up -d`.
   Terminate TLS in front of it (the binary does not do TLS).
2. **Set the prompt.** Drop `prompt.md` at the platform path for the consuming
   repo (see table). The in-repo file overrides dashboard/global instructions;
   edit the `[bracketed]` placeholders for your project's voice.
3. **Wire a release workflow.** Pick `workflows/forgejo-release.yml` or
   `workflows/github-release.yml`, copy it into the consuming repo, set the
   trigger branch, and set the `ANNALIST_URL` env at the top.
4. **Add the secret.** In the consuming repo's settings, add
   `ANNALIST_ADMIN_TOKEN` with the same value as the annalist `ADMIN_TOKEN`.
   (Forgejo also needs a `FORGEJO_TOKEN` with release write access; on GitHub
   `GH_TOKEN` is automatic.)

## How the pieces fit

Annalist can fill release bodies passively via release webhooks, or actively —
the pattern shown here is **generate-first**: the workflow asks annalist for
complete notes with `"publish": false` **before** the release exists, then
creates the release once with the full body. No empty release, no webhook
round-trip, no polling.

The workflows also implement automatic versioning: every push to the trigger
branch bumps the version from conventional commits (`BREAKING CHANGE` /
`feat!` → major, `feat` → minor, else patch), with `workflow_dispatch` inputs
(`version`, `bump`) for manual control. Re-runs are idempotent — a re-dispatch
after a partial failure reuses the existing tag and only completes the missing
release.

## Adapting

- **Trigger branch:** change `branches:` in the workflow to wherever your team
  merges (`main`, `develop`, …).
- **Container image:** the optional commented block in the workflows shows an
  idempotent `docker build`/`push`; add it if the repo ships an image. The
  release step itself does not build anything.
- **One-time or manual releases:** if you don't want every merge to release,
  remove the `push:` trigger and release via `workflow_dispatch` only — the
  versioning and notes generation still work.
