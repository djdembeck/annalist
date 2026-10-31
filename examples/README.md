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
| `release-notes.yaml` | Named release-note profile manifest (three example profiles) | `.annalist/release-notes.yaml` in the consuming repo |
| `prompts/maintainer.md` | Technical-audience profile prompt (example) | `.annalist/prompts/maintainer.md` in the consuming repo |
| `prompts/customer.md` | End-user-audience profile prompt (example) | `.annalist/prompts/customer.md` in the consuming repo |
| `prompts/compact.md` | Compact changelog-style profile prompt (example) | `.annalist/prompts/compact.md` in the consuming repo |
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
   Forgejo also needs a `FORGEJO_TOKEN` with release write access; on GitHub
   `GH_TOKEN` is automatic.

## How the pieces fit

Annalist can fill release bodies passively via release webhooks, or actively —
the pattern shown here is **generate-first**: the workflow asks annalist for
complete notes with `"publish": false` **before** the release exists, then
creates the release once with the full body. No empty release, no webhook
round-trip, no polling.

The workflows also implement **automatic versioning with an immutable,
create-only reservation**:

1. A **read-only preflight** freezes the single source SHA for the
   invocation (push: the pushed commit; dispatch: the tip of the selected
   branch, after rejecting a wrong-branch dispatch) and decides the version
   from the baseline — the highest valid semver-ish tag reachable from the
   frozen SHA, **including unfinished reservations** (a failed number stays
   reserved; it is never recirculated). The first release is `v0.1.0`.
2. The **reserve job** is the only writer of version tags: it creates the
   decided number as an annotated tag built **against the exact frozen SHA**
   (create-only; never moved, force-pushed, or deleted). If the tag already
   points exactly at this source, the reservation is **reused** — a rerun of
   the same source, even after newer releases exist. A tag at a different
   SHA is a **collision: the run fails**, it never moves the ref.
3. The **publish job** re-verifies the tag → frozen source, then creates the
   release **once** with complete notes. A missing tag at publish time is
   itself a failure (tags are only ever created by reserve).

The notes range is annalist's `range_policy: "previous_published"`
resolution — the previous published, non-draft ancestor release, full
history for the first published release. The request deliberately **omits
`from_tag`**: passing the previous reachable tag would reproduce the
unfinished-reservation bug (an unreleased tag hides the changes behind it).
Migrated GitHub apps keep their fill-after webhook path separately; the same
policy applies there. No new token is needed.

Versioning is conventional-commit based (`BREAKING CHANGE` / `feat!` →
major, `feat` → minor, else patch) with `workflow_dispatch` inputs
(`version`, `bump`) for manual control. A push that adds no reachable commit
past the newest reservation stops successfully (coalescing, not failure).

An **existing tag is not completion**: a release with an empty body is
refilled on rerun; a fully published release no-ops. Re-runs are idempotent.
The release job exposes `source` (the 40-character SHA), `tag` (`vX.Y.Z`),
and `version` (`X.Y.Z`) outputs for native downstream adapters.

GitHub: `concurrency: { group: release, cancel-in-progress: false,
queue: max }` (the documented pending queue; a queued manual dispatch is not
replaced by later pushes). actionlint v1.7.12 has a known parser gap on the
documented `queue: max` key (rhysd/actionlint#657) — keep the key.
Forgejo: best-effort `concurrency` without the `queue` key; Forgejo's queued-run
retention is unverified, so until that retention proof exists keep a
manual-only (dispatch-only) repository on the flow the team has verified,
and do not assume automatic-mode parity with GitHub.

**Dispatch-only mode:** delete the `push:` trigger block — the
`workflow_dispatch` entry then runs on its own.

## Named release-note profiles

The default workflow above does **not** use profiles: it stays a single
generate-first call with one note per release. Profiles are an opt-in,
explicit extension.

A consuming repository defines named profiles in `.annalist/release-notes.yaml`
(see `release-notes.yaml` here) — a strict manifest mapping each profile name
to a prompt file. Each prompt file under `.annalist/prompts/` (the three
examples are in `prompts/` here) is the **entire** system prompt for that
profile: no built-in persona or the legacy instructions file is layered on
top.

Behavior:

- A request selects exactly **one** profile per generation call — one
  request, one profile's output. There is no batch mode and no output
  composition.
- No profile requested → legacy behavior, unchanged (the provider-specific
  in-repo instructions file, or settings/persona fallback).
- Both the manifest and the selected prompt are read **pinned to the source
  tag** (`to_tag`), so notes are generated from the prompt at that version.
- `display_version` may differ from the source tag (e.g. a beta tag shown as
  a release version); omitted, it resolves to the source tag.
- Multi-output releases: generate each profile with `publish: false`, compose
  the body externally, and publish once. Repeated `publish: true` calls for
  different profiles on the same release replace the same Annalist-owned body
  last-write-wins.

Both workflow examples include a commented, optional second generate call
showing a named-profile request. See [docs/release-notes-voices.md](../docs/release-notes-voices.md)
for the full contract.

## Adapting

- **Trigger branch:** change `branches:` in the workflow to wherever your team
  merges (`main`, `develop`, …).
- **Container image:** the optional commented block in the workflows shows an
  idempotent `docker build`/`push`; add it if the repo ships an image. The
  release step itself does not build anything.
- **One-time or manual releases:** if you don't want every merge to release,
  delete the `push:` trigger block and release via `workflow_dispatch` only —
  the exact-source reservation and notes generation work unchanged.
