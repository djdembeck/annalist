# Annalist

<p align="center">
  <img src="web/static/annalist-mark.svg" alt="Annalist" width="240">
</p>

AI-generated release notes for GitHub and Forgejo. Annalist listens for release webhooks, clones the repository, summarizes commit history through an OpenAI-compatible LLM, and writes the notes back into the release body. It also ships an embedded admin dashboard for manual generation and per-repository settings.

Annalist is built for self-hosting developers and operators, especially DevOps and SRE teams that want consistent, **human-sounding release notes** without writing each one by hand. GitHub Apps, Forgejo webhooks, on-demand CLI generation, and idempotent regeneration are supported in one service.

> **New here?** The [`examples/`](examples/README.md) directory contains copy-paste templates: a deployable `docker-compose.yml`, an annotated `config.yaml`, a release-notes prompt file, and generate-first release workflows for Forgejo and GitHub Actions. The examples are language- and framework-neutral.

## Table of contents

- [Security](#security)
- [Install](#install)
- [Background](#background)
- [Dashboard](#dashboard)
- [Platform setup](#platform-setup)
- [Release workflows](#release-workflows)
- [Usage](#usage)
- [Configuration](#configuration)
- [HTTP API and webhook routes](#http-api-and-webhook-routes)
- [Building](#building)
- [Contributing](#contributing)
- [License](#license)

## Security

Run Annalist behind a TLS-terminating reverse proxy; the binary does not terminate TLS. Keep `ADMIN_TOKEN` and `LLM_API_KEY` in environment variables, or in a `config.yaml` readable only by the service user. Never commit either secret. The data directory contains the SQLite database and git clones, so restrict it to the service user. `ADMIN_TOKEN` protects the dashboard and admin API routes; `GET /api/health` is intentionally unauthenticated.

The GitHub App private key must also be mounted read-only. The service creates the data directory with mode `0700`; preserve that restriction when mounting or copying data. Requests are limited to 1 MiB and LLM responses to 4096 bytes.

## Install

The fastest path is the pre-built image from GitHub Container Registry:

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

The container listens on port `8080`. Persist `/app/data`; it holds the SQLite database and git clones. The runtime runs as the non-root `annalist` user. The image includes the embedded dashboard and uses `serve` by default.

Images are published by the CI workflow on pushes to `main` and tags matching `v*`; tags follow semver. See [`.github/workflows/`](.github/workflows/) for the publishing workflows.

For a Compose deployment, copy [`examples/docker-compose.yml`](examples/docker-compose.yml), provide `ADMIN_TOKEN`, `LLM_BASE_URL`, and `LLM_API_KEY` in `.env`, and run:

```bash
docker compose up -d
```

Terminate TLS in front of the container. After it starts, confirm the health endpoint responds:

```text
GET https://<annalist-host>/api/health
→ {"status":"ok",...}
```

## Background

Release notes are tedious to write by hand and often inconsistent between releases and repositories. Annalist provides one self-hosted, vendor-neutral pipeline for GitHub and Forgejo/Gitea: receive a release event or on-demand request, resolve repository settings, clone the repository, read the commit history and in-repository instructions, call an OpenAI-compatible LLM, and publish the result.

Webhook payloads are deduplicated by event hash, and generated notes carry an Annalist marker. Retried deliveries and regeneration therefore remain idempotent. The `generate` CLI intentionally always forces regeneration, bypassing that webhook idempotency behavior for explicit operator requests.

## Dashboard

The dashboard is an embedded SvelteKit 5 application served by the Annalist binary. Open it at `http://<SERVER_HOST>:<SERVER_PORT>/` and authenticate with `ADMIN_TOKEN` as a bearer token.

The installed dashboard now treats `/` as a **setup-first operating home**, not as the post-setup repository list:

1. Open `/` to see the activation status and the release workflow path.
2. Use `/setup` for first-run activation and to establish the local onboarding state.
3. After activation, use `/repos` (and `/repos/add`) for repository access and per-repository settings, and `/settings` for global settings.

The landing page and setup page are public routes. The management routes and admin `/api/*` routes require the bearer token matching `ADMIN_TOKEN`; `GET /api/health` is the unauthenticated health check. Repositories granted to the configured GitHub App or Forgejo credentials appear in **Repos** and are enabled by default; there is no separate per-repository registration step.

The dashboard uses the **Release Trace Wall** visual system: dark operator panels, thermal-paper proof surfaces, copper action rails, and cyan trace signals. This describes the interface style, not a separate asset or deployment requirement.

## Platform setup

Connect Annalist to GitHub or Forgejo. The order matters: **configure and start the container first, then register platform-side webhooks against the live URL.** Tokens and secrets are container configuration and must exist before `serve` starts. Webhook registration targets a running instance at `/webhooks/github` or `/webhooks/forgejo`, so it happens after startup.

Copy-paste deployment, config, prompt, and workflow templates are in [`examples/`](examples/README.md).

### Prerequisites before startup

- `ADMIN_TOKEN` protects the dashboard and admin `/api/*` routes. `GET /api/health` is unauthenticated. It is required to start `serve`.
- `LLM_BASE_URL` identifies an OpenAI-compatible endpoint. It is required to start when a platform is enabled; `LLM_API_KEY` is optional when the endpoint does not require one.
- Add one credential set per platform you enable. A platform is enabled once its cloning credential is present; a webhook secret alone enables the listener but not cloning or publishing.

### Startup and registration order

1. Set the environment variables below and start the container with `docker run` or `docker compose up -d` (see the examples pack).
2. Confirm the public health check: `GET https://<annalist-host>/api/health` returns `{"status":"ok",...}`.
3. Register the platform webhook against `https://<annalist-host>/webhooks/github` or `https://<annalist-host>/webhooks/forgejo`.
4. Open the dashboard, enter `ADMIN_TOKEN`, and check **Repos**. Repositories granted to the integration appear there and are enabled by default.

### GitHub (account or organization)

Annalist connects to GitHub as a **GitHub App**. The App is configured at the account level and installed onto the account or organization to grant repository access.

1. Create the App at GitHub → Settings → Developer settings → GitHub Apps → New GitHub App. Set the **Webhook URL** to `https://<annalist-host>/webhooks/github`, set the **Webhook secret** to the value of `GITHUB_WEBHOOK_SECRET`, and subscribe to the **Release** event. Leave the event enabled even when using only generate-first releases; it is the passive-fill path.
2. Grant these repository permissions: **Contents** → Read-only (clone and read the in-repository instructions file), **Releases** → Read and write (read and edit the release body), and **Metadata** → Read-only (required to list repositories and installations). Under **Where can this App be installed**, choose the account or organization you manage.
3. Generate a **private key** on the App's General tab and download the `.pem`. Mount it read-only into the container; in Docker Compose, for example: `./github-app.pem:/etc/annalist/github-app.pem:ro`. Set `GITHUB_APP_PRIVATE_KEY_FILE` to the path inside the container: `/etc/annalist/github-app.pem`.
4. Install the App on the account or organization at GitHub → Settings → Applications → GitHub Apps. Grant it **All repositories** or only the selected repositories it should manage.
5. Set `GITHUB_APP_ID`, `GITHUB_APP_PRIVATE_KEY_FILE`, and `GITHUB_WEBHOOK_SECRET` in the container environment.

### Forgejo (instance, account, or repository)

Annalist connects to Forgejo/Gitea with a personal or organization API token and a registered repository or organization webhook.

1. Create a token at Forgejo → Settings → Applications → Generate New Token. Give it the repository read access needed to clone and read files, and release read/write access needed to read and edit release bodies.
2. Register the webhook at repository (or organization) → Settings → Webhooks → Add Webhook → Forgejo. Use target URL `https://<annalist-host>/webhooks/forgejo`, HTTP method `POST`, content type `application/json`, the value of `FORGEJO_WEBHOOK_SECRET` as **Secret**, and the **Release** event. An organization-level webhook covers every repository in that organization.
3. Set `FORGEJO_URL` to the Forgejo instance base URL with no trailing path or default (for example, `https://git.example.com`), along with `FORGEJO_TOKEN` and `FORGEJO_WEBHOOK_SECRET`.

## Release workflows

Annalist supports two ways to fill a release body:

- **Passive (webhook fill):** Create an empty release in the platform UI or through your own tooling. The Release webhook fires, Annalist clones the repository, generates the notes, and fills the body. This requires the platform webhook registration above.
- **Generate-first (recommended):** A CI workflow asks Annalist for complete notes with `publish:false` **before** the release exists, then creates the release once with the full body. This needs **no webhook**: store `ANNALIST_ADMIN_TOKEN` as a CI secret using the same value as the container's `ADMIN_TOKEN`, and provide the platform credential used for cloning.

Ready-to-copy workflows are [`examples/workflows/forgejo-release.yml`](examples/workflows/forgejo-release.yml) and [`examples/workflows/github-release.yml`](examples/workflows/github-release.yml). The examples pack also includes a prompt file for Forgejo (`.forgejo/release-notes.md`) or GitHub (`.github/release-notes-instructions.md`). An in-repository prompt overrides dashboard and global instructions.

## Usage

Configure the server by copying `config.example.yaml` to `config.yaml`, or set the environment variables directly. The `serve` command requires `ADMIN_TOKEN`; when GitHub or Forgejo is enabled, it also requires `LLM_BASE_URL`.

Start the daemon (webhooks, dashboard, and API):

```bash
./annalist serve
```

The binary always looks for `config.yaml` in the working directory. To use a config file elsewhere, change into that directory before starting the binary:

```bash
cd /etc/annalist && /usr/local/bin/annalist serve
```

Generate notes for one release from the command line. `--publish` writes them to the release body; without `--publish`, Annalist prints the notes to stdout.

```bash
./annalist generate \
  --platform github \
  --owner djdembeck \
  --repo my-project \
  --to-tag v1.2.0 \
  --from-tag v1.1.0 \
  --publish
```

The command accepts `--platform --owner --repo --to-tag [--from-tag] [--profile name] [--display-version text] [--publish]`. `--profile` selects one named release-note profile from the repository's `.annalist/release-notes.yaml` manifest, and `--display-version` presents a version other than the source tag (omitted, the source tag is used). It always forces regeneration. Platform credentials and `LLM_BASE_URL` must be configured for generation.

Show the build version:

```bash
./annalist version
```

Run `annalist generate --help` (or `annalist serve` with no config) for a brief usage summary. Full configuration keys are documented in [`config.example.yaml`](config.example.yaml).

### Configurable release-note tone

Annalist treats `tone` as a configuration term for the generated release-note style. Set it globally or per repository. The dashboard provides three built-in presets—`chronicler`, `engineer`, and `launch`; any other `tone` value is treated as a custom freeform system prompt. Additional per-repository Markdown instructions can bind style, length, or terminology.

Repositories can also define **named release-note profiles** in `.annalist/release-notes.yaml`, one prompt per named audience, selected explicitly per generation request with `profile` (API) or `--profile` (CLI). A request with no profile keeps the legacy in-repository instructions behavior unchanged; see [docs/release-notes-voices.md](docs/release-notes-voices.md) for the manifest schema, `display_version` semantics, and publication rules.

## Configuration

Configuration is optional: Annalist reads `config.yaml` from the working directory when present, then applies environment variables. Precedence is:

```text
defaults < config.yaml < environment variables
```

The annotated [`config.example.yaml`](config.example.yaml) documents every key. The commonly used keys are:

| Config key | Environment variable | Default or requirement |
|---|---|---|
| `server.host` | `SERVER_HOST` | `0.0.0.0` |
| `server.port` | `SERVER_PORT` | `8080` |
| `data.dir` | `DATA_DIR` | `./data` |
| `llm.base_url` | `LLM_BASE_URL` | Required when a platform is enabled |
| `llm.api_key` | `LLM_API_KEY` | Optional; depends on the endpoint |
| `llm.model` | `LLM_MODEL` | `qwen3.5-397b-a17b` |
| `llm.temperature` | `LLM_TEMPERATURE` | `0.85` |
| `llm.max_tokens` | `LLM_MAX_TOKENS` | `4096` |
| `llm.timeout_s` | `LLM_TIMEOUT_S` | `120` seconds |
| `github.app_id` | `GITHUB_APP_ID` | GitHub App ID |
| `github.app_private_key_file` | `GITHUB_APP_PRIVATE_KEY_FILE` | Container path to the `.pem` file |
| `github.webhook_secret` | `GITHUB_WEBHOOK_SECRET` | Secret used to verify GitHub webhooks |
| `forgejo.url` | `FORGEJO_URL` | Forgejo instance base URL; no default |
| `forgejo.token` | `FORGEJO_TOKEN` | Forgejo API token |
| `forgejo.webhook_secret` | `FORGEJO_WEBHOOK_SECRET` | Secret used to verify Forgejo webhooks |
| `admin.token` | `ADMIN_TOKEN` | Required to start `annalist serve` |

## HTTP API and webhook routes

`GET /api/health` is the unauthenticated health check. The following admin API routes require the bearer token matching `ADMIN_TOKEN`:

- `GET /api/status`
- `GET /api/repos`
- `GET /api/repos/available`
- `POST /api/repos`
- `PUT /api/repos/{platform}/{owner}/{repo}/settings`
- `POST /api/repos/{platform}/{owner}/{repo}/generate`
- `GET /api/settings`
- `PUT /api/settings`

The generate endpoint, `POST /api/repos/{platform}/{owner}/{repo}/generate`, accepts optional JSON fields `from_tag`, `to_tag`, `profile`, `display_version`, `publish` (default `false`), `force`, and `mode`. `profile` selects one named profile from the repository's `.annalist/release-notes.yaml` manifest; `display_version` presents a version other than the source tag and resolves to the source tag when omitted. The response carries `notes`, `release_id`, and `published` alongside the resolved generation contract: `profile`, `display_version`, `from_tag`, `to_tag`, and a `config_digest` that anchors the idempotency cache. An invalid profile name or a blank `display_version` returns 400; a missing or malformed manifest, an unknown profile, or a missing or empty prompt file returns 422 with no LLM call made.

The webhook routes are:

- `/webhooks/github`, mounted when GitHub is enabled by a webhook secret or by `app_id` plus `private_key_file`.
- `/webhooks/forgejo`, mounted when Forgejo is enabled by a token or a webhook secret.

All other paths fall through to the embedded SvelteKit dashboard. A binary built without the `webui` build tag serves the 404 fallback instead of the dashboard.

## Building

The repository is a Go module at [`github.com/djdembeck/annalist`](https://github.com/djdembeck/annalist). Go `1.26.5` is required. Bun `1.x` is needed only when modifying or rebuilding the web dashboard.

Build the command-line binary from source:

```bash
git clone https://github.com/djdembeck/annalist.git
cd annalist
go build -o annalist ./cmd/annalist
```

To embed the dashboard, build the SvelteKit frontend first and use the `webui` build tag:

```bash
cd web && bun run build && cd ..
go build -tags webui -o annalist ./cmd/annalist
```

The Docker build performs this frontend build and embeds the resulting SPA automatically. Without `-tags webui`, `web/embed_stub.go` supplies a 404 dashboard fallback.

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
