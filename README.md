# Annalist

AI-generated release notes for GitHub and Forgejo. Annalist listens for release webhooks, clones the repository, summarizes commit history through an OpenAI-compatible LLM, and writes the notes back into the release body. It ships with a small admin dashboard for manual generation and per-repo settings.

Annalist is aimed at DevOps and SRE teams who want consistent, human-sounding release notes without hand-writing each one. It supports GitHub Apps, Forgejo webhooks, and on-demand CLI generation, with idempotent regeneration so the same webhook can be retried safely.

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
