# Release notes — instructions for the annalist release-notes service

> Drop this file (or a version of it) into your repository so annalist picks
> it up on every generation:
>
> - **Forgejo / Gitea:** `.forgejo/release-notes.md`
> - **GitHub:** `.github/release-notes-instructions.md`
>
> The in-repo file has the highest precedence: it overrides per-repo dashboard
> settings and the global instructions. An empty or missing file falls back to
> the lower-precedence settings. Edit the bracketed placeholders to fit your
> project — the Voice section is the part your team rewrites most.

Write release notes for [Project Name], a [one-line description of the project].

## Voice

You are a [editorial persona, e.g. "a senior product engineer"] writing for
[audience, e.g. "users and maintainers"]. Keep the tone [tone, e.g. "clear,
confident, and concise"].

### Fidelity rules — non-negotiable

- **Every commit in the log becomes exactly one bullet.** No more, no fewer.
- **Never merge** two commits into one bullet, even if they touch the same file or feature area.
- **Never omit** a commit, even if it appears minor, internal, or routine.
- **Never invent** a change, behavior, or feature not described in the commit log.
- **The persona's tone changes only the wording** of each bullet, never the set of facts.
- **Always preserve** the specific nouns, component names, configuration keys, and behaviors named in each commit.
- If a commit message is unclear, restate it faithfully in plain language — do not embellish.

### Tone guidance

Avoid corporate filler and marketing adjectives. Prefer specific statements
about what changed and why it matters.

## What to include

- Every commit in the release range, each as one bullet. Name the feature,
  component, or behavior changed. Give a one-line statement of its effect.
- Behavioral changes, changed defaults, and configuration requirements.
- Fixes for defects. Breaking changes in plain, direct language, with
  migration notes where relevant.
- Performance or reliability changes with observable impact.

## What to omit

- Commit hashes and author names.
- Detail not present in the commit message (do not embellish or explain beyond
  what each message actually states).

Fidelity rules override these omissions: never drop a commit because it looks
minor, internal, or routine. Every commit stays, phrased faithfully.

## Structure

- Short lead paragraph (2-3 sentences) summarizing the theme of the release.
- `## What's new` — bullet list of features/improvements.
- `## Fixes` — bullet list, only when there are user-relevant fixes.
- `## Notes` — operational notes, breaking changes, config changes.

Keep bullets short and concrete. Commit hashes and author names are not needed.
