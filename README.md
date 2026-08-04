# Annalist

AI-generated release notes for GitHub and Forgejo. Annalist listens for release webhooks, clones the repository, summarizes commit history through an OpenAI-compatible LLM, and writes the notes back into the release body. It ships with a small admin dashboard for manual generation and per-repo settings.

Annalist is aimed at DevOps and SRE teams who want consistent, human-sounding release notes without hand-writing each one. It supports GitHub Apps, Forgejo webhooks, and on-demand CLI generation, with idempotent regeneration so the same webhook can be retried safely.

> **New here?** The [`examples/`](examples/README.md) directory holds copy-paste
> templates: a deployable `docker-compose.yml`, an annotated `config.yaml`, a
> release-notes prompt file, and generate-first release workflows for both
> Forgejo and GitHub Actions — language-neutral, ready to adopt in any repo.

## Install

The fastest path is to pull the pre-built image from the GitHub Container Registry:

```bash
docker pull ghcr.io/djdembeck/annalist:latest
docker run -d \
  -p 8080:8080 \
  -e ADMIN_TOKEN="$(openssl rand -hex 32)" \
  -e LLM_BASE_URL="https://api.example.com/v1" \
  -e LLM_API_KEY="your-api-key" \
  -v "$PWD/data:/app/data" \
  --name annalist \
  ghcr.io/djdembeck/annalist:latest
```

Images are published by the CI workflow on pushes to `main` and tags (`v*`); tags follow semver. See `.github/workflows/` for details.

### From source

```bash
git clone https://github.com/djdembeck/annalist.git
cd annalist
go build -o annalist ./cmd/annalist
```

Prerequisites: Go 1.26+, Bun 1.x (only if modifying the web dashboard). The build embeds the dashboard frontend automatically.

## Security

Run the service behind a TLS-terminating reverse proxy; the binary itself does not terminate TLS. Keep `ADMIN_TOKEN` and `LLM_API_KEY` in environment variables (or a `config.yaml` readable only by the service user) — never commit them anywhere. The data directory holds the SQLite database and git clones; restrict its access to the service user.

## Platform setup

Connect annalist to GitHub or Forgejo. Order matters: **configure and start the
container first, then register the platform-side webhooks against the now-live
URL.** The tokens and secrets are container configuration (they must exist
before `serve` starts); webhook registration points a running instance's URL
(`/webhooks/github`, `/webhooks/forgejo`) and therefore happens after startup.

Copy-paste templates for deployment, config, and CI workflows live in
[`examples/`](examples/README.md).

### Prerequisites (container config — before startup)

- `ADMIN_TOKEN` — protects the dashboard and every `/api/*` route. Required.
- `LLM_BASE_URL` (+ optionally `LLM_API_KEY`) — any OpenAI-compatible endpoint.
  Required to start when a platform is enabled.
- One credential pair per platform (below). A platform is "enabled" once its
  cloning credential is present; a webhook secret alone enables the listener
  but not cloning/publishing.

### Ordering

1. Set the environment variables from the table below and start the container
   (`docker run` / `docker compose up -d` — see the examples pack).
2. Confirm the container is reachable at its public URL:
   `GET https://<annalist-host>/api/health` → `{"status":"ok",...}`.
3. Register the platform webhooks (GitHub App webhook / Forgejo repo or org
   webhook) against `https://<annalist-host>/webhooks/github` and
   `https://<annalist-host>/webhooks/forgejo`.
4. Open the dashboard, enter `ADMIN_TOKEN`, and check **Repos** — the repos
   you granted access appear there. No per-repo registration step: repos are
   enabled by default.

### GitHub (account / organization)

Annalist talks to GitHub as a **GitHub App**; the App is the account-level
setup, installed onto the account/org to grant repo access.

1. Create the App: GitHub → Settings → Developer settings → GitHub Apps →
   New GitHub App. Set **Webhook URL** to `https://<annalist-host>/webhooks/github`
   and **Webhook secret** to the value of `GITHUB_WEBHOOK_SECRET`. Subscribe to
   the **Release** event. (Leave it enabled even if you only use the generate-first
   workflow — it is the passive-fill path.)
2. Grant permissions (Repository permissions): **Contents** → Read-only
   (clone + read the in-repo instructions file), **Releases** → Read and write
   (read + edit the release body), **Metadata** → Read-only (default, required
   to list repos/installations). Under **Where can this App be installed**,
   choose the account/org you manage.
3. Generate a **private key** on the App's General tab and download the `.pem`;
   mount it read-only into the container (in Docker Compose:
   `./github-app.pem:/etc/annalist/github-app.pem:ro`) and set
   `GITHUB_APP_PRIVATE_KEY_FILE` to the **container** path
   (`/etc/annalist/github-app.pem`).
4. Install the App on your account/org (GitHub → Settings → Applications →
   GitHub Apps), granting it **All repositories** or the selected repos to manage.
5. Set the container env: `GITHUB_APP_ID`, `GITHUB_APP_PRIVATE_KEY_FILE`,
   `GITHUB_WEBHOOK_SECRET`.

### Forgejo (instance / account / repo)

Annalist talks to Forgejo with a **personal/org API token** plus a registered
**repository or organization webhook**.

1. Create a token: Forgejo → Settings → Applications → Generate New Token.
   Scope it for the operations annalist performs: repository read (clone and
   read files) and release read/write (read + edit release bodies).
2. Register the webhook: repo (or org) → Settings → Webhooks → Add Webhook →
   Forgejo. **Target URL** `https://<annalist-host>/webhooks/forgejo`,
   **HTTP method** POST, **Content type** application/json, **Secret** = the
   value of `FORGEJO_WEBHOOK_SECRET`, and the **Release** event. An org-level
   webhook covers every repo in the org.
3. Set the container env: `FORGEJO_URL` (instance base, default
   `https://git.theiahd.nl`), `FORGEJO_TOKEN`, `FORGEJO_WEBHOOK_SECRET`.

### Two ways annalist fills release notes

- **Passive (webhook fill).** Create an empty release in the platform UI (or
  via your own tooling); the Release webhook fires and annalist fills the body.
  Requires the webhook registration above.
- **Generate-first (recommended).** A CI workflow asks annalist for complete
  notes with `publish:false` **before** the release exists, then creates the
  release once with the full body. Needs **no webhook** — only
  `ANNALIST_ADMIN_TOKEN` stored as a CI secret (same value as the container's
  `ADMIN_TOKEN`) plus the platform credential for cloning. Ready-to-copy
  workflows: `examples/workflows/forgejo-release.yml` and
  `examples/workflows/github-release.yml`.

## Usage

Configure the server by copying `config.example.yaml` to `config.yaml` or by setting environment variables. The `serve` command requires `ADMIN_TOKEN`; when GitHub or Forgejo is enabled, `LLM_BASE_URL` is also required.

```bash
# Start the daemon (webhooks + dashboard + API)
./annalist serve

# With a config file in another location (the binary always looks for config.yaml in the working directory)
cd /etc/annalist && /usr/local/bin/annalist serve
```

Generate notes for a single release from the command line. `--publish` writes them to the release body; without it, notes are printed to stdout.

```bash
./annalist generate \
  --platform github \
  --owner djdembeck \
  --repo my-project \
  --to-tag v1.2.0 \
  --from-tag v1.1.0 \
  --publish
```

Show the build version:

```bash
./annalist version
```

Run `annalist generate --help` (or `annalist serve` with no config) for a brief usage summary. Full configuration keys are documented in `config.example.yaml`.

### Dashboard

When running, the web dashboard is available at `http://<SERVER_HOST>:<SERVER_PORT>/`. Log in with the `ADMIN_TOKEN` as a bearer token. From the dashboard you can list repositories, queue or manually refresh release notes, override per-repo tone/instructions, and review global settings.

### Configurable release-note voice

Annalist lets you control the generated voice through a per-repo or global `tone`. Three built-in presets (`chronicler`, `engineer`, `launch`) are selectable in the dashboard; any other `tone` value is treated as a custom freeform system prompt. Additional per-repo markdown instructions can be injected to bind style, length, or terminology.

## Background

Annalist abstracts the differences between GitHub and Forgejo/Gitea APIs into a shared pipeline: receive release event, resolve repo settings, clone and read commit logs, call the LLM, and publish the result. Webhook payloads are deduplicated by event hash, and regenerated notes carry a marker so repeated deliveries are idempotent.

## Contributing

Contributions are welcome. Before opening a pull request, run the same checks the CI runs:

```bash
gofmt -l .
go vet ./...
cd web && bun run check && cd ..
go test ./...
go build ./...
```

No `CONTRIBUTING.md`, `SECURITY.md`, or `CODE_OF_CONDUCT.md` files exist yet; guidance can be added once they are written.

## License

UNLICENSED
